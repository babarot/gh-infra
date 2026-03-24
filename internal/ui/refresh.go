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
)

// RefreshTask describes a task to track in the spinner display.
type RefreshTask struct {
	Name      string // key for Done()/Error() matching AND running label
	DoneLabel string // shown when task completes (defaults to Name if empty)
}

type taskStatus int

const (
	taskRunning taskStatus = iota
	taskDone
	taskError
)

type refreshItem struct {
	name      string
	doneLabel string
	status    taskStatus
	errMsg    string
	spinner   spinner.Model
}

type refreshModel struct {
	items     []refreshItem
	remaining int
}

type taskDoneMsg struct{ name string }
type taskErrorMsg struct {
	name string
	err  error
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
		items[i] = refreshItem{
			name:      task.Name,
			doneLabel: doneLabel,
			status:    taskRunning,
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
	case taskDoneMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].status = taskDone
				m.remaining--
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

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
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
		case taskRunning:
			fmt.Fprintf(&b, "  %s %s...\n", item.spinner.View(), item.name)
		}
	}
	return tea.NewView(b.String())
}

// RefreshTracker tracks parallel task progress with spinners or plain text.
type RefreshTracker struct {
	program  *tea.Program
	fallback bool
	done     chan struct{}
	mu       sync.Mutex
}

// RunRefresh starts a spinner display for the given tasks.
// Returns a non-nil tracker in all cases.
func RunRefresh(tasks []RefreshTask) *RefreshTracker {
	if len(tasks) == 0 {
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	f, ok := DefaultPrinter.ErrWriter().(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		for _, task := range tasks {
			DefaultPrinter.Progress(task.Name + "...")
		}
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	model := newRefreshModel(tasks)
	p := tea.NewProgram(
		model,
		tea.WithOutput(os.Stderr),
		tea.WithInput(nil),
	)

	tracker := &RefreshTracker{
		program: p,
		done:    make(chan struct{}),
	}

	go func() {
		defer close(tracker.done)
		_, _ = p.Run()
	}()

	return tracker
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

// Error marks a task as failed.
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
