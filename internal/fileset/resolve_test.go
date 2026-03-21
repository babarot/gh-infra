package fileset

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestResolveFiles_NoOverrides(t *testing.T) {
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{Path: "a.txt", Content: "aaa"},
				{Path: "b.txt", Content: "bbb"},
			},
		},
	}
	target := manifest.FileSetTarget{Name: "owner/repo"}

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
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{Path: "a.txt", Content: "original-a"},
				{Path: "b.txt", Content: "original-b"},
				{Path: "c.txt", Content: "original-c"},
			},
		},
	}
	target := manifest.FileSetTarget{
		Name: "owner/repo",
		Overrides: []manifest.FileEntry{
			{Path: "b.txt", Content: "overridden-b"},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result))
	}
	if result[0].Content != "original-a" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "original-a")
	}
	if result[1].Content != "overridden-b" {
		t.Errorf("result[1].Content = %q, want %q", result[1].Content, "overridden-b")
	}
	if result[2].Content != "original-c" {
		t.Errorf("result[2].Content = %q, want %q", result[2].Content, "original-c")
	}
}
