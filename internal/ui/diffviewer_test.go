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
		Current: "hello\nworld\n",
		OnDrift: "skip",
	}}, width: 100, listWidth: 30}

	lines := m.buildRightPane(m.entries[0], 60)

	found := false
	for _, l := range lines {
		if strings.Contains(l, "kept as-is") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'kept as-is' description for skip mode")
	}
	// Should show current content, not diff
	hasHello := false
	for _, l := range lines {
		if strings.Contains(l, "hello") {
			hasHello = true
		}
	}
	if !hasHello {
		t.Error("expected current content 'hello' in skip mode")
	}
}

func TestBuildRightPane_Overwrite(t *testing.T) {
	m := &diffViewModel{entries: []DiffEntry{{
		Path:    "a.txt",
		Current: "old\n",
		Desired: "new\n",
		OnDrift: "overwrite",
	}}, width: 100, listWidth: 30}

	lines := m.buildRightPane(m.entries[0], 60)

	found := false
	for _, l := range lines {
		if strings.Contains(l, "will overwrite") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'will overwrite' description for overwrite mode")
	}
	// Should show desired content (green), not unified diff
	hasDiffHeader := false
	for _, l := range lines {
		if strings.Contains(l, "---") || strings.Contains(l, "@@") {
			hasDiffHeader = true
		}
	}
	if hasDiffHeader {
		t.Error("overwrite mode should show desired content, not unified diff")
	}
}

func TestBuildRightPane_Warn(t *testing.T) {
	m := &diffViewModel{entries: []DiffEntry{{
		Path:    "a.txt",
		Current: "old\n",
		Desired: "new\n",
		OnDrift: "warn",
	}}, width: 100, listWidth: 30}

	lines := m.buildRightPane(m.entries[0], 60)

	found := false
	for _, l := range lines {
		if strings.Contains(l, "warn but skip") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'warn but skip' description for warn mode")
	}
	// Should show unified diff
	hasDiffMarker := false
	for _, l := range lines {
		if strings.Contains(l, "@@") || strings.Contains(l, "---") {
			hasDiffMarker = true
		}
	}
	if !hasDiffMarker {
		t.Error("warn mode should show unified diff")
	}
}

func TestRenderOnDriftShort(t *testing.T) {
	DisableStyles()
	defer func() {
		// Re-initialize would be complex; just check the values
	}()

	tests := []struct {
		input string
		want  string
	}{
		{"warn", "[W]"},
		{"overwrite", "[O]"},
		{"skip", "[S]"},
		{"", ""},
	}
	for _, tt := range tests {
		got := renderOnDriftShort(tt.input)
		if got != tt.want {
			t.Errorf("renderOnDriftShort(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
