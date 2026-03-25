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
	target := manifest.FileSetRepository{Name: "repo"}

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
	target := manifest.FileSetRepository{
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

func TestResolveFiles_InheritsDirScopeAndReconcile(t *testing.T) {
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{
					Path:     "config/a.yml",
					Content:  "original-a",
					DirScope: "config",
					Reconcile: manifest.ReconcileMirror,
					Vars:     map[string]string{"env": "prod"},
				},
				{
					Path:     "config/b.yml",
					Content:  "original-b",
					DirScope: "config",
					Reconcile: manifest.ReconcileMirror,
				},
			},
		},
	}
	target := manifest.FileSetRepository{
		Name: "repo",
		Overrides: []manifest.FileEntry{
			// Override content but don't set DirScope or Reconcile
			{Path: "config/a.yml", Content: "overridden-a"},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}

	// Check the overridden entry inherits DirScope and Reconcile
	if result[0].Content != "overridden-a" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "overridden-a")
	}
	if result[0].DirScope != "config" {
		t.Errorf("result[0].DirScope = %q, want %q (should inherit from original)", result[0].DirScope, "config")
	}
	if result[0].Reconcile != manifest.ReconcileMirror {
		t.Errorf("result[0].Reconcile = %q, want %q (should inherit from original)", result[0].Reconcile, manifest.ReconcileMirror)
	}
	// Vars should also be inherited
	if result[0].Vars == nil || result[0].Vars["env"] != "prod" {
		t.Errorf("result[0].Vars should inherit from original, got %v", result[0].Vars)
	}

	// Non-overridden entry should retain its fields
	if result[1].DirScope != "config" {
		t.Errorf("result[1].DirScope = %q, want %q", result[1].DirScope, "config")
	}
	if result[1].Reconcile != manifest.ReconcileMirror {
		t.Errorf("result[1].Reconcile = %q, want %q", result[1].Reconcile, manifest.ReconcileMirror)
	}
}

