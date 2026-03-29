package ui

import (
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
