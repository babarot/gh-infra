package ui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// refreshModel message handling (unit tests for Update logic)
// ---------------------------------------------------------------------------

// helper: create a refreshModel with given task names and pending counts.
func makeModel(tasks ...struct {
	name    string
	pending int
}) refreshModel {
	var rt []RefreshTask
	for _, t := range tasks {
		rt = append(rt, RefreshTask{Name: t.name, Pending: t.pending})
	}
	return newRefreshModel(rt)
}

func task(name string, pending int) struct {
	name    string
	pending int
} {
	return struct {
		name    string
		pending int
	}{name, pending}
}

// updateCmd is like update but also returns the command Update gave.
func updateCmd(t *testing.T, m refreshModel, msg any) (refreshModel, tea.Cmd) {
	t.Helper()
	result, cmd := m.Update(msg)
	rm, ok := result.(refreshModel)
	if !ok {
		t.Fatalf("Update returned %T, want refreshModel", result)
	}
	return rm, cmd
}

// update is a test helper that calls Update, asserts the result type, and returns it.
func update(t *testing.T, m refreshModel, msg any) refreshModel {
	t.Helper()
	result, cmd := m.Update(msg)
	_ = cmd // tea.Cmd is not exercised in unit tests
	rm, ok := result.(refreshModel)
	if !ok {
		t.Fatalf("Update returned %T, want refreshModel", result)
	}
	return rm
}

func TestRefreshModel_DoneSimple(t *testing.T) {
	m := makeModel(task("a", 1))
	m = update(t, m, taskDoneMsg{name: "a"})

	if m.items[0].status != taskDone {
		t.Errorf("status = %v, want taskDone", m.items[0].status)
	}
	if m.remaining != 0 {
		t.Errorf("remaining = %d, want 0", m.remaining)
	}
}

func TestRefreshModel_DoneRefCounted(t *testing.T) {
	m := makeModel(task("a", 2))

	// First Done: should still be running
	m = update(t, m, taskDoneMsg{name: "a"})
	if m.items[0].status != taskRunning {
		t.Errorf("after 1st Done: status = %v, want taskRunning", m.items[0].status)
	}
	if m.remaining != 1 {
		t.Errorf("after 1st Done: remaining = %d, want 1", m.remaining)
	}

	// Second Done: should complete
	m = update(t, m, taskDoneMsg{name: "a"})
	if m.items[0].status != taskDone {
		t.Errorf("after 2nd Done: status = %v, want taskDone", m.items[0].status)
	}
	if m.remaining != 0 {
		t.Errorf("after 2nd Done: remaining = %d, want 0", m.remaining)
	}
}

func TestRefreshModel_ErrorThenDone(t *testing.T) {
	// Regression test: Error on one source, then Done on the other.
	// The tracker must not hang.
	m := makeModel(task("a", 2))

	// Error from first source
	m = update(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("fail")})
	if m.items[0].status != taskError {
		t.Errorf("after Error: status = %v, want taskError", m.items[0].status)
	}
	if m.remaining != 1 {
		t.Errorf("after Error: remaining = %d, want 1", m.remaining)
	}

	// Done from second source: should decrement remaining
	m = update(t, m, taskDoneMsg{name: "a"})
	if m.remaining != 0 {
		t.Errorf("after Done: remaining = %d, want 0", m.remaining)
	}
	// Status stays as taskError (first error wins)
	if m.items[0].status != taskError {
		t.Errorf("after Done: status = %v, want taskError", m.items[0].status)
	}
}

// A row that two sources report as failed finishes after both: the second
// report must count, or the spinner never quits.
func TestRefreshModel_SecondFailureCounts(t *testing.T) {
	for _, tt := range []struct {
		name   string
		second any
	}{
		{"error then error", taskErrorMsg{name: "a", err: fmt.Errorf("second")}},
		{"error then fail", taskFailMsg{name: "a"}},
	} {
		m := makeModel(task("a", 2), task("b", 1))
		m = update(t, m, taskDoneMsg{name: "b"})
		m = update(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("first")})
		m, cmd := updateCmd(t, m, tt.second)
		if m.remaining != 0 || cmd == nil {
			t.Errorf("%s: remaining = %d, quit = %v; want 0 and quit", tt.name, m.remaining, cmd != nil)
		}
		if m.items[0].status != taskError || m.items[0].errMsg != "first" {
			t.Errorf("%s: row = %v %q, want the first error kept", tt.name, m.items[0].status, m.items[0].errMsg)
		}
	}

	m := makeModel(task("a", 2))
	m = update(t, m, taskFailMsg{name: "a"})
	m, cmd := updateCmd(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("later")})
	if m.remaining != 0 || cmd == nil || m.items[0].status != taskFailed {
		t.Errorf("fail then error: remaining = %d, quit = %v, status = %v; want 0, quit, failed", m.remaining, cmd != nil, m.items[0].status)
	}
}

func TestRefreshModel_FailThenDone(t *testing.T) {
	m := makeModel(task("a", 2))

	m = update(t, m, taskFailMsg{name: "a"})
	if m.items[0].status != taskFailed {
		t.Errorf("after Fail: status = %v, want taskFailed", m.items[0].status)
	}
	if m.remaining != 1 {
		t.Errorf("after Fail: remaining = %d, want 1", m.remaining)
	}

	m = update(t, m, taskDoneMsg{name: "a"})
	if m.remaining != 0 {
		t.Errorf("after Done: remaining = %d, want 0", m.remaining)
	}
}

func TestRefreshModel_CtrlCCallsInterruptHandler(t *testing.T) {
	called := 0
	restore := SetInterruptHandler(func() { called++ })
	defer restore()

	m := makeModel(task("a", 1))
	m.canceled = make(chan struct{})
	key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if key.String() != "ctrl+c" {
		t.Fatalf("key = %q, want ctrl+c", key.String())
	}
	m = update(t, m, key)

	select {
	case <-m.canceled:
	default:
		t.Error("canceled was not closed")
	}
	if called != 1 {
		t.Errorf("interrupt handler called %d times, want 1", called)
	}
	if m.items[0].status != taskCanceled {
		t.Errorf("status = %v, want canceled", m.items[0].status)
	}
}

func TestRefreshModel_InterruptCancelsRunning(t *testing.T) {
	called := 0
	restore := SetInterruptHandler(func() { called++ })
	defer restore()

	m := makeModel(task("a", 1))
	m.canceled = make(chan struct{})
	m = update(t, m, interruptMsg{})

	if m.items[0].status != taskCanceled {
		t.Errorf("status = %v, want canceled", m.items[0].status)
	}
	select {
	case <-m.canceled:
		t.Error("canceled is for Ctrl+C only")
	default:
	}
	if called != 0 {
		t.Error("Interrupt must not call the interrupt handler")
	}
}

func TestRefreshTracker_InterruptIgnoresLaterErrors(t *testing.T) {
	var out bytes.Buffer
	restore := SetDefaultPrinter(NewStandardPrinterWith(&out, &out))
	defer restore()

	tracker := &RefreshTracker{fallback: true, done: closedChan()}
	tracker.Error("before", fmt.Errorf("real error"))
	tracker.Interrupt()
	tracker.Error("after", fmt.Errorf("gh exited with code -1"))

	errs := tracker.Errors()
	if len(errs) != 1 || errs[0].Name != "before" {
		t.Errorf("errors = %v, want only the one before Interrupt", errs)
	}
	if strings.Contains(out.String(), "code -1") {
		t.Errorf("an error after Interrupt was printed: %q", out.String())
	}
}

func TestRefreshModel_StatusUpdate(t *testing.T) {
	m := makeModel(task("a", 1))

	m = update(t, m, taskStatusMsg{name: "a", status: "fetching secrets..."})
	if m.items[0].statusText != "fetching secrets..." {
		t.Errorf("statusText = %q, want %q", m.items[0].statusText, "fetching secrets...")
	}

	// Status cleared on Done
	m = update(t, m, taskDoneMsg{name: "a"})
	if m.items[0].statusText != "" {
		t.Errorf("statusText after Done = %q, want empty", m.items[0].statusText)
	}
}

func TestRefreshModel_CheckpointRemainsRunning(t *testing.T) {
	m := makeModel(task("a", 1))
	m = update(t, m, taskCheckpointMsg{name: "a", status: "repository"})

	if m.items[0].status != taskRunning {
		t.Errorf("status = %v, want taskRunning", m.items[0].status)
	}
	if m.items[0].checkpoint != "repository" {
		t.Fatal("checkpoint should be set")
	}
	if m.remaining != 1 {
		t.Errorf("remaining = %d, want 1", m.remaining)
	}

	m = update(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("validation failed")})
	if m.items[0].status != taskError {
		t.Errorf("status after error = %v, want taskError", m.items[0].status)
	}
}

func TestRefreshModel_StatusIgnoredAfterError(t *testing.T) {
	m := makeModel(task("a", 1))

	m = update(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("boom")})
	m = update(t, m, taskStatusMsg{name: "a", status: "should be ignored"})

	// statusText should not be set because the task is no longer running
	if m.items[0].statusText != "" {
		t.Errorf("statusText = %q, want empty (task already errored)", m.items[0].statusText)
	}
}

func TestRefreshModel_MultipleTasks(t *testing.T) {
	m := makeModel(task("a", 1), task("b", 2))

	m = update(t, m, taskDoneMsg{name: "a"})
	if m.remaining != 1 {
		t.Errorf("remaining = %d, want 1", m.remaining)
	}

	m = update(t, m, taskDoneMsg{name: "b"})
	if m.remaining != 1 {
		t.Errorf("remaining = %d, want 1 (b has pending=2)", m.remaining)
	}

	m = update(t, m, taskDoneMsg{name: "b"})
	if m.remaining != 0 {
		t.Errorf("remaining = %d, want 0", m.remaining)
	}
}

func TestRefreshModel_ErrorClearsStatusText(t *testing.T) {
	m := makeModel(task("a", 2))

	m = update(t, m, taskStatusMsg{name: "a", status: "fetching..."})
	if m.items[0].statusText != "fetching..." {
		t.Fatalf("statusText not set")
	}

	m = update(t, m, taskErrorMsg{name: "a", err: fmt.Errorf("403")})
	if m.items[0].statusText != "" {
		t.Errorf("statusText after Error = %q, want empty", m.items[0].statusText)
	}
}

func TestNewRefreshModel_DefaultPending(t *testing.T) {
	m := newRefreshModel([]RefreshTask{{Name: "a"}})
	if m.items[0].pending != 1 {
		t.Errorf("default pending = %d, want 1", m.items[0].pending)
	}
}

func TestNewRefreshModel_ExplicitPending(t *testing.T) {
	m := newRefreshModel([]RefreshTask{{Name: "a", Pending: 3}})
	if m.items[0].pending != 3 {
		t.Errorf("pending = %d, want 3", m.items[0].pending)
	}
}

// ---------------------------------------------------------------------------
// View rendering
// ---------------------------------------------------------------------------

func TestView_ColumnAlignment(t *testing.T) {
	m := makeModel(task("short", 1), task("much-longer-name", 1))

	// Complete the first task
	m = update(t, m, taskDoneMsg{name: "short"})

	view := m.View().Content
	// Both lines should exist
	if !strings.Contains(view, "short") {
		t.Error("view missing 'short'")
	}
	if !strings.Contains(view, "much-longer-name") {
		t.Error("view missing 'much-longer-name'")
	}
}

func TestView_StatusTextRendered(t *testing.T) {
	m := makeModel(task("repo", 1))
	m = update(t, m, taskStatusMsg{name: "repo", status: "fetching secrets..."})

	view := m.View().Content
	if !strings.Contains(view, "fetching secrets...") {
		t.Errorf("view missing status text, got:\n%s", view)
	}
}

// frozen makes m's clock stand still: every task has run for 1.5s.
func frozen(m refreshModel) refreshModel {
	t0 := time.Unix(1_000_000, 0)
	m.start = t0
	m.now = func() time.Time { return t0.Add(1500 * time.Millisecond) }
	return m
}

// viewLines returns the view without styles, one line per element.
func viewLines(m refreshModel) []string {
	return strings.Split(strings.TrimSuffix(stripANSIForTest(m.View().Content), "\n"), "\n")
}

func TestView_CheckpointKeepsTheSpinner(t *testing.T) {
	m := frozen(makeModel(task("repo", 1)))
	spin := stripANSIForTest(m.items[0].spinner.View())
	m = update(t, m, taskCheckpointMsg{name: "repo", status: "repository"})
	if got, want := viewLines(m)[0], "  "+spin+" repo   1.5s  repository ✓"; got != want {
		t.Errorf("after the checkpoint:\ngot  %q\nwant %q", got, want)
	}
	m = update(t, m, taskStatusMsg{name: "repo", status: "fetching file LICENSE..."})
	if got, want := viewLines(m)[0], "  "+spin+" repo   1.5s  repository ✓ · fetching file LICENSE..."; got != want {
		t.Errorf("after the next status:\ngot  %q\nwant %q", got, want)
	}
}

func TestView_Rows(t *testing.T) {
	tests := []struct {
		name  string
		tasks []RefreshTask
		msgs  []any
		want  string // the first row
	}{
		{name: "done with a change",
			tasks: []RefreshTask{{Name: "o/a"}},
			msgs:  []any{taskDoneMsg{name: "o/a", labeled: true, mark: MarkChange, label: "4 changes · 3 files"}},
			want:  "  ~ o/a   1.5s  4 changes · 3 files"},
		{name: "done, created",
			tasks: []RefreshTask{{Name: "o/a"}},
			msgs:  []any{taskDoneMsg{name: "o/a", labeled: true, mark: MarkCreate, label: "will be created"}},
			want:  "  + o/a   1.5s  will be created"},
		{name: "done, deleted",
			tasks: []RefreshTask{{Name: "o/a"}},
			msgs:  []any{taskDoneMsg{name: "o/a", labeled: true, mark: MarkDelete, label: "will be destroyed"}},
			want:  "  - o/a   1.5s  will be destroyed"},
		{name: "done, nothing to change",
			tasks: []RefreshTask{{Name: "o/a"}},
			msgs:  []any{taskDoneMsg{name: "o/a", labeled: true, mark: MarkNone, label: "up to date"}},
			want:  "  ✓ o/a   1.5s  up to date"},
		{name: "done with the task's label",
			tasks: []RefreshTask{{Name: "o/a", DoneLabel: "fetched repository state"}},
			msgs:  []any{taskDoneMsg{name: "o/a"}},
			want:  "  ✓ o/a   1.5s  fetched repository state"},
		{name: "the last result sent before completing is shown",
			tasks: []RefreshTask{{Name: "o/a", Pending: 2}},
			msgs: []any{
				taskDoneMsg{name: "o/a", labeled: true, mark: MarkChange, label: "1 change"},
				taskDoneMsg{name: "o/a", labeled: true, mark: MarkChange, label: "1 change · 2 files"},
			},
			want: "  ~ o/a   1.5s  1 change · 2 files"},
		{name: "an error with the task's prefix",
			tasks: []RefreshTask{{Name: "o/a", ErrorPrefix: "skipped"}},
			msgs:  []any{taskErrorMsg{name: "o/a", err: fmt.Errorf("HTTP 403: Forbidden\nmore")}},
			want:  "  ✗ o/a   1.5s  skipped · HTTP 403: Forbidden…"},
		{name: "an error without a prefix",
			tasks: []RefreshTask{{Name: "o/a"}},
			msgs:  []any{taskErrorMsg{name: "o/a", err: fmt.Errorf("boom")}},
			want:  "  ✗ o/a   1.5s  boom"},
		{name: "waiting for the next report",
			tasks: []RefreshTask{{Name: "o/a", Pending: 2, WaitLabel: "waiting for the next step..."}},
			msgs: []any{
				taskStatusMsg{name: "o/a", status: "applying topics..."},
				taskDoneMsg{name: "o/a", labeled: true, label: "3 applied"},
			},
			want: "  SPIN o/a   1.5s  waiting for the next step..."},
		{name: "without a wait label the status stays",
			tasks: []RefreshTask{{Name: "o/a", Pending: 2}},
			msgs: []any{
				taskStatusMsg{name: "o/a", status: "fetching file LICENSE..."},
				taskDoneMsg{name: "o/a", labeled: true, mark: MarkChange, label: "1 change"},
			},
			want: "  SPIN o/a   1.5s  fetching file LICENSE..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := frozen(newRefreshModel(append(tt.tasks, RefreshTask{Name: "o/b"})))
			spin := stripANSIForTest(m.items[0].spinner.View())
			for _, msg := range tt.msgs {
				m = update(t, m, msg)
			}
			want := strings.Replace(tt.want, "SPIN", spin, 1)
			if got := viewLines(m)[0]; got != want {
				t.Errorf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestView_ElapsedStopsWhenTheTaskEnds(t *testing.T) {
	m := frozen(makeModel(task("o/a", 1), task("o/b", 1)))
	m = update(t, m, taskDoneMsg{name: "o/a"})
	t0 := m.start
	m.now = func() time.Time { return t0.Add(9 * time.Second) }
	lines := viewLines(m)
	if !strings.HasPrefix(lines[0], "  ✓ o/a   1.5s") {
		t.Errorf("a finished task shows what it took: %q", lines[0])
	}
	if !strings.Contains(lines[1], "o/b   9.0s") {
		t.Errorf("a running task shows its time so far: %q", lines[1])
	}
}

func TestView_CtrlCHint(t *testing.T) {
	const hint = "ctrl+c to cancel"
	m := frozen(makeModel(task("o/a", 1), task("o/b", 1)))
	if !strings.HasSuffix(stripANSIForTest(m.View().Content), "\n\n  "+hint+"\n") {
		t.Errorf("running: want the hint after a blank line, got:\n%s", m.View().Content)
	}

	m = update(t, m, noticeMsg{text: "waiting for the rate limit"})
	if !strings.HasSuffix(stripANSIForTest(m.View().Content), "waiting for the rate limit\n\n  "+hint+"\n") {
		t.Errorf("with a notice: want the hint last, got:\n%s", m.View().Content)
	}

	done := update(t, update(t, m, taskDoneMsg{name: "o/a"}), taskDoneMsg{name: "o/b"})
	if strings.Contains(done.View().Content, hint) {
		t.Errorf("finished: want no hint, got:\n%s", done.View().Content)
	}

	stopped := update(t, m, interruptMsg{})
	if strings.Contains(stopped.View().Content, hint) {
		t.Errorf("stopped: want no hint, got:\n%s", stopped.View().Content)
	}
}

func TestView_DoneHidesStatusText(t *testing.T) {
	m := makeModel(task("repo", 1))
	m = update(t, m, taskStatusMsg{name: "repo", status: "fetching..."})
	m = update(t, m, taskDoneMsg{name: "repo"})

	view := m.View().Content
	if strings.Contains(view, "fetching...") {
		t.Errorf("view should not contain status text after Done, got:\n%s", view)
	}
}

func TestView_DoneLabelRenderedAsStatus(t *testing.T) {
	m := frozen(newRefreshModel([]RefreshTask{
		{Name: "short", DoneLabel: "fetched repository state"},
		{Name: "much-longer-name"},
	}))
	m = update(t, m, taskDoneMsg{name: "short"})

	if got, want := viewLines(m)[0], "  ✓ short              1.5s  fetched repository state"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestView_ErrorShowsMessage(t *testing.T) {
	m := makeModel(task("repo", 1))
	m = update(t, m, taskErrorMsg{name: "repo", err: fmt.Errorf("403 Forbidden")})

	view := m.View().Content
	if !strings.Contains(view, "403 Forbidden") {
		t.Errorf("view missing error message, got:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// truncateError
// ---------------------------------------------------------------------------

func TestTruncateError(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		maxWidth int
		want     string
	}{
		{"short single line", "403 Forbidden", 40, "403 Forbidden"},
		{"multi-line", "first line\nsecond line\nthird line", 40, "first line…"},
		{"long single line", "abcdefghijklmnopqrstuvwxyz", 10, "abcdefghi…"},
		{"long multi-line", "abcdefghijklmnopqrstuvwxyz\nsecond", 10, "abcdefghi…"},
		{"empty", "", 40, ""},
		{"exact width", "abcdefghij", 10, "abcdefghij"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateError(tt.msg, tt.maxWidth)
			if got != tt.want {
				t.Errorf("truncateError(%q, %d) = %q, want %q", tt.msg, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestView_ErrorTruncated(t *testing.T) {
	m := makeModel(task("repo", 1))
	m = update(t, m, taskErrorMsg{
		name: "repo",
		err:  fmt.Errorf("first line\nHint: do something\nUnderlying error: details"),
	})

	view := m.View().Content
	if !strings.Contains(view, "first line") {
		t.Errorf("view missing first line of error, got:\n%s", view)
	}
	if strings.Contains(view, "Hint:") {
		t.Errorf("view should not contain second line of error, got:\n%s", view)
	}
	if strings.Contains(view, "Underlying error:") {
		t.Errorf("view should not contain third line of error, got:\n%s", view)
	}
}

func TestRefreshTracker_ErrorsCollected(t *testing.T) {
	tracker := &RefreshTracker{fallback: true, done: closedChan()}

	// In fallback mode, Error() prints to DefaultPrinter but also collects.
	// Override DefaultPrinter to suppress output.
	oldPrinter := DefaultPrinter
	var buf bytes.Buffer
	DefaultPrinter = NewStandardPrinterWith(&buf, &buf)
	defer func() { DefaultPrinter = oldPrinter }()

	tracker.Error("repo/a", fmt.Errorf("err1"))
	tracker.Error("repo/b", fmt.Errorf("err2"))

	errors := tracker.Errors()
	if len(errors) != 2 {
		t.Fatalf("len(Errors()) = %d, want 2", len(errors))
	}
	if errors[0].Name != "repo/a" || errors[0].Err.Error() != "err1" {
		t.Errorf("errors[0] = {%q, %q}, want {repo/a, err1}", errors[0].Name, errors[0].Err)
	}
	if errors[1].Name != "repo/b" || errors[1].Err.Error() != "err2" {
		t.Errorf("errors[1] = {%q, %q}, want {repo/b, err2}", errors[1].Name, errors[1].Err)
	}
}

// stripANSIForTest removes ANSI escape codes to make output assertions
// resilient to color rendering (Bold, Dim, etc.).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSIForTest(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestRefreshTracker_PrintErrors_NoErrors(t *testing.T) {
	tracker := &RefreshTracker{fallback: true, done: closedChan()}

	oldPrinter := DefaultPrinter
	var buf bytes.Buffer
	DefaultPrinter = NewStandardPrinterWith(&buf, &buf)
	defer func() { DefaultPrinter = oldPrinter }()

	if printed := tracker.PrintErrors(); printed {
		t.Errorf("PrintErrors() = true on empty tracker, want false")
	}
	if buf.Len() != 0 {
		t.Errorf("PrintErrors() emitted output on empty tracker: %q", buf.String())
	}
}

func TestRefreshTracker_PrintErrors_BlockFormat(t *testing.T) {
	tracker := &RefreshTracker{fallback: true, done: closedChan()}

	oldPrinter := DefaultPrinter
	var collectBuf bytes.Buffer
	DefaultPrinter = NewStandardPrinterWith(&collectBuf, &collectBuf)
	defer func() { DefaultPrinter = oldPrinter }()

	// Append errors directly to avoid the fallback-path inline print from
	// tracker.Error(). We want to inspect only the PrintErrors() output.
	tracker.errors = []TaskError{
		{Name: "org/repo1", Err: fmt.Errorf("line-one\nline-two")},
		{Name: "org/repo2", Err: fmt.Errorf("single line error")},
	}

	collectBuf.Reset()
	if printed := tracker.PrintErrors(); !printed {
		t.Fatalf("PrintErrors() = false on tracker with errors, want true")
	}

	got := stripANSIForTest(collectBuf.String())
	want := "\n  org/repo1:\n    line-one\n    line-two\n\n  org/repo2:\n    single line error\n"
	if got != want {
		t.Errorf("PrintErrors output mismatch.\n got: %q\nwant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// StreamReporter
// ---------------------------------------------------------------------------

func TestStreamReporter_Start(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	r := NewStreamReporter(p, "Applying", "Applied")

	r.Start("org/repo", []string{"description", "visibility"})
	out := buf.String()
	if !strings.Contains(out, "Applying") {
		t.Errorf("Start output missing verb, got:\n%s", out)
	}
	if !strings.Contains(out, "org/repo") {
		t.Errorf("Start output missing name, got:\n%s", out)
	}
}

func TestStreamReporter_Done(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	r := NewStreamReporter(p, "Applying", "Applied")

	r.Done("org/repo", 1500000000, 3) // 1.5s
	out := buf.String()
	if !strings.Contains(out, "Applied") {
		t.Errorf("Done output missing past verb, got:\n%s", out)
	}
	if !strings.Contains(out, "3 changes") {
		t.Errorf("Done output missing count, got:\n%s", out)
	}
}

func TestStreamReporter_Error(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	r := NewStreamReporter(p, "Applying", "Applied")

	r.Error("org/repo", 0, fmt.Errorf("permission denied"))
	out := buf.String()
	if !strings.Contains(out, "permission denied") {
		t.Errorf("Error output missing error message, got:\n%s", out)
	}
}

func TestStreamReporter_Wait(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	r := NewStreamReporter(p, "Applying", "Applied")

	// Should not panic or block
	r.Wait()
}

func TestRefreshModel_Notice(t *testing.T) {
	m := newRefreshModel([]RefreshTask{{Name: "o/r"}})
	next, _ := m.Update(noticeMsg{text: "Waiting for the GitHub API rate limit until 10:42:00 ..."})
	m, _ = next.(refreshModel)
	if view := m.View().Content; !strings.Contains(view, "Waiting for the GitHub API rate limit until 10:42:00") {
		t.Errorf("notice missing:\n%s", view)
	}
	next, _ = m.Update(noticeMsg{})
	m, _ = next.(refreshModel)
	if view := m.View().Content; strings.Contains(view, "Waiting") {
		t.Errorf("notice should be cleared:\n%s", view)
	}
}
