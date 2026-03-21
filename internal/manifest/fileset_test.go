package manifest

import (
	"testing"

	"github.com/goccy/go-yaml"
)

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
