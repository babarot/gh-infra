package manifest

import (
	"testing"

	"github.com/goccy/go-yaml"
)

func TestFileSetRepository_UnmarshalYAML_String(t *testing.T) {
	input := `"my-repo"`
	var target FileSetRepository
	if err := yaml.Unmarshal([]byte(input), &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "my-repo" {
		t.Errorf("Name = %q, want %q", target.Name, "my-repo")
	}
	if len(target.Overrides) != 0 {
		t.Errorf("expected no overrides, got %d", len(target.Overrides))
	}
}

func TestFileSet_Identity_WithName(t *testing.T) {
	fs := &FileSet{
		Metadata: FileSetMetadata{Name: "gomi", Owner: "babarot"},
		Spec:     FileSetSpec{Repositories: []FileSetRepository{{Name: "gomi"}}},
	}
	if got := fs.Identity(); got != "babarot/gomi" {
		t.Errorf("Identity() = %q, want %q", got, "babarot/gomi")
	}
}

func TestFileSet_Identity_WithoutName(t *testing.T) {
	fs := &FileSet{
		Metadata: FileSetMetadata{Owner: "org"},
		Spec:     FileSetSpec{Repositories: []FileSetRepository{{Name: "b"}, {Name: "a"}}},
	}
	if got := fs.Identity(); got != "org/a+b" {
		t.Errorf("Identity() = %q, want %q", got, "org/a+b")
	}
}

func TestFileSetRepository_UnmarshalYAML_Struct(t *testing.T) {
	input := `
name: my-repo
overrides:
  - path: .github/ci.yml
    content: "custom content"
`
	var target FileSetRepository
	if err := yaml.Unmarshal([]byte(input), &target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Name != "my-repo" {
		t.Errorf("Name = %q, want %q", target.Name, "my-repo")
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

func TestFileEntry_UnmarshalYAML_ExecutableTrue(t *testing.T) {
	input := `
path: bin/tool
content: "#!/bin/sh\necho hello"
executable: true
`
	var entry FileEntry
	if err := yaml.Unmarshal([]byte(input), &entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Executable == nil {
		t.Fatal("Executable should be non-nil when set to true in YAML")
	}
	if !*entry.Executable {
		t.Errorf("Executable = false, want true")
	}
}

func TestFileEntry_UnmarshalYAML_ExecutableOmitted(t *testing.T) {
	input := `
path: some/file.txt
content: "hello"
`
	var entry FileEntry
	if err := yaml.Unmarshal([]byte(input), &entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Executable != nil {
		t.Errorf("Executable should be nil when omitted from YAML, got %v", *entry.Executable)
	}
}
