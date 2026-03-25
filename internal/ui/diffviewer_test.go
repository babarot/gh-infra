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
