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
	// Total is how many steps the task has (an apply's changes). With it,
	// the row shows how many have ended (Step) and the heading counts steps.
	Total int
}

// StepResult is how one step of a task ended, reported with Step.
type StepResult int

const (
	StepApplied StepResult = iota
	StepFailed
	StepSkipped
)

// RefreshHeading is the line above the spinner rows.
type RefreshHeading struct {
	// Title is shown while the tasks run, followed by their progress, as in
	// "Fetching current state from GitHub API  ━━━━━━──────  3/5  1.4s".
	Title string
	// Phase is printed instead without a terminal (the spinner's fallback)
	// or without tasks: the static line the command has always printed.
	Phase string
	// Done starts the line that replaces the title once every task has
	// finished, as "Fetched" in "✓ Fetched 5 repositories in 2.1s".
	Done string
	// Failed names the tasks that ended with an error in that line, as
	// "skipped" in "✗ Fetched 5 repositories in 2.1s, 1 skipped".
	Failed string
	// Load returns how many gh processes run, how many calls wait for one,
	// and the limit (--concurrency). Without it the heading has no gauge.
	Load func() (running, waiting, limit int)
	// Changes, when set, makes the heading count the tasks' steps instead
	// of the tasks: "8/13" while running, then "✓ <Done> 13 changes in 2.8s"
	// or "✗ <Done> 12 of 13 changes in 2.8s, 1 failed".
	Changes int
}

// Mark is how a finished row shows its result: its symbol and colors.
type Mark int

const (
	MarkDone   Mark = iota // ✓; the label in dim
	MarkNone               // ✓; the name and label in dim, as for nothing to change
	MarkChange             // ~ in yellow
	MarkCreate             // + in green; the label in green
	MarkDelete             // - in red; the label in red
	MarkFailed             // ✗ in red; the name in bold, as for an error
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
	// started is set by the first message about the task. Until then the
	// task is queued. "Started" means its work has begun, not that a gh
	// process runs for it: a started task can be waiting for a slot of the
	// runner, which only the heading's gauge shows.
	started bool
	begin   time.Time // when the first message came
	end     time.Time
	printed bool // printed above the compact view
	// total is the number of steps (RefreshTask.Total); the others count
	// the steps that ended, by how.
	total, applied, failed, skipped int
	spinner                         spinner.Model
}

type refreshModel struct {
	heading   RefreshHeading
	items     []refreshItem
	remaining int
	canceled  chan struct{} // closed on Ctrl+C to signal callers
	width     int           // terminal width for error truncation
	notice    string        // dim line under the tasks, e.g. a rate limit wait
	stopped   bool          // Ctrl+C or an interrupt ended the display
	start     time.Time     // when the tasks started, for their elapsed time
	now       func() time.Time
	// lastReport is when the last report of the last row to finish came:
	// the end of the work, even when a row errored at an earlier report.
	lastReport time.Time
	height     int  // terminal height; 0 when unknown
	compact    bool // many rows: see compactView. Once set, it stays set
	// finished lists the rows in the order they finished, for printing them
	// above the compact view in that order.
	finished []int
	queue    []string // lines waiting to be printed above the view (flush)
	printing bool     // a batch of queue is being printed
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
type taskStepMsg struct {
	name   string
	result StepResult
}

func newRefreshModel(tasks []RefreshTask) refreshModel {
	width, height := 80, 0 // defaults; 0 height is unknown
	if w, h, err := term.GetSize(os.Stderr.Fd()); err == nil && w > 0 {
		width, height = w, h
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
			total:       task.Total,
			status:      taskRunning,
			pending:     pending,
			spinner:     s,
		}
	}
	return refreshModel{
		items:     items,
		remaining: len(tasks),
		width:     width,
		height:    height,
		start:     time.Now(),
		now:       time.Now,
	}
}

func (m refreshModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.items))
	for i := range m.items {
		cmds = append(cmds, m.items[i].spinner.Tick)
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

// markStarted marks item i as started at the first message about it (see
// refreshItem.started).
func (m refreshModel) markStarted(i int) {
	if !m.items[i].started {
		m.items[i].started = true
		m.items[i].begin = m.now()
	}
}

func (m refreshModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case noticeMsg:
		m.notice = msg.text
		return m, m.settle()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, m.settle()

	case taskStepMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			m.markStarted(i)
			switch msg.result {
			case StepApplied:
				m.items[i].applied++
			case StepFailed:
				m.items[i].failed++
			case StepSkipped:
				m.items[i].skipped++
			}
			break
		}
		return m, nil

	case printedMsg:
		m.printing = false
		return m, m.flush()

	case taskStatusMsg:
		for i := range m.items {
			if m.items[i].name == msg.name && m.items[i].status == taskRunning {
				m.markStarted(i)
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
				m.markStarted(i)
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
			m.markStarted(i)
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
					m.finish(i)
					m.remaining--
					m.lastReport = m.now()
				} else if m.items[i].waitLabel != "" {
					m.items[i].statusText = m.items[i].waitLabel
				}
			} else if m.items[i].pending > 0 {
				// Already errored/failed but other sources still pending.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
					m.lastReport = m.now()
				}
			}
			break
		}
		return m, m.settle()

	case taskErrorMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			m.markStarted(i)
			if m.items[i].status == taskRunning {
				m.items[i].status = taskError
				m.items[i].errMsg = msg.err.Error()
				m.items[i].statusText = ""
				m.items[i].checkpoint = ""
				m.items[i].end = m.now()
				m.finish(i)
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
					m.lastReport = m.now()
				}
			} else if m.items[i].pending > 0 {
				// Already done, errored or failed, with other sources still
				// pending: count this one and keep the first outcome shown.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
					m.lastReport = m.now()
				}
			}
			break
		}
		return m, m.settle()

	case taskFailMsg:
		for i := range m.items {
			if m.items[i].name != msg.name {
				continue
			}
			m.markStarted(i)
			if m.items[i].status == taskRunning {
				m.items[i].status = taskFailed
				m.items[i].statusText = ""
				m.items[i].checkpoint = ""
				m.items[i].end = m.now()
				m.finish(i)
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
					m.lastReport = m.now()
				}
			} else if m.items[i].pending > 0 {
				// Already done, errored or failed, with other sources still
				// pending: count this one and keep the first outcome shown.
				m.items[i].pending--
				if m.items[i].pending <= 0 {
					m.remaining--
					m.lastReport = m.now()
				}
			}
			break
		}
		return m, m.settle()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.stopped {
				// Already stopping: the rows are being printed before the end.
				return m, nil
			}
			m.cancelRunning()
			m.stopped = true
			close(m.canceled)
			callInterruptHandler()
			if m.compact {
				m.stopCompact()
			}
			return m, m.flush()
		}
		return m, nil

	case interruptMsg:
		if m.stopped {
			return m, nil
		}
		m.cancelRunning()
		m.stopped = true
		if m.compact {
			m.stopCompact()
		}
		return m, m.flush()

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
	if m.compact {
		return tea.NewView(m.compactView())
	}
	return tea.NewView(m.fullView())
}

// fullView is the view with every row, before it becomes compact.
func (m refreshModel) fullView() string {
	maxName := m.maxNameWidth()
	var b strings.Builder
	if m.heading.Title != "" {
		fmt.Fprintf(&b, "%s\n\n", m.headingLine())
	}
	for _, item := range m.items {
		fmt.Fprintf(&b, "%s\n", m.rowLine(item, maxName))
	}
	if m.notice != "" {
		fmt.Fprintf(&b, "\n  %s\n", Dim.Render(m.notice))
	}
	// While tasks run, say how to stop them. The display's last frame (all
	// finished, or stopped by Ctrl+C) leaves it out.
	if m.remaining > 0 && !m.stopped {
		fmt.Fprintf(&b, "\n  %s\n", Dim.Render("ctrl+c to cancel"))
	}
	return b.String()
}

// maxNameWidth is the width of the longest task name, for aligning rows.
func (m refreshModel) maxNameWidth() int {
	w := 0
	for _, item := range m.items {
		w = max(w, len(item.name))
	}
	return w
}

// rowLine renders one task's row, its name padded to maxName.
func (m refreshModel) rowLine(item refreshItem, maxName int) string {
	padded := fmt.Sprintf("%-*s", maxName, item.name)
	if item.status == taskRunning && !item.started {
		return fmt.Sprintf("  %s %s  %s  %s", Dim.Render("·"), Dim.Render(padded), strings.Repeat(" ", elapsedWidth), Dim.Render("queued"))
	}
	// elapsed is the task's time so far, or what it took once finished.
	end := m.now()
	if item.status != taskRunning && !item.end.IsZero() {
		end = item.end
	}
	elapsed := Dim.Render(formatElapsed(end.Sub(item.begin)))
	switch item.status {
	case taskDone:
		icon, name, label := doneParts(item.mark)
		if item.doneLabel != "" && item.doneLabel != item.name {
			return fmt.Sprintf("  %s %s  %s  %s", icon, name.Render(padded), elapsed, label.Render(item.doneLabel))
		}
		return fmt.Sprintf("  %s %s  %s", icon, name.Render(padded), elapsed)
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
		return fmt.Sprintf("  %s %s  %s  %s%s", Red.Render(IconError), Bold.Render(padded), elapsed, Red.Render(lead), Dim.Render(errText))
	case taskFailed:
		if item.failLabel == item.name {
			return fmt.Sprintf("  %s %s  %s", Red.Render(IconError), padded, elapsed)
		}
		return fmt.Sprintf("  %s %s", Red.Render(IconError), item.failLabel)
	case taskCanceled:
		return fmt.Sprintf("  %s %s", Dim.Render(IconError), Dim.Render(padded+" (canceled)"))
	default: // taskRunning
		status := Dim.Render(item.statusText)
		if item.checkpoint != "" {
			status = Dim.Render(item.checkpoint) + " " + Green.Render(IconSuccess)
			if item.statusText != "" {
				status += Dim.Render(" · " + item.statusText)
			}
		}
		if item.total > 0 {
			ended := item.applied + item.failed + item.skipped
			steps := fmt.Sprintf("%s %s", progressBar(ended, item.total, rowBarWidth), Dim.Render(fmt.Sprintf("%d/%d", ended, item.total)))
			status = steps + "  " + status
			if item.failed > 0 {
				status += "  " + Red.Render(fmt.Sprintf("%d failed", item.failed))
			}
		}
		return fmt.Sprintf("  %s %s  %s  %s", item.spinner.View(), padded, elapsed, status)
	}
}

// headingLine is the heading: its title with the progress while tasks run,
// then what they did once all have finished.
func (m refreshModel) headingLine() string {
	failed, queued := 0, 0
	for _, item := range m.items {
		if item.status == taskError || item.status == taskFailed || (item.status == taskDone && item.mark == MarkFailed) {
			failed++
		}
		if item.status == taskRunning && !item.started {
			queued++
		}
	}
	finished, total := m.progress()

	switch {
	case m.stopped:
		return Dim.Render(IconError+" "+m.heading.Title) + Dim.Render(" (canceled)")
	case m.remaining <= 0 && m.heading.Changes > 0:
		return m.changesOutcome()
	case m.remaining <= 0:
		line := fmt.Sprintf("%s %s in %s", m.heading.Done, repositories(total), formatSeconds(m.lastReport.Sub(m.start)))
		if failed > 0 {
			return Red.Render(IconError) + " " + line + ", " + Red.Render(fmt.Sprintf("%d %s", failed, m.heading.Failed))
		}
		return Green.Render(IconSuccess) + " " + line
	}

	line := fmt.Sprintf("%s  %s  %s  %s", m.heading.Title, progressBar(finished, total, progressBarWidth), m.progressCount(finished, total), Dim.Render(formatSeconds(m.now().Sub(m.start))))
	more := m.gauge()
	if queued > 0 {
		more = append(more, Dim.Render(fmt.Sprintf("%d queued", queued)))
	}
	if len(more) > 0 {
		line += Dim.Render("  · ") + strings.Join(more, Dim.Render(" · "))
	}
	return line
}

// progressCount is how far the heading is, as "2/5". A heading that counts
// repositories says so ("2/5 repositories"), since its title does not;
// one that counts changes names them in its title ("Applying 6 changes").
func (m refreshModel) progressCount(done, total int) string {
	if m.heading.Changes > 0 {
		return fmt.Sprintf("%d/%d", done, total)
	}
	return fmt.Sprintf("%d/%s", done, repositories(total))
}

// progress returns how much is done of how much: the steps of the tasks
// when the heading counts changes, the finished rows otherwise.
func (m refreshModel) progress() (done, total int) {
	if m.heading.Changes > 0 {
		for _, item := range m.items {
			done += item.applied + item.failed + item.skipped
		}
		return done, m.heading.Changes
	}
	return m.finishedCount(), len(m.items)
}

// changesOutcome is the heading once every task has finished, counting
// changes: "✓ Applied 13 changes in 2.8s", or with some not applied
// "✗ Applied 12 of 13 changes in 2.8s, 1 failed" (✓ when only skipped).
func (m refreshModel) changesOutcome() string {
	applied, failed, skipped := 0, 0, 0
	for _, item := range m.items {
		applied += item.applied
		failed += item.failed
		skipped += item.skipped
	}
	total := m.heading.Changes
	took := formatSeconds(m.lastReport.Sub(m.start))
	if applied == total {
		return Green.Render(IconSuccess) + " " + fmt.Sprintf("%s %d %s in %s", m.heading.Done, total, pluralWord(total, "change", "changes"), took)
	}
	line := fmt.Sprintf("%s %d of %d %s in %s", m.heading.Done, applied, total, pluralWord(total, "change", "changes"), took)
	if failed > 0 {
		line += ", " + Red.Render(fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		line += fmt.Sprintf(", %d skipped", skipped)
	}
	icon := Green.Render(IconSuccess)
	if failed > 0 {
		icon = Red.Render(IconError)
	}
	return icon + " " + line
}

// finishedCount counts the rows whose reports are all in: one that errored
// at an early report still waits for the others.
func (m refreshModel) finishedCount() int {
	n := 0
	for _, item := range m.items {
		if item.status != taskRunning && item.status != taskCanceled && item.pending <= 0 {
			n++
		}
	}
	return n
}

// gauge describes the runner's gh processes: "gh 8/8", in yellow at the
// limit, and the calls waiting for one. Nothing without heading.Load.
func (m refreshModel) gauge() []string {
	if m.heading.Load == nil {
		return nil
	}
	running, waiting, limit := m.heading.Load()
	g := fmt.Sprintf("gh %d/%d", running, limit)
	if running >= limit {
		g = Yellow.Render(g)
	} else {
		g = Dim.Render(g)
	}
	parts := []string{g}
	if waiting > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("%d %s waiting", waiting, pluralWord(waiting, "call", "calls"))))
	}
	return parts
}

// progressBarWidth is the width of the heading's progress bar, and
// rowBarWidth that of a row's steps.
const (
	progressBarWidth = 24
	rowBarWidth      = 10
)

// progressBar draws done of total as a bar of width cells.
func progressBar(done, total, width int) string {
	filled := 0
	if total > 0 {
		filled = done * width / total
	}
	return Cyan.Render(strings.Repeat("━", filled)) + Dim.Render(strings.Repeat("─", width-filled))
}

// formatSeconds formats d as in "2.1s".
func formatSeconds(d time.Duration) string {
	return fmt.Sprintf("%.1fs", max(d, 0).Seconds())
}

// repositories returns n with "repository" or "repositories".
func repositories(n int) string {
	return fmt.Sprintf("%d %s", n, pluralWord(n, "repository", "repositories"))
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
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
	case MarkFailed:
		return Red.Render(IconError), Bold, plain
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
//
// The heading is drawn above the rows. Without a terminal, or without tasks,
// its Phase is printed instead, followed by a blank line, as the static line
// it replaces.
func RunRefresh(heading RefreshHeading, tasks []RefreshTask) *RefreshTracker {
	printPhase := func() {
		if heading.Phase != "" {
			DefaultPrinter.Phase(heading.Phase)
			DefaultPrinter.BlankLine()
		}
	}
	if len(tasks) == 0 {
		printPhase()
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	// When logging is active, spinners would interleave with log lines and
	// produce unreadable output. Fall back to plain-text progress instead.
	f, ok := DefaultPrinter.ErrWriter().(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) || logger.Enabled() {
		printPhase()
		for _, task := range tasks {
			DefaultPrinter.Progress(task.Name + "...")
		}
		return &RefreshTracker{fallback: true, done: closedChan()}
	}

	canceled := make(chan struct{})
	model := newRefreshModel(tasks)
	model.heading = heading
	// Compact from the start: print the title as a static line before the
	// display starts. A tea.Println sent before the first frame is drawn
	// lands at the top of the screen, and that frame then erases what is
	// below it; the switch during a run prints through tea.Println.
	if model.startCompact() {
		DefaultPrinter.Phase(heading.Title)
		DefaultPrinter.BlankLine()
	}
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

// Interactive reports whether the spinner draws in a terminal, rather than
// its fallback (no terminal, logging, or no tasks), which shows only the
// task names as they start.
func (t *RefreshTracker) Interactive() bool {
	return t != nil && !t.fallback && t.program != nil
}

// Step reports how one step of a task ended (see RefreshTask.Total).
func (t *RefreshTracker) Step(name string, result StepResult) {
	if t == nil || t.fallback {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.program != nil {
		t.program.Send(taskStepMsg{name: name, result: result})
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
