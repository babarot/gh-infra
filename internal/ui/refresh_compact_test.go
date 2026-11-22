package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// printed runs cmd and returns what it prints above the view, in order,
// with "ACK" for printedMsg and "QUIT" where it ends the display.
// tea.Sequence and tea.Batch are expanded: they produce a slice of commands
// (unexported for Sequence).
func printed(t *testing.T, cmd tea.Cmd) []string {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg.(type) {
	case tea.QuitMsg:
		return []string{"QUIT"}
	case printedMsg:
		return []string{"ACK"}
	case drawnMsg:
		return nil
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice {
		var out []string
		for i := range v.Len() {
			if c, ok := reflect.TypeAssert[tea.Cmd](v.Index(i)); ok {
				out = append(out, printed(t, c)...)
			}
		}
		return out
	}
	// tea.Println's message is unexported; its text shows in fmt.Sprint.
	return []string{stripANSIForTest(fmt.Sprint(msg))}
}

func manyTasks(n int) []RefreshTask {
	tasks := make([]RefreshTask, n)
	for i := range tasks {
		tasks[i] = RefreshTask{Name: fmt.Sprintf("o/r%02d", i+1)}
	}
	return tasks
}

// compactModel is a model of n rows with a heading and a frozen clock, set
// up as RunRefresh does, on a terminal of the given height.
func compactModel(t *testing.T, n, height int) refreshModel {
	t.Helper()
	m := frozen(newRefreshModel(manyTasks(n)))
	m.heading = RefreshHeading{Title: "Fetching current state from GitHub API", Done: "Fetched", Failed: "skipped"}
	m.height = height
	m.startCompact()
	return m
}

// fits fails the test when m's frame is taller than height, measured as
// bubbletea measures it (frameHeight).
func fits(t *testing.T, m refreshModel, height int, what string) {
	t.Helper()
	view := m.View().Content
	if h := frameHeight(view); h > height {
		t.Errorf("%s: a %d-line frame on a %d-line terminal:\n%s", what, h, height, view)
	}
}

func TestCompact_SwitchesForManyRowsOrAShortTerminal(t *testing.T) {
	if !compactModel(t, 11, 0).compact {
		t.Error("11 rows: want compact")
	}
	if compactModel(t, 10, 0).compact || compactModel(t, 10, 40).compact {
		t.Error("10 rows on an unknown or tall terminal: want the full view")
	}
	if !compactModel(t, 5, 8).compact {
		t.Error("5 rows on an 8-line terminal (the full frame needs 10): want compact")
	}
	// The full frame of 10 rows is 15 lines (heading 2, rows 10, hint 2, and
	// the line under the frame): it fits 15 lines, not 14.
	if compactModel(t, 10, 15).compact || !compactModel(t, 10, 14).compact {
		t.Error("10 rows: want the full view on 15 lines and compact on 14")
	}
	m := compactModel(t, 10, 15)
	fits(t, m, 15, "10 rows, full view")
}

// The heading is the compact view's first line from the start; nothing is
// printed before the display.
func TestCompact_HeadingIsTheFirstLine(t *testing.T) {
	m := compactModel(t, 11, 30)
	if !m.compact {
		t.Fatal("11 rows: want compact")
	}
	lines := viewLines(m)
	if !strings.HasPrefix(lines[0], "Fetching current state from GitHub API") || !strings.Contains(lines[0], "0/11 repositories") {
		t.Errorf("first line: want the title with the progress, got %q", lines[0])
	}
	if !strings.Contains(strings.Join(lines, "\n"), "ctrl+c to cancel") {
		t.Errorf("want the hint under the rows:\n%s", strings.Join(lines, "\n"))
	}
}

func TestCompact_ViewFitsTheTerminal(t *testing.T) {
	var first10 []string
	for _, task := range manyTasks(10) {
		first10 = append(first10, task.Name)
	}
	// Five running rows need 9 lines (rows 5, what is left, a blank line,
	// the progress, the line under the frame); with a notice, 10. Below
	// that the rows shrink, down to one, and then the blank line goes: a
	// terminal of 5 lines still fits (4 without a notice).
	for _, height := range []int{5, 6, 7, 8, 9, 10, 12, 40} {
		m := begun(t, compactModel(t, 40, height), first10...)
		fits(t, m, height, fmt.Sprintf("height %d", height))
		m = update(t, m, noticeMsg{text: "waiting for the rate limit"})
		fits(t, m, height, fmt.Sprintf("height %d with a notice", height))
	}
}

func TestCompact_ResizeSwitchesOneWay(t *testing.T) {
	m := compactModel(t, 5, 40)
	if m.compact {
		t.Fatal("5 rows on a 40-line terminal: want the full view")
	}
	m, cmd := updateCmd(t, m, tea.WindowSizeMsg{Width: 100, Height: 6})
	if !m.compact {
		t.Fatal("after shrinking to 6 lines: want compact")
	}
	if got := printed(t, cmd); len(got) != 0 {
		t.Errorf("switching prints nothing; the heading stays in the view, got %q", got)
	}
	fits(t, m, 6, "after shrinking to 6 lines")
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !m.compact {
		t.Error("growing back: the view stays compact")
	}
}

// While the rows run, nothing is printed above the view: finished rows with
// something to show stay in it, under the heading.
func TestCompact_FinishedRowsStayInTheView(t *testing.T) {
	m := begun(t, compactModel(t, 12, 30), "o/r01", "o/r02", "o/r03")
	m = later(m, 1500*time.Millisecond)

	m, cmd := updateCmd(t, m, taskDoneMsg{name: "o/r02", labeled: true, mark: MarkChange, label: "2 changes"})
	if got := printed(t, cmd); len(got) != 0 {
		t.Errorf("nothing is printed while the rows run, got %q", got)
	}
	m, cmd = updateCmd(t, m, taskDoneMsg{name: "o/r01", labeled: true, mark: MarkNone, label: "up to date"})
	if got := printed(t, cmd); len(got) != 0 {
		t.Errorf("nothing is printed while the rows run, got %q", got)
	}
	m, cmd = updateCmd(t, m, taskErrorMsg{name: "o/r03", err: fmt.Errorf("HTTP 403")})
	if got := printed(t, cmd); len(got) != 0 {
		t.Errorf("nothing is printed while the rows run, got %q", got)
	}

	view := strings.Join(viewLines(m), "\n")
	for _, want := range []string{"~ o/r02   1.5s  2 changes", "✗ o/r03", "9 queued · 1 up to date"} {
		if !strings.Contains(view, want) {
			t.Errorf("want %q in the view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "o/r01") {
		t.Errorf("a row with nothing to change is only counted:\n%s", view)
	}
}

// The heading stays the first line and the frame keeps its height from the
// first frame to the last one before the end, through rows starting and
// finishing and a notice coming and going: in the terminal, the heading
// never moves.
func TestCompact_HeadingNeverMoves(t *testing.T) {
	for _, height := range []int{8, 12, 30} {
		m := compactModel(t, 16, height)
		height0 := frameHeight(m.View().Content)
		check := func(what string) {
			t.Helper()
			view := m.View().Content
			if h := frameHeight(view); h != height0 {
				t.Errorf("height %d, %s: frame of %d lines, want %d:\n%s", height, what, h, height0, view)
			}
			if h := frameHeight(view); h > height {
				t.Errorf("height %d, %s: frame of %d lines on a %d-line terminal", height, what, h, height)
			}
			if first := viewLines(m)[0]; !strings.HasPrefix(first, "Fetching current state from GitHub API") {
				t.Errorf("height %d, %s: first line %q, want the heading", height, what, first)
			}
		}
		check("start")
		tasks := manyTasks(16)
		for _, task := range tasks {
			m = begun(t, m, task.Name)
			check("started " + task.Name)
		}
		for i, task := range tasks[:15] {
			if i == 4 {
				m = update(t, m, noticeMsg{text: "Waiting for the GitHub API rate limit until 10:42:00 ..."})
				check("notice")
			}
			if i == 9 {
				m = update(t, m, noticeMsg{text: ""})
				check("notice gone")
			}
			mark, label := MarkNone, "up to date"
			if i%3 == 0 {
				mark, label = MarkChange, "1 change"
			}
			m = update(t, m, taskDoneMsg{name: task.Name, labeled: true, mark: mark, label: label})
			check("done " + task.Name)
		}
	}
}

// When the rows outnumber the view, it shows the latest finished ones; the
// end prints every one of them.
func TestCompact_ShowsTheLatestFinishedRows(t *testing.T) {
	m := compactModel(t, 30, 40)
	tasks := manyTasks(30)
	for _, task := range tasks {
		m = begun(t, m, task.Name)
	}
	for _, task := range tasks[:12] {
		m = update(t, m, taskDoneMsg{name: task.Name, labeled: true, mark: MarkChange, label: "1 change"})
	}
	view := strings.Join(viewLines(m), "\n")
	// 10 rows: the 5 running rows that started first, and the 5 latest finished.
	if strings.Contains(view, "o/r07 ") || !strings.Contains(view, "o/r08 ") || !strings.Contains(view, "o/r12 ") {
		t.Errorf("want o/r08 to o/r12 finished in the view, not o/r07:\n%s", view)
	}
	if !strings.Contains(view, "o/r13 ") || strings.Contains(view, "o/r18 ") || !strings.Contains(view, "+13 more") {
		t.Errorf("want five running rows and the rest counted:\n%s", view)
	}

	var cmd tea.Cmd
	for _, task := range tasks[12:] {
		m, cmd = updateCmd(t, m, taskDoneMsg{name: task.Name, labeled: true, mark: MarkNone, label: "up to date"})
	}
	got := printed(t, cmd)
	if len(got) != 14 || !strings.Contains(got[0], "Fetched 30 repositories") || !strings.HasSuffix(got[0], "\n}") || got[13] != "ACK" {
		t.Fatalf("want the outcome with a blank line, the 12 rows and ACK; got %q", got)
	}
	for i, line := range got[1:13] {
		if want := fmt.Sprintf("o/r%02d", i+1); !strings.Contains(line, want) {
			t.Errorf("line %d: want %s, in the order they finished; got %q", i, want, line)
		}
	}
}

func TestCompact_LastRowIsPrintedBeforeTheDisplayEnds(t *testing.T) {
	m := compactModel(t, 11, 20)
	var cmd tea.Cmd
	for i, task := range manyTasks(11) {
		m = begun(t, m, task.Name)
		mark, label := MarkNone, "up to date"
		if i == 10 {
			mark, label = MarkChange, "1 change"
		}
		m, cmd = updateCmd(t, m, taskDoneMsg{name: task.Name, labeled: true, mark: mark, label: label})
	}
	got := printed(t, cmd)
	if len(got) != 3 || !strings.Contains(got[0], "✓ Fetched 11 repositories in 0.0s") || !strings.Contains(got[1], "~ o/r11") || got[2] != "ACK" {
		t.Errorf("want the outcome, a blank line and the row with a change printed; got %q", got)
	}
	m, cmd = updateCmd(t, m, printedMsg{})
	if got := printed(t, cmd); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("once printed, want the end of the display; got %q", got)
	}
	view := strings.Join(viewLines(later(m, 2*time.Second)), "\n")
	if view != "  ✓ 10 up to date" {
		t.Errorf("final view: want only what was up to date, got\n%s", view)
	}
	if strings.Contains(view, "ctrl+c") {
		t.Errorf("final view: no hint once finished\n%s", view)
	}
}

func TestCompact_RunningRows(t *testing.T) {
	m := compactModel(t, 20, 40)
	// Seven rows start, one at a time; the view keeps the five that started first.
	for i, task := range manyTasks(7) {
		m = later(m, time.Duration(i)*time.Second)
		m = begun(t, m, task.Name)
	}
	view := strings.Join(viewLines(m), "\n")
	for _, name := range []string{"o/r01", "o/r05"} {
		if !strings.Contains(view, name) {
			t.Errorf("want %s among the running rows:\n%s", name, view)
		}
	}
	if strings.Contains(view, "o/r06") || !strings.Contains(view, "+2 more · 13 queued") {
		t.Errorf("want five running rows and the rest counted:\n%s", view)
	}
	lines := viewLines(m)
	if !strings.Contains(lines[0], "0/20 repositories") {
		t.Errorf("want the progress on the first line:\n%s", view)
	}
	if !strings.Contains(view, "+2 more · 13 queued · ctrl+c to cancel") {
		t.Errorf("want the hint on the line that counts what is left:\n%s", view)
	}
}

// Commands from separate Updates run concurrently in bubbletea, so the end
// prints its lines as one tea.Sequence ending with printedMsg, and the end
// of the display waits for it.
func TestCompact_PrintsInOrderAndQuitsAfterThem(t *testing.T) {
	m := compactModel(t, 11, 20)
	var cmd tea.Cmd
	for _, task := range manyTasks(11) {
		m, cmd = updateCmd(t, begun(t, m, task.Name), taskDoneMsg{name: task.Name, labeled: true, mark: MarkChange, label: "1 change"})
	}
	if got := fmt.Sprintf("%T", cmd()); got != "tea.sequenceMsg" {
		t.Fatalf("the end must print in order (tea.Sequence), got %s", got)
	}
	m, again := updateCmd(t, m, noticeMsg{text: "late"})
	if again != nil {
		t.Fatalf("nothing more is printed while the end prints, got %q", printed(t, again))
	}
	_, last := updateCmd(t, m, printedMsg{})
	if got := printed(t, last); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("after the end is printed, want the end of the display; got %q", got)
	}
}

// Stopped in the compact view, every row that was running keeps its name
// above, not only the few the view shows.
func TestCompact_StopPrintsTheRunningRows(t *testing.T) {
	m := compactModel(t, 11, 20)
	var names []string
	for _, task := range manyTasks(11) {
		names = append(names, task.Name)
	}
	m = begun(t, m, names...)

	m, cmd := updateCmd(t, m, interruptMsg{})
	got := printed(t, cmd)
	if len(got) != 13 || got[12] != "ACK" {
		t.Fatalf("want the outcome and the 11 canceled rows printed; got %q", got)
	}
	if !strings.Contains(got[0], "Fetching current state from GitHub API (canceled)") || !strings.HasSuffix(got[0], "\n}") {
		t.Errorf("want the canceled heading with a blank line first; got %q", got[0])
	}
	for i, line := range got[1:12] {
		if !strings.Contains(line, names[i]+" (canceled)") {
			t.Errorf("line %d: want %s canceled, got %q", i, names[i], line)
		}
	}
	m, cmd = updateCmd(t, m, printedMsg{})
	if got := printed(t, cmd); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("then the end of the display; got %q", got)
	}
	view := strings.Join(viewLines(m), "\n")
	if strings.Contains(view, "o/r") || strings.Contains(view, "Fetching") {
		t.Errorf("the printed lines leave the view:\n%s", view)
	}
}

// Stopping waits for the running rows to be printed; another Ctrl+C in that
// time is ignored rather than closing the canceled channel twice.
func TestCompact_SecondCtrlCWhileStopping(t *testing.T) {
	restore := SetInterruptHandler(func() {})
	defer restore()
	m := begun(t, compactModel(t, 11, 20), "o/r01", "o/r02")
	m.canceled = make(chan struct{})
	key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

	m, cmd := updateCmd(t, m, key)
	if got := printed(t, cmd); len(got) != 4 || got[3] != "ACK" {
		t.Fatalf("first Ctrl+C: want the outcome and the two running rows printed, got %q", got)
	}
	m, cmd = updateCmd(t, m, key) // would panic on a second close
	if cmd != nil {
		t.Errorf("second Ctrl+C: want nothing more, got %q", printed(t, cmd))
	}
	_, cmd = updateCmd(t, m, printedMsg{})
	if got := printed(t, cmd); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("then the end of the display; got %q", got)
	}
}
