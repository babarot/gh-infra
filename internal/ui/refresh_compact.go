package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// When there are many rows, or they do not fit the terminal, the spinner
// switches to its compact view, which keeps a fixed height. A view taller
// than the terminal cannot be redrawn by bubbletea's inline renderer: its
// top lines scroll away and leave broken copies behind.
//
// In the compact view, each finished row with something to show (a change,
// a result, an error) is printed above the view with tea.Println, in the
// order the rows finish; printed lines stay in the scrollback and never move
// again. Rows with nothing to change are only counted. The view holds the
// running rows (at most maxRunningRows), what is left, and the progress.
//
// The switch is one way: printed lines cannot be taken back, so the view
// stays compact when the terminal grows.

const (
	// compactAt is the number of rows from which the view is compact.
	compactAt = 11
	// maxRunningRows is how many running rows the compact view shows at most.
	maxRunningRows = 5
)

// wantCompact reports whether the view should be compact: many rows, or a
// full view taller than the terminal.
func (m refreshModel) wantCompact() bool {
	if len(m.items) >= compactAt {
		return true
	}
	return m.height > 0 && frameHeight(m.fullView()) > m.height
}

// frameHeight is how many terminal lines bubbletea gives a view: one per
// line break and one more, so a view ending with "\n" takes a line below
// its text. A frame taller than the terminal loses its top lines.
func frameHeight(view string) int {
	return strings.Count(view, "\n") + 1
}

// startCompact makes a new model compact when it should be from the start,
// and reports whether its title is to be printed before the display.
func (m *refreshModel) startCompact() bool {
	m.compact = m.wantCompact()
	return m.compact && m.heading.Title != ""
}

// settle switches to the compact view when it is wanted, queues what the
// compact view no longer holds (the heading's title when switching, and the
// finished rows not printed yet), and returns the command that prints it, or
// ends the display once every row has finished and all is printed.
func (m *refreshModel) settle() tea.Cmd {
	if !m.compact && m.wantCompact() {
		m.compact = true
		if m.heading.Title != "" {
			m.queue = append(m.queue, m.heading.Title+"\n")
		}
	}
	if m.compact {
		maxName := m.maxNameWidth()
		for _, i := range m.finished {
			item := &m.items[i]
			if item.printed || !shownAbove(*item) {
				continue
			}
			item.printed = true
			m.queue = append(m.queue, m.rowLine(*item, maxName))
		}
	}
	return m.flush()
}

// stopCompact queues the rows that were running when the display was
// stopped, so each one's name is kept above the view; the compact view
// shows only a few of them.
func (m *refreshModel) stopCompact() {
	var rows []refreshItem
	for i := range m.items {
		item := &m.items[i]
		if item.status == taskCanceled && item.started && !item.printed {
			item.printed = true
			rows = append(rows, *item)
		}
	}
	slices.SortStableFunc(rows, func(a, b refreshItem) int { return a.begin.Compare(b.begin) })
	maxName := m.maxNameWidth()
	for _, item := range rows {
		m.queue = append(m.queue, m.rowLine(item, maxName))
	}
}

// printedMsg reports that a batch of queued lines has been printed.
type printedMsg struct{}

// flush prints the queued lines, or ends the display once there is nothing
// left to print and every row has finished (or the display was stopped).
//
// Commands returned by separate Updates run concurrently, so two batches of
// tea.Println could come out of order, and a tea.Quit could overtake a batch
// and drop its lines. One batch is in flight at a time: it is a tea.Sequence
// (run in order) that ends with printedMsg, and the next batch, or the
// quit, waits for it.
func (m *refreshModel) flush() tea.Cmd {
	if m.printing {
		return nil
	}
	if len(m.queue) > 0 {
		cmds := make([]tea.Cmd, 0, len(m.queue)+1)
		for _, line := range m.queue {
			cmds = append(cmds, tea.Println(line))
		}
		cmds = append(cmds, func() tea.Msg { return printedMsg{} })
		m.queue = nil
		m.printing = true
		return tea.Sequence(cmds...)
	}
	if m.remaining <= 0 || m.stopped {
		return tea.Quit
	}
	return nil
}

// shownAbove reports whether a finished row is printed above the compact
// view: every result but nothing to change, which is only counted.
func shownAbove(item refreshItem) bool {
	return item.status != taskDone || item.mark != MarkNone
}

// finish records that row i reached its final state.
func (m *refreshModel) finish(i int) {
	m.finished = append(m.finished, i)
}

// compactView is the view while it is compact: the running rows, what is
// left, and the progress (or the outcome once finished).
func (m refreshModel) compactView() string {
	maxName := m.maxNameWidth()
	var running []refreshItem
	upToDate, queued := 0, 0
	for _, item := range m.items {
		switch {
		case item.printed:
			// Already above the view (finished rows, and running rows once stopped).
		case item.status == taskRunning && item.started,
			item.status == taskCanceled && item.started:
			running = append(running, item)
		case item.status == taskRunning:
			queued++
		case item.status == taskDone && item.mark == MarkNone:
			upToDate++
		}
	}
	// Longest running first, as the rows started.
	slices.SortStableFunc(running, func(a, b refreshItem) int { return a.begin.Compare(b.begin) })

	var b strings.Builder
	shown := min(len(running), m.runningRows())
	for _, item := range running[:shown] {
		fmt.Fprintf(&b, "%s\n", m.rowLine(item, maxName))
	}

	var left []string
	if more := len(running) - shown; more > 0 {
		left = append(left, fmt.Sprintf("+%d more", more))
	}
	if queued > 0 {
		left = append(left, fmt.Sprintf("%d queued", queued))
	}
	if upToDate > 0 {
		left = append(left, fmt.Sprintf("%d up to date", upToDate))
	}
	switch {
	case m.remaining <= 0 && !m.stopped:
		if upToDate > 0 {
			fmt.Fprintf(&b, "  %s %s\n", Green.Render(IconSuccess), Dim.Render(fmt.Sprintf("%d up to date", upToDate)))
		}
	case len(left) > 0:
		fmt.Fprintf(&b, "  %s\n", Dim.Render(strings.Join(left, " · ")))
	}
	if m.notice != "" {
		fmt.Fprintf(&b, "  %s\n", Dim.Render(m.notice))
	}
	if !m.tight() {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s\n", m.compactProgress())
	return b.String()
}

// runningRows is how many running rows fit the compact view: at most
// maxRunningRows, fewer on a short terminal, at least one.
func (m refreshModel) runningRows() int {
	if m.height <= 0 {
		return maxRunningRows
	}
	return max(1, min(maxRunningRows, m.height-m.fixedLines()))
}

// fixedLines counts the compact view's lines besides the running rows: what
// is left, the notice, the blank line before the progress (left out when
// tight), the progress, and the line under the frame (frameHeight).
func (m refreshModel) fixedLines() int {
	n := 3 // what is left, the progress, the line under the frame
	if m.notice != "" {
		n++
	}
	if !m.tight() {
		n++
	}
	return n
}

// tight reports whether the terminal is too short for a running row and
// every other line of the compact view; the blank line is then left out.
func (m refreshModel) tight() bool {
	n := 5 // a running row, what is left, the blank line, the progress, the line under the frame
	if m.notice != "" {
		n++
	}
	return m.height > 0 && m.height < n
}

// compactProgress is the last line of the compact view: the progress with
// the gh gauge and the Ctrl+C hint while running, then the outcome. The
// title itself was printed above when the view became compact.
func (m refreshModel) compactProgress() string {
	if m.stopped || m.remaining <= 0 || m.heading.Title == "" {
		if m.heading.Title == "" {
			return ""
		}
		return m.headingLine()
	}
	finished, total := m.finishedCount(), len(m.items)
	line := fmt.Sprintf("  %s  %d/%d  %s", progressBar(finished, total, progressBarWidth), finished, total, Dim.Render(formatSeconds(m.now().Sub(m.start))))
	more := m.gauge()
	more = append(more, Dim.Render("ctrl+c to cancel"))
	return line + Dim.Render("  · ") + strings.Join(more, Dim.Render(" · "))
}
