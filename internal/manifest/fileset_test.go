package manifest

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// ---------------------------------------------------------------------------
// UnmarshalYAML tests
// ---------------------------------------------------------------------------

func TestFileSetTarget_UnmarshalYAML_String(t *testing.T) {
	input := `"owner/repo"`
	var target FileSetTarget
	if err := yaml.Unmarshal([]byte(input), &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "owner/repo" {
		t.Errorf("Name = %q, want %q", target.Name, "owner/repo")
	}
	if len(target.Overrides) != 0 {
		t.Errorf("expected no overrides, got %d", len(target.Overrides))
	}
}

func TestFileSetTarget_UnmarshalYAML_Struct(t *testing.T) {
	input := `
name: owner/repo
overrides:
  - path: .github/ci.yml
    content: "custom content"
`
	var target FileSetTarget
	if err := yaml.Unmarshal([]byte(input), &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "owner/repo" {
		t.Errorf("Name = %q, want %q", target.Name, "owner/repo")
	}
	if len(target.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(target.Overrides))
	}
	if target.Overrides[0].Path != ".github/ci.yml" {
		t.Errorf("override path = %q, want %q", target.Overrides[0].Path, ".github/ci.yml")
	}
	if target.Overrides[0].Content != "custom content" {
		t.Errorf("override content = %q, want %q", target.Overrides[0].Content, "custom content")
	}
}

// ---------------------------------------------------------------------------
// ResolveFiles tests
// ---------------------------------------------------------------------------

func TestResolveFiles_NoOverrides(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "a.txt", Content: "aaa"},
				{Path: "b.txt", Content: "bbb"},
			},
		},
	}
	target := FileSetTarget{Name: "owner/repo"}

	result := ResolveFiles(fs, target)

	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}
	if result[0].Content != "aaa" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "aaa")
	}
	if result[1].Content != "bbb" {
		t.Errorf("result[1].Content = %q, want %q", result[1].Content, "bbb")
	}
}

func TestResolveFiles_WithOverrides(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "a.txt", Content: "original-a"},
				{Path: "b.txt", Content: "original-b"},
				{Path: "c.txt", Content: "original-c"},
			},
		},
	}
	target := FileSetTarget{
		Name: "owner/repo",
		Overrides: []FileEntry{
			{Path: "b.txt", Content: "overridden-b"},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result))
	}
	// a.txt: unchanged
	if result[0].Content != "original-a" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "original-a")
	}
	// b.txt: overridden
	if result[1].Content != "overridden-b" {
		t.Errorf("result[1].Content = %q, want %q", result[1].Content, "overridden-b")
	}
	// c.txt: unchanged
	if result[2].Content != "original-c" {
		t.Errorf("result[2].Content = %q, want %q", result[2].Content, "original-c")
	}
}
