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

func TestCompact_StartsWithAStaticTitle(t *testing.T) {
	m := frozen(newRefreshModel(manyTasks(11)))
	m.heading = RefreshHeading{Title: "Fetching current state from GitHub API"}
	if !m.startCompact() {
		t.Error("compact from the start with a title: RunRefresh prints the title before the display")
	}
	m = frozen(newRefreshModel(manyTasks(3)))
	m.heading = RefreshHeading{Title: "Fetching current state from GitHub API"}
	if m.startCompact() {
		t.Error("3 rows: the title stays in the view")
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
	if got := printed(t, cmd); len(got) == 0 || !strings.Contains(got[0], "Fetching current state from GitHub API") {
		t.Errorf("switching prints the heading's title above the view, got %q", got)
	}
	fits(t, m, 6, "after shrinking to 6 lines")
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if !m.compact {
		t.Error("growing back: the printed lines cannot be taken back, so the view stays compact")
	}
}

func TestCompact_FinishedRowsArePrintedAbove(t *testing.T) {
	m := begun(t, compactModel(t, 12, 20), "o/r01", "o/r02", "o/r03")
	m = later(m, 1500*time.Millisecond)

	m, cmd := updateCmd(t, m, taskDoneMsg{name: "o/r02", labeled: true, mark: MarkChange, label: "2 changes"})
	if got := printed(t, cmd); len(got) != 2 || !strings.Contains(got[0], "~ o/r02   1.5s  2 changes") || got[1] != "ACK" {
		t.Errorf("a row with changes is printed above, got %q", got)
	}
	m = update(t, m, printedMsg{})

	m, cmd = updateCmd(t, m, taskDoneMsg{name: "o/r01", labeled: true, mark: MarkNone, label: "up to date"})
	if got := printed(t, cmd); len(got) != 0 {
		t.Errorf("a row with nothing to change is only counted, got %q", got)
	}

	m, cmd = updateCmd(t, m, taskErrorMsg{name: "o/r03", err: fmt.Errorf("HTTP 403")})
	if got := printed(t, cmd); len(got) != 2 || !strings.Contains(got[0], "✗ o/r03") {
		t.Errorf("an error is printed above, got %q", got)
	}
	m = update(t, m, printedMsg{})

	if got := viewLines(m); !strings.Contains(strings.Join(got, "\n"), "9 queued · 1 up to date") {
		t.Errorf("the view counts what is left:\n%s", strings.Join(got, "\n"))
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
	if len(got) != 2 || !strings.Contains(got[0], "~ o/r11") || got[1] != "ACK" {
		t.Errorf("want the last row printed; got %q", got)
	}
	m, cmd = updateCmd(t, m, printedMsg{})
	if got := printed(t, cmd); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("once printed, want the end of the display; got %q", got)
	}
	view := strings.Join(viewLines(later(m, 2*time.Second)), "\n")
	for _, want := range []string{"✓ 10 up to date", "✓ Fetched 11 repositories in 0.0s"} {
		if !strings.Contains(view, want) {
			t.Errorf("final view: want %q in\n%s", want, view)
		}
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
	if !strings.Contains(view, "0/20") || !strings.Contains(view, "ctrl+c to cancel") {
		t.Errorf("want the progress with the hint as the last line:\n%s", view)
	}
}

// Commands from separate Updates run concurrently in bubbletea, so prints
// are sent one batch at a time, each a tea.Sequence ending with printedMsg:
// the next batch, and the end of the display, wait for it.
func TestCompact_PrintsInOrderAndQuitsAfterThem(t *testing.T) {
	m := begun(t, compactModel(t, 11, 20), "o/r01", "o/r02")
	m, first := updateCmd(t, m, taskDoneMsg{name: "o/r01", labeled: true, mark: MarkChange, label: "1 change"})
	if got := fmt.Sprintf("%T", first()); got != "tea.sequenceMsg" {
		t.Fatalf("a batch must run in order (tea.Sequence), got %s", got)
	}

	// o/r02 finishes while the first batch is still being printed.
	m, second := updateCmd(t, m, taskDoneMsg{name: "o/r02", labeled: true, mark: MarkChange, label: "1 change"})
	if second != nil {
		t.Fatalf("a second batch must wait for the first, got %q", printed(t, second))
	}
	for _, task := range manyTasks(11)[2:] {
		m = update(t, begun(t, m, task.Name), taskDoneMsg{name: task.Name, labeled: true, mark: MarkNone, label: "up to date"})
	}

	m, next := updateCmd(t, m, printedMsg{})
	if got := printed(t, next); len(got) != 2 || !strings.Contains(got[0], "o/r02") || got[1] != "ACK" {
		t.Fatalf("after the first batch, want o/r02 printed (not the end yet); got %q", got)
	}
	_, last := updateCmd(t, m, printedMsg{})
	if got := printed(t, last); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("after the last batch, want the end of the display; got %q", got)
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
	if len(got) != 12 || got[11] != "ACK" {
		t.Fatalf("want the 11 canceled rows printed; got %q", got)
	}
	for i, line := range got[:11] {
		if !strings.Contains(line, names[i]+" (canceled)") {
			t.Errorf("line %d: want %s canceled, got %q", i, names[i], line)
		}
	}
	m, cmd = updateCmd(t, m, printedMsg{})
	if got := printed(t, cmd); len(got) != 1 || got[0] != "QUIT" {
		t.Errorf("then the end of the display; got %q", got)
	}
	view := strings.Join(viewLines(m), "\n")
	if strings.Contains(view, "o/r") {
		t.Errorf("the printed rows leave the view:\n%s", view)
	}
	if !strings.Contains(view, "Fetching current state from GitHub API (canceled)") {
		t.Errorf("the last line says it was canceled:\n%s", view)
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
	if got := printed(t, cmd); len(got) != 3 || got[2] != "ACK" {
		t.Fatalf("first Ctrl+C: want the two running rows printed, got %q", got)
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
