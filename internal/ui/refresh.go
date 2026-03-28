package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/babarot/gh-infra/internal/logger"
)

// RefreshTask describes a task to track in the spinner display.
type RefreshTask struct {
	Name      string // key for Done()/Error()/Fail() matching AND running label
	DoneLabel string // shown when task completes successfully (defaults to Name if empty)
	FailLabel string // shown when task fails via Fail() (defaults to Name if empty)
	Pending   int    // expected Done() calls before completion (default 1)
}

// BuildRefreshTasks creates RefreshTask entries for a list of target names.
// Each task's Name is "Fetching {name} ({suffix})" and DoneLabel is "Fetched {name} ({suffix})".
func BuildRefreshTasks(names []string, suffix string) []RefreshTask {
	tasks := make([]RefreshTask, len(names))
	for i, name := range names {
		tasks[i] = RefreshTask{
			Name:      "Fetching " + name + " (" + suffix + ")",
			DoneLabel: "Fetched " + name + " (" + suffix + ")",
		}
	}
	return tasks
}

type taskStatus int

const (
	taskRunning taskStatus = iota
	taskDone
	taskError
	taskFailed
	taskCanceled
)

type refreshItem struct {
	name       string
	doneLabel  string
	failLabel  string
	status     taskStatus
	errMsg     string
	statusText string // right-side live status (e.g. "secrets...", ".github/ci.yml...")
	pending    int    // expected Done() calls before marking complete (default 1)
	spinner    spinner.Model
}

type refreshModel struct {
	items     []refreshItem
	remaining int
	canceled  chan struct{} // closed on Ctrl+C to signal callers
}

type taskDoneMsg struct{ name string }
type taskErrorMsg struct {
	name string
	err  error
}
type taskFailMsg struct{ name string }
type taskStatusMsg struct {
	name   string
	status string
}

func newRefreshModel(tasks []RefreshTask) refreshModel {
	items := make([]refreshItem, len(tasks))
	for i, task := range tasks {
		s := spinner.New(
			spinner.WithSpinner(spinner.Jump),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("6"))),
		)
		doneLabel := task.DoneLabel
		if doneLabel == "" {
			doneLabel = task.Name
		}
		failLabel := task.FailLabel
		if failLabel == "" {
			failLabel = task.Name
		}
		pending := task.Pending
		if pending <= 0 {
			pending = 1
		}
		items[i] = refreshItem{
			name:      task.Name,
			doneLabel: doneLabel,
			failLabel: failLabel,
			status:    taskRunning,
			pending:   pending,
			spinner:   s,
		}
	}
	return refreshModel{
		items:     items,
		remaining: len(tasks),
	}
}

func (m refreshModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.items))
	for i := range m.items {
		cmds[i] = m.items[i].spinner.Tick
	}
	return tea.Batch(cmds...)
}

func (m refreshModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case taskStatusMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].statusText = msg.status
				break
			}
		}
		return m, nil

	case taskDoneMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.items[i].status = taskDone
					m.items[i].statusText = ""
					m.remaining--
				}
				break
			}
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case taskErrorMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].status = taskError
				m.items[i].errMsg = msg.err.Error()
				m.remaining--
				break
			}
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case taskFailMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].status = taskFailed
				m.remaining--
				break
			}
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			for i := range m.items {
				if m.items[i].status == taskRunning {
					m.items[i].status = taskCanceled
				}
			}
			close(m.canceled)
			return m, tea.Quit
		}
		return m, nil

	default:
		var cmds []tea.Cmd
		for i := range m.items {
			if m.items[i].status == taskRunning {
				var cmd tea.Cmd
				m.items[i].spinner, cmd = m.items[i].spinner.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		return m, tea.Batch(cmds...)
	}
}

func (m refreshModel) View() tea.View {
	var b strings.Builder
	for _, item := range m.items {
		switch item.status {
		case taskDone:
			fmt.Fprintf(&b, "  %s %s\n", Green.Render(IconSuccess), item.doneLabel)
		case taskError:
			fmt.Fprintf(&b, "  %s %s: %s\n", Red.Render(IconError), Bold.Render(item.name), item.errMsg)
		case taskFailed:
			fmt.Fprintf(&b, "  %s %s\n", Red.Render(IconError), item.failLabel)
		case taskCanceled:
			fmt.Fprintf(&b, "  %s %s\n", Dim.Render(IconError), Dim.Render(item.name+" (canceled)"))
		case taskRunning:
			if item.statusText != "" {
				fmt.Fprintf(&b, "  %s %s  %s\n", item.spinner.View(), item.name, Dim.Render(item.statusText))
			} else {
				fmt.Fprintf(&b, "  %s %s...\n", item.spinner.View(), item.name)
			}
		}
	}
	return tea.NewView(b.String())
}

// RefreshTracker tracks parallel task progress with spinners or plain text.
type RefreshTracker struct {
	program  *tea.Program
	fallback bool
	done     chan struct{}
	canceled chan struct{}
	mu       sync.Mutex
}

// RunRefresh starts a spinner display for the given tasks.
// Returns a non-nil tracker in all cases.
func RunRefresh(tasks []RefreshTask) *RefreshTracker {
	if len(tasks) == 0 {
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	// When logging is active, spinners would interleave with log lines and
	// produce unreadable output. Fall back to plain-text progress instead.
	f, ok := DefaultPrinter.ErrWriter().(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) || logger.Enabled() {
		for _, task := range tasks {
			DefaultPrinter.Progress(task.Name + "...")
		}
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	canceled := make(chan struct{})
	model := newRefreshModel(tasks)
	model.canceled = canceled
	p := tea.NewProgram(
		model,
		tea.WithOutput(os.Stderr),
	)

	tracker := &RefreshTracker{
		program:  p,
		done:     make(chan struct{}),
		canceled: canceled,
	}

	go func() {
		defer close(tracker.done)
		_, _ = p.Run()
		// FIXME(bubbletea): Drain pending terminal query responses that
		// bubbletea v2 fails to consume during shutdown, causing escape
		// sequences to leak into the shell. Remove when upstream is fixed.
		// https://github.com/charmbracelet/bubbletea/issues/1590
		drainStdinAfterBubbletea()

	}()

	return tracker
}

// UpdateStatus updates the right-side live status text for a running task.
func (t *RefreshTracker) UpdateStatus(name, status string) {
	if t == nil {
		return
	}
	if t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskStatusMsg{name: name, status: status})
	}
}

// Done marks a task as successfully completed.
func (t *RefreshTracker) Done(name string) {
	if t == nil {
		return
	}
	if t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskDoneMsg{name: name})
	}
}

// Fail marks a task as failed without inline error detail.
// Use this when the error detail will be displayed separately (e.g. via Printer.Warning).
func (t *RefreshTracker) Fail(name string) {
	if t == nil {
		return
	}
	if t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskFailMsg{name: name})
	}
}

// Error marks a task as failed with inline error detail.
func (t *RefreshTracker) Error(name string, err error) {
	if t == nil {
		return
	}
	if t.fallback {
		DefaultPrinter.Error(name, err.Error())
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskErrorMsg{name: name, err: err})
	}
}

// Canceled returns a channel that is closed when the user presses Ctrl+C.
func (t *RefreshTracker) Canceled() <-chan struct{} {
	if t == nil {
		return nil
	}
	return t.canceled
}

// Wait blocks until all tasks are reported and the display finishes.
func (t *RefreshTracker) Wait() {
	if t == nil {
		return
	}
	<-t.done
}

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
