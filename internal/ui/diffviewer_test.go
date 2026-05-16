package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestGenerateDiff_Update(t *testing.T) {
	current := "line1\nline2\n"
	desired := "line1\nline2\nline3\n"
	diff := GenerateDiff(current, desired, "test.txt")

	if !strings.Contains(diff, "+line3") {
		t.Errorf("expected +line3 in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "test.txt (current)") {
		t.Errorf("expected 'test.txt (current)' header in diff")
	}
}

func TestGenerateDiff_Create(t *testing.T) {
	diff := GenerateDiff("", "new content\n", "new.txt")
	if !strings.Contains(diff, "+new content") {
		t.Errorf("expected +new content in diff, got:\n%s", diff)
	}
}

func TestGenerateDiff_Delete(t *testing.T) {
	diff := GenerateDiff("old content\n", "", "old.txt")
	if !strings.Contains(diff, "-old content") {
		t.Errorf("expected -old content in diff, got:\n%s", diff)
	}
}

func TestGenerateDiff_NoDiff(t *testing.T) {
	diff := GenerateDiff("same\n", "same\n", "file.txt")
	if diff != "" {
		t.Errorf("expected empty diff for identical content, got:\n%s", diff)
	}
}

func TestBuildListItems_GroupsByRepo(t *testing.T) {
	m := &diffViewModel{
		entries: []DiffEntry{
			{Path: ".github/release.yml", Target: "org/repo-a", Icon: "~"},
			{Path: ".octocov.yaml", Target: "org/repo-a", Icon: "+"},
			{Path: ".github/release.yml", Target: "org/repo-b", Icon: "~"},
			{Path: ".tagpr", Target: "org/repo-b", Icon: "+"},
		},
		listWidth: 50,
	}

	items := m.buildListItems()

	// Expect: header(repo-a), file, file, header(repo-b), file, file = 6 items
	if len(items) != 6 {
		t.Fatalf("expected 6 items (2 headers + 4 files), got %d", len(items))
	}

	// Headers should have entryIdx -1
	if items[0].entryIdx != -1 {
		t.Error("first item should be a header (entryIdx=-1)")
	}
	if !strings.Contains(items[0].text, "org/repo-a") {
		t.Error("first header should contain repo-a")
	}
	if items[3].entryIdx != -1 {
		t.Error("fourth item should be a header (entryIdx=-1)")
	}
	if !strings.Contains(items[3].text, "org/repo-b") {
		t.Error("second header should contain repo-b")
	}

	// File items should reference correct entry indices
	if items[1].entryIdx != 0 || items[2].entryIdx != 1 {
		t.Errorf("first group file indices: got %d, %d; want 0, 1", items[1].entryIdx, items[2].entryIdx)
	}
	if items[4].entryIdx != 2 || items[5].entryIdx != 3 {
		t.Errorf("second group file indices: got %d, %d; want 2, 3", items[4].entryIdx, items[5].entryIdx)
	}
}

func TestBuildRightPane_Skip(t *testing.T) {
	m := &diffViewModel{entries: []DiffEntry{{
		Path:    "a.txt",
		Target:  "org/repo",
		Current: "hello\nworld\n",
		Desired: "new\n",
		Skip:    true,
	}}, width: 100, listWidth: 30}

	lines := m.buildRightPane(m.entries[0], 60)

	found := false
	for _, l := range lines {
		if strings.Contains(l, "Skipped") || strings.Contains(l, "will not be applied") {
			found = true
		}
	}
	if !found {
		t.Error("expected skip description for skipped entry")
	}
}

func TestBuildRightPane_NotSkipped(t *testing.T) {
	m := &diffViewModel{entries: []DiffEntry{{
		Path:    "a.txt",
		Target:  "org/repo",
		Current: "old\n",
		Desired: "new\n",
		Skip:    false,
	}}, width: 100, listWidth: 30}

	lines := m.buildRightPane(m.entries[0], 60)

	// Should show unified diff
	hasDiffMarker := false
	for _, l := range lines {
		if strings.Contains(l, "@@") || strings.Contains(l, "---") {
			hasDiffMarker = true
		}
	}
	if !hasDiffMarker {
		t.Error("non-skipped entry should show unified diff")
	}
}

// ---------------------------------------------------------------------------
// colorDiffLine
// ---------------------------------------------------------------------------

func TestColorDiffLine(t *testing.T) {
	DisableStyles()

	tests := []struct {
		name     string
		line     string
		maxWidth int
		want     string
	}{
		{"added line no trunc", "+new content", 0, "+new content"},
		{"removed line no trunc", "-old content", 0, "-old content"},
		{"hunk header", "@@ -1,3 +1,4 @@", 0, "@@ -1,3 +1,4 @@"},
		{"file header plus", "+++ b/file.go", 0, "+++ b/file.go"},
		{"file header minus", "--- a/file.go", 0, "--- a/file.go"},
		{"context line", " unchanged", 0, " unchanged"},
		{"empty string", "", 0, ""},
		{"added line truncated", "+very long added content here", 10, "+very l..."},
		{"removed line truncated", "-very long removed content", 10, "-very l..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorDiffLine(tt.line, tt.maxWidth)
			if got != tt.want {
				t.Errorf("colorDiffLine(%q, %d) = %q, want %q", tt.line, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestColorDiffLine_StripsANSI(t *testing.T) {
	DisableStyles()

	line := "+\x1b[2Jmalicious content"
	got := colorDiffLine(line, 0)
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI sequences stripped, got: %q", got)
	}
	if got != "+malicious content" {
		t.Errorf("expected '+malicious content', got: %q", got)
	}
}

func TestColorDiffLine_PrefixNotGreen(t *testing.T) {
	DisableStyles()

	// "+++ " lines should be treated as file headers, not added lines
	got := colorDiffLine("+++ b/file.go (desired)", 0)
	if got != "+++ b/file.go (desired)" {
		t.Errorf("expected file header unchanged with disabled styles, got: %q", got)
	}

	// "--- " lines should be treated as file headers, not removed lines
	got = colorDiffLine("--- a/file.go (current)", 0)
	if got != "--- a/file.go (current)" {
		t.Errorf("expected file header unchanged with disabled styles, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// TruncateDiff
// ---------------------------------------------------------------------------

func TestTruncateDiff_UnderLimit(t *testing.T) {
	diff := "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n"
	got := TruncateDiff(diff, 500)
	if got != diff {
		t.Errorf("expected unchanged diff, got:\n%s", got)
	}
}

func TestTruncateDiff_OverLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("--- a/f\n+++ b/f\n@@ -1 +1 @@\n")
	for i := range 600 {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	diff := b.String()

	got := TruncateDiff(diff, 10)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 11 { // 10 retained + 1 truncation note
		t.Errorf("expected 11 lines, got %d", len(lines))
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "lines truncated") {
		t.Errorf("expected truncation message, got: %q", last)
	}
}

func TestTruncateDiff_Empty(t *testing.T) {
	got := TruncateDiff("", 500)
	if got != "" {
		t.Errorf("expected empty, got: %q", got)
	}
}

func TestDiffEntry_CycleAction(t *testing.T) {
	entry := DiffEntry{
		Path:           "VERSION",
		Target:         "org/repo",
		Action:         "skip",
		DefaultAction:  "skip",
		AllowedActions: []string{"write", "patch", "skip"},
		Current:        "skip-current\n",
		SkipCurrent:    "skip-current\n",
		WriteCurrent:   "write-current\n",
		PatchCurrent:   "patch-current\n",
	}

	entry.cycleAction()
	if entry.Action != "write" {
		t.Fatalf("Action after first cycle = %q, want write", entry.Action)
	}
	if entry.Current != "write-current\n" {
		t.Fatalf("Current after first cycle = %q, want write-current", entry.Current)
	}

	entry.cycleAction()
	if entry.Action != "patch" {
		t.Fatalf("Action after second cycle = %q, want patch", entry.Action)
	}
	if entry.Current != "patch-current\n" {
		t.Fatalf("Current after second cycle = %q, want patch-current", entry.Current)
	}

	entry.cycleAction()
	if entry.Action != "skip" {
		t.Fatalf("Action after third cycle = %q, want skip", entry.Action)
	}
	if entry.Current != "skip-current\n" {
		t.Fatalf("Current after third cycle = %q, want skip-current", entry.Current)
	}
}
