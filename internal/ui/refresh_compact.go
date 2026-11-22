package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// When there are many rows, or they do not fit the terminal, the spinner
// switches to its compact view, which never grows taller than the terminal.
// A view taller than the terminal cannot be redrawn by bubbletea's inline
// renderer: its top lines scroll away and leave broken copies behind.
//
// The compact view keeps its heading (the title and the progress) as its
// first line, where it never moves: nothing is printed above the view while
// the rows run, and the view claims the height it can grow to in its first
// frame, so the terminal scrolls once, before anything has run. Under the
// heading it shows the latest finished rows with something to show (a
// change, a result, an error) and the running rows, at most
// maxCompactRows; rows with nothing to change are only counted. The last
// line counts what is left, with the gh gauge and how to stop.
//
// When every row has finished, or the display is stopped, the outcome of
// the heading and every finished row with something to show are printed
// above with tea.Println, in the order the rows finished, so all of them
// stay in the scrollback.
//
// The switch is one way: the view stays compact when the terminal grows.

const (
	// compactAt is the number of rows from which the view is compact.
	compactAt = 11
	// maxRunningRows is how many running rows the compact view shows at most.
	maxRunningRows = 5
	// maxCompactRows is how many rows, finished and running, the compact
	// view shows at most.
	maxCompactRows = 10
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

// startCompact makes a new model compact when it should be from the start.
func (m *refreshModel) startCompact() {
	m.compact = m.wantCompact()
}

// settle switches to the compact view when it is wanted, queues what the
// compact view leaves for the end once every row has finished, and returns
// the command that prints it, or ends the display when all is printed.
func (m *refreshModel) settle() tea.Cmd {
	if !m.compact && m.wantCompact() {
		m.compact = true
	}
	if m.compact && m.remaining <= 0 {
		m.endCompact()
	}
	return m.flush()
}

// endCompact queues, once, what the compact view printed nowhere: the
// heading's outcome, then every finished row with something to show in the
// order the rows finished, then (when stopped) the rows that were running,
// as they started.
func (m *refreshModel) endCompact() {
	if m.ended {
		return
	}
	m.ended = true
	if m.heading.Title != "" {
		// tea.Println drops an empty line; the line break makes the blank
		// line under the outcome.
		m.queue = append(m.queue, m.headingLine()+"\n")
	}
	maxName := m.maxNameWidth()
	for _, i := range m.finished {
		item := &m.items[i]
		if item.printed || !shownAbove(*item) {
			continue
		}
		item.printed = true
		m.queue = append(m.queue, m.rowLine(*item, maxName))
	}
	var rows []refreshItem
	for i := range m.items {
		item := &m.items[i]
		if item.status == taskCanceled && item.started && !item.printed {
			item.printed = true
			rows = append(rows, *item)
		}
	}
	slices.SortStableFunc(rows, func(a, b refreshItem) int { return a.begin.Compare(b.begin) })
	for _, item := range rows {
		m.queue = append(m.queue, m.rowLine(item, maxName))
	}
}

// printedMsg reports that a batch of queued lines has been printed.
type printedMsg struct{}

// drawnMsg reports that the view has had time to be drawn (endFrameWait).
type drawnMsg struct{}

// endFrameWait is how long the end waits before printing: a few frames at
// bubbletea's default 60 fps.
const endFrameWait = 50 * time.Millisecond

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
		cmds := make([]tea.Cmd, 0, len(m.queue)+2)
		// The queue is what the compact view prints at its end, with the
		// view now a line or two. bubbletea inserts printed lines above the
		// last frame it drew; were that still the full-height frame, the
		// lines could push it past the bottom and scroll the terminal for
		// nothing. Wait for the short frame to be drawn first.
		cmds = append(cmds, func() tea.Msg {
			time.Sleep(endFrameWait)
			return drawnMsg{}
		})
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

// shownAbove reports whether a finished row is shown in the compact view
// and printed at its end: every result but nothing to change, which is only
// counted.
func shownAbove(item refreshItem) bool {
	return item.status != taskDone || item.mark != MarkNone
}

// finish records that row i reached its final state.
func (m *refreshModel) finish(i int) {
	m.finished = append(m.finished, i)
}

// compactView is the view while it is compact: the heading, the latest
// finished rows and the running rows, the notice and what is left, filled
// up to the height the view can grow to. Once ended, what was not printed:
// how many rows were up to date (or, when stopped, were left).
func (m refreshModel) compactView() string {
	var lines []string
	if m.ended {
		lines = m.endLines()
	} else {
		lines = m.compactLines()
		for len(lines) < m.compactHeight() {
			lines = append(lines, "")
		}
	}
	// On a terminal too short for even the tight view, keep the top lines,
	// the heading first: a frame taller than the terminal loses its top.
	if m.height > 0 && len(lines) > m.height-1 {
		lines = lines[:max(0, m.height-1)]
	}
	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, "%s\n", line)
	}
	return b.String()
}

// compactLines are the compact view's lines while the rows run.
func (m refreshModel) compactLines() []string {
	maxName := m.maxNameWidth()
	var running, shown []refreshItem
	upToDate, queued := 0, 0
	for _, item := range m.items {
		switch {
		case item.status == taskRunning && item.started:
			running = append(running, item)
		case item.status == taskRunning:
			queued++
		case item.status == taskDone && item.mark == MarkNone:
			upToDate++
		}
	}
	for _, i := range m.finished {
		if shownAbove(m.items[i]) {
			shown = append(shown, m.items[i])
		}
	}
	// Longest running first, as the rows started.
	slices.SortStableFunc(running, func(a, b refreshItem) int { return a.begin.Compare(b.begin) })

	room := m.compactRows()
	nRunning := min(len(running), maxRunningRows, room)
	shown = shown[max(0, len(shown)-(room-nRunning)):]

	var lines []string
	if m.heading.Title != "" {
		lines = append(lines, m.compactHeading())
		if !m.tight() {
			lines = append(lines, "")
		}
	}
	for _, item := range shown {
		lines = append(lines, m.rowLine(item, maxName))
	}
	for _, item := range running[:nRunning] {
		lines = append(lines, m.rowLine(item, maxName))
	}
	if len(shown)+nRunning > 0 && !m.tight() {
		lines = append(lines, "")
	}
	if m.notice != "" {
		lines = append(lines, "  "+Yellow.Render(m.notice))
	}

	var parts []string
	if more := len(running) - nRunning; more > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("+%d more", more)))
	}
	if queued > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("%d queued", queued)))
	}
	if upToDate > 0 {
		parts = append(parts, Dim.Render(fmt.Sprintf("%d up to date", upToDate)))
	}
	parts = append(parts, m.gauge()...)
	parts = append(parts, Dim.Render("ctrl+c to cancel"))
	return append(lines, "  "+strings.Join(parts, Dim.Render(" · ")))
}

// endLines are the compact view's last lines, under what endCompact printed.
func (m refreshModel) endLines() []string {
	upToDate, queued := 0, 0
	for _, item := range m.items {
		switch {
		case item.status == taskDone && item.mark == MarkNone:
			upToDate++
		case item.status == taskCanceled && !item.started:
			queued++
		}
	}
	if !m.stopped {
		if upToDate == 0 {
			return nil
		}
		return []string{fmt.Sprintf("  %s %s", Green.Render(IconSuccess), Dim.Render(fmt.Sprintf("%d up to date", upToDate)))}
	}
	var left []string
	if queued > 0 {
		left = append(left, fmt.Sprintf("%d queued", queued))
	}
	if upToDate > 0 {
		left = append(left, fmt.Sprintf("%d up to date", upToDate))
	}
	if len(left) == 0 {
		return nil
	}
	return []string{"  " + Dim.Render(strings.Join(left, " · "))}
}

// compactHeading is the compact view's first line: the title with the
// progress, as the full view's heading without the gh gauge, which is on
// the last line.
func (m refreshModel) compactHeading() string {
	finished, total := m.progress()
	return fmt.Sprintf("%s  %s  %s  %s", m.heading.Title, progressBar(finished, total, progressBarWidth), m.progressCount(finished, total), Dim.Render(formatSeconds(m.now().Sub(m.start))))
}

// compactRows is how many rows the compact view shows: at most
// maxCompactRows, fewer on a short terminal, at least one.
func (m refreshModel) compactRows() int {
	if m.height <= 0 {
		return maxCompactRows
	}
	return max(1, min(maxCompactRows, m.height-m.fixedLines()))
}

// compactHeight is the height the compact view can grow to, which it takes
// from its first frame: every line but the rows, a line for a notice
// whether one is shown or not, and as many rows as there are, up to
// compactRows.
func (m refreshModel) compactHeight() int {
	return m.fixedLines() - 1 + min(len(m.items), m.compactRows())
}

// fixedLines counts the compact view's lines besides the rows: the heading
// and the blank line under it, the blank line under the rows (both left out
// when tight), a notice, what is left, and the line under the frame
// (frameHeight).
func (m refreshModel) fixedLines() int {
	n := 3 // a notice, what is left, the line under the frame
	if m.heading.Title != "" {
		n++
	}
	if !m.tight() {
		n += 2
	}
	return n
}

// tight reports whether the terminal is too short for a row and every
// other line of the compact view; the blank lines are then left out.
func (m refreshModel) tight() bool {
	n := 7 // the heading, two blank lines, a row, a notice, what is left, the line under the frame
	if m.heading.Title == "" {
		n--
	}
	return m.height > 0 && m.height < n
}
