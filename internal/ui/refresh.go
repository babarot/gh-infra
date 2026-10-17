package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
	// ErrorPrefix comes before the first line of an error, as in
	// "skipped · HTTP 403: ..." (none if empty).
	ErrorPrefix string
	// WaitLabel replaces the status while the task waits for its next
	// report after one has come in (the status is kept if empty).
	WaitLabel string
}

// Mark is how a finished row shows its result: its symbol and colors.
type Mark int

const (
	MarkDone   Mark = iota // ✓; the label in dim
	MarkNone               // ✓; the name and label in dim, as for nothing to change
	MarkChange             // ~ in yellow
	MarkCreate             // + in green; the label in green
	MarkDelete             // - in red; the label in red
)

type taskStatus int

const (
	taskRunning taskStatus = iota
	taskDone
	taskError
	taskFailed
	taskCanceled
)

type refreshItem struct {
	name        string
	doneLabel   string
	failLabel   string
	errorPrefix string
	waitLabel   string
	mark        Mark
	status      taskStatus
	errMsg      string
	statusText  string // right-side live status (e.g. "secrets...", ".github/ci.yml...")
	checkpoint  string // what is already done while the task still runs, shown as "<checkpoint> ✓"
	pending     int    // expected Done() calls before marking complete (default 1)
	end         time.Time
	spinner     spinner.Model
}

type refreshModel struct {
	items     []refreshItem
	remaining int
	canceled  chan struct{} // closed on Ctrl+C to signal callers
	width     int           // terminal width for error truncation
	notice    string        // dim line under the tasks, e.g. a rate limit wait
	stopped   bool          // Ctrl+C or an interrupt ended the display
	start     time.Time     // when the tasks started, for their elapsed time
	now       func() time.Time
}

var (
	interruptMu      sync.Mutex
	interruptHandler func()
)

// SetInterruptHandler sets the function called when Ctrl+C is pressed while
// the spinner runs. The spinner reads Ctrl+C as a key, so no signal reaches
// the process; the command uses this to cancel its work as a signal would.
// It returns a function that restores the previous handler.
func SetInterruptHandler(fn func()) (restore func()) {
	interruptMu.Lock()
	defer interruptMu.Unlock()
	prev := interruptHandler
	interruptHandler = fn
	return func() {
		interruptMu.Lock()
		defer interruptMu.Unlock()
		interruptHandler = prev
	}
}

func callInterruptHandler() {
	interruptMu.Lock()
	fn := interruptHandler
	interruptMu.Unlock()
	if fn != nil {
		fn()
	}
}

// taskDoneMsg reports one completion of a task. With labeled, mark and label
// replace how the row shows its result; the last one sent before the task
// completes is shown.
type taskDoneMsg struct {
	name    string
	labeled bool
	mark    Mark
	label   string
}
type interruptMsg struct{}
type taskErrorMsg struct {
	name string
	err  error
}
type taskFailMsg struct{ name string }
type taskCheckpointMsg struct {
	name   string
	status string
}
type taskStatusMsg struct {
	name   string
	status string
}
type noticeMsg struct{ text string }

func newRefreshModel(tasks []RefreshTask) refreshModel {
	width := 80 // default
	if w, _, err := term.GetSize(os.Stderr.Fd()); err == nil && w > 0 {
		width = w
	}

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
			name:        task.Name,
			doneLabel:   doneLabel,
			failLabel:   failLabel,
			errorPrefix: task.ErrorPrefix,
			waitLabel:   task.WaitLabel,
			status:      taskRunning,
			pending:     pending,
			spinner:     s,
		}
	}
	return refreshModel{
		items:     items,
		remaining: len(tasks),
		width:     width,
		start:     time.Now(),
		now:       time.Now,
	}
}

func (m refreshModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.items))
	for i := range m.items {
		cmds[i] = m.items[i].spinner.Tick
	}
	return tea.Batch(cmds...)
}

// cancelRunning shows the tasks still running as canceled.
func (m refreshModel) cancelRunning() {
	for i := range m.items {
		if m.items[i].status == taskRunning {
			m.items[i].status = taskCanceled
		}
	}
}

func (m refreshModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case noticeMsg:
		m.notice = msg.text
		return m, nil

	case taskStatusMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].statusText = msg.status
				break
			}
		}
		return m, nil

	case taskCheckpointMsg:
		// The spinner keeps running: a checkpoint is shown in front of the
		// status, not as the ✓ of a finished row.
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.items[i].checkpoint = msg.status
				m.items[i].statusText = ""
				break
			}
		}
		return m, nil

	case taskDoneMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			if msg.labeled {
				m.items[i].mark = msg.mark
				m.items[i].doneLabel = msg.label
			}
			if m.items[i].status == taskRunning {
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.items[i].status = taskDone
					m.items[i].statusText = ""
					m.items[i].checkpoint = ""
					m.items[i].end = m.now()
					m.remaining--
				} else if m.items[i].waitLabel != "" {
					m.items[i].statusText = m.items[i].waitLabel
				}
			} else if m.items[i].pending > 0 {
				// Already errored/failed but other sources still pending.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
				}
			}
			break
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case taskErrorMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			if m.items[i].status == taskRunning {
				m.items[i].status = taskError
				m.items[i].errMsg = msg.err.Error()
				m.items[i].statusText = ""
				m.items[i].checkpoint = ""
				m.items[i].end = m.now()
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
				}
			} else if m.items[i].pending > 0 {
				// Already done, errored or failed, with other sources still
				// pending: count this one and keep the first outcome shown.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
				}
			}
			break
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case taskFailMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			if m.items[i].status == taskRunning {
				m.items[i].status = taskFailed
				m.items[i].statusText = ""
				m.items[i].checkpoint = ""
				m.items[i].end = m.now()
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
				}
			} else if m.items[i].pending > 0 {
				// Already done, errored or failed, with other sources still
				// pending: count this one and keep the first outcome shown.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
				}
			}
			break
		}
		if m.remaining <= 0 {
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancelRunning()
			m.stopped = true
			close(m.canceled)
			callInterruptHandler()
			return m, tea.Quit
		}
		return m, nil

	case interruptMsg:
		m.cancelRunning()
		m.stopped = true
		return m, tea.Quit

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
	// Compute max name width for column alignment.
	maxName := 0
	for _, item := range m.items {
		if n := len(item.name); n > maxName {
			maxName = n
		}
	}

	var b strings.Builder
	for _, item := range m.items {
		padded := fmt.Sprintf("%-*s", maxName, item.name)
		// elapsed is the task's time so far, or what it took once finished.
		end := m.now()
		if item.status != taskRunning && !item.end.IsZero() {
			end = item.end
		}
		elapsed := Dim.Render(formatElapsed(end.Sub(m.start)))
		switch item.status {
		case taskDone:
			icon, name, label := doneParts(item.mark)
			if item.doneLabel != "" && item.doneLabel != item.name {
				fmt.Fprintf(&b, "  %s %s  %s  %s\n", icon, name.Render(padded), elapsed, label.Render(item.doneLabel))
			} else {
				fmt.Fprintf(&b, "  %s %s  %s\n", icon, name.Render(padded), elapsed)
			}
		case taskError:
			// Show only a brief, single-line error; full details are printed after the spinner.
			prefix := 2 + 1 + 1 + maxName + 2 + elapsedWidth + 2 // "  " + icon + " " + padded + "  " + elapsed + "  "
			lead := ""
			if item.errorPrefix != "" {
				lead = item.errorPrefix + " · "
				prefix += len(lead)
			}
			available := m.width - prefix
			available = max(available, 20)
			errText := truncateError(item.errMsg, available)
			fmt.Fprintf(&b, "  %s %s  %s  %s%s\n", Red.Render(IconError), Bold.Render(padded), elapsed, Red.Render(lead), Dim.Render(errText))
		case taskFailed:
			if item.failLabel == item.name {
				fmt.Fprintf(&b, "  %s %s  %s\n", Red.Render(IconError), padded, elapsed)
			} else {
				fmt.Fprintf(&b, "  %s %s\n", Red.Render(IconError), item.failLabel)
			}
		case taskCanceled:
			fmt.Fprintf(&b, "  %s %s\n", Dim.Render(IconError), Dim.Render(padded+" (canceled)"))
		case taskRunning:
			status := Dim.Render(item.statusText)
			if item.checkpoint != "" {
				status = Dim.Render(item.checkpoint) + " " + Green.Render(IconSuccess)
				if item.statusText != "" {
					status += Dim.Render(" · " + item.statusText)
				}
			}
			fmt.Fprintf(&b, "  %s %s  %s  %s\n", item.spinner.View(), padded, elapsed, status)
		}
	}
	if m.notice != "" {
		fmt.Fprintf(&b, "\n  %s\n", Dim.Render(m.notice))
	}
	// While tasks run, say how to stop them. The display's last frame (all
	// finished, or stopped by Ctrl+C) leaves it out.
	if m.remaining > 0 && !m.stopped {
		fmt.Fprintf(&b, "\n  %s\n", Dim.Render("ctrl+c to cancel"))
	}
	return tea.NewView(b.String())
}

// elapsedWidth is the width of the elapsed time column ("12.3s").
const elapsedWidth = 5

// formatElapsed formats d for the elapsed column, right-aligned.
func formatElapsed(d time.Duration) string {
	return fmt.Sprintf("%*.1fs", elapsedWidth-1, max(d, 0).Seconds())
}

// doneParts returns how a finished row with mark shows: its icon, and the
// styles of its name and label.
func doneParts(mark Mark) (icon string, name, label lipgloss.Style) {
	plain := lipgloss.NewStyle()
	switch mark {
	case MarkNone:
		return Green.Render(IconSuccess), Dim, Dim
	case MarkChange:
		return Yellow.Render(IconChange), plain, plain
	case MarkCreate:
		return Green.Render(IconAdd), plain, Green
	case MarkDelete:
		return Red.Render(IconRemove), plain, Red
	default:
		return Green.Render(IconSuccess), plain, Dim
	}
}

// TaskError records an error for post-spinner display.
type TaskError struct {
	Name string
	Err  error
}

// RefreshTracker tracks parallel task progress with spinners or plain text.
type RefreshTracker struct {
	program  *tea.Program
	fallback bool
	done     chan struct{}
	canceled chan struct{}
	mu       sync.Mutex
	errors   []TaskError // collected errors for post-spinner display
	// interrupted is set by Interrupt; errors reported afterwards come from
	// the interrupted work and are not shown.
	interrupted bool
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

// SetNotice shows a dim line under the tasks, or removes it when text is "".
// Without the spinner (fallback mode, e.g. in CI) it prints text as a line
// of progress, and removing it does nothing.
func (t *RefreshTracker) SetNotice(text string) {
	if t == nil {
		return
	}
	if t.fallback {
		if text != "" {
			DefaultPrinter.Progress(text)
		}
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(noticeMsg{text: text})
	}
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

// Checkpoint marks a sub-phase as completed while the task remains running.
func (t *RefreshTracker) Checkpoint(name, status string) {
	if t == nil {
		return
	}
	if t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskCheckpointMsg{name: name, status: status})
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

// DoneWith is Done that also sets how the row shows its result once the
// task completes: mark and label replace the done label. A task reported by
// several sources can send its result so far with each report; the last one
// before it completes is shown.
func (t *RefreshTracker) DoneWith(name string, mark Mark, label string) {
	if t == nil {
		return
	}
	if t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskDoneMsg{name: name, labeled: true, mark: mark, label: label})
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

// Interrupt stops the display as Ctrl+C does when the work was interrupted
// another way (a signal): running tasks show as canceled, and errors
// reported afterwards are ignored. It does not call the interrupt handler.
func (t *RefreshTracker) Interrupt() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.interrupted {
		return
	}
	t.interrupted = true
	if t.program != nil {
		t.program.Send(interruptMsg{})
	}
}

// Error marks a task as failed with inline error detail.
func (t *RefreshTracker) Error(name string, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.interrupted {
		t.mu.Unlock()
		return
	}
	t.errors = append(t.errors, TaskError{Name: name, Err: err})
	if t.fallback {
		t.mu.Unlock()
		DefaultPrinter.Error(name, err.Error())
		return
	}
	if t.program != nil {
		t.program.Send(taskErrorMsg{name: name, err: err})
	}
	t.mu.Unlock()
}

// Errors returns all errors collected during the tracker's lifetime.
// Safe to call after Wait() returns.
func (t *RefreshTracker) Errors() []TaskError {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TaskError, len(t.errors))
	copy(out, t.errors)
	return out
}

// PrintErrors prints detailed errors to stderr, grouped by task name,
// as multi-line per-target blocks separated by blank lines:
//
//	owner/repo-1:
//	  error line 1
//	  error line 2
//
//	owner/repo-2:
//	  error line 1
//
// Call after Wait() to display the full error messages that were truncated
// in the spinner view. Returns true if any errors were printed.
func (t *RefreshTracker) PrintErrors() bool {
	errors := t.Errors()
	if len(errors) == 0 {
		return false
	}
	p := DefaultPrinter
	for _, te := range errors {
		p.BlankLine()
		p.ErrorReport(te.Name, te.Err.Error())
	}
	return true
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

// truncateError returns the first line of msg, truncated to maxWidth runes.
// If the original had multiple lines or was truncated, "…" is appended.
func truncateError(msg string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 60
	}
	first, _, hasMore := strings.Cut(msg, "\n")
	if len(first) > maxWidth {
		return first[:maxWidth-1] + IconEllipsis
	}
	if hasMore {
		return first + IconEllipsis
	}
	return first
}
