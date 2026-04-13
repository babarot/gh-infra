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
					Path:      "config/a.yml",
					Content:   "original-a",
					DirScope:  "config",
					Reconcile: manifest.ReconcileMirror,
					Vars:      map[string]string{"env": "prod"},
				},
				{
					Path:      "config/b.yml",
					Content:   "original-b",
					DirScope:  "config",
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

func TestResolveFiles_InheritsPatches(t *testing.T) {
	patches := []string{"--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n"}
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{
					Path:    ".tagpr",
					Content: "original",
					Patches: patches,
				},
			},
		},
	}
	target := manifest.FileSetRepository{
		Name: "repo",
		Overrides: []manifest.FileEntry{
			// Override content but don't set Patches — should inherit
			{Path: ".tagpr", Content: "overridden"},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Content != "overridden" {
		t.Errorf("Content = %q, want %q", result[0].Content, "overridden")
	}
	if len(result[0].Patches) != 1 {
		t.Fatalf("Patches should be inherited, got %d patches", len(result[0].Patches))
	}
	if result[0].Patches[0] != patches[0] {
		t.Errorf("Patches[0] = %q, want %q", result[0].Patches[0], patches[0])
	}
}

func TestResolveFiles_OverridePatchesReplacesOriginal(t *testing.T) {
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{
					Path:    ".tagpr",
					Content: "original",
					Patches: []string{"original-patch"},
				},
			},
		},
	}
	target := manifest.FileSetRepository{
		Name: "repo",
		Overrides: []manifest.FileEntry{
			// Override provides its own patches — should NOT inherit original
			{Path: ".tagpr", Content: "overridden", Patches: []string{"override-patch"}},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if len(result[0].Patches) != 1 || result[0].Patches[0] != "override-patch" {
		t.Errorf("Patches should use override's patches, got %v", result[0].Patches)
	}
}

func TestResolveFiles_InheritsContentAndSource(t *testing.T) {
	fs := &manifest.FileSet{
		Spec: manifest.FileSetSpec{
			Files: []manifest.FileEntry{
				{
					Path:           ".goreleaser.yaml",
					Content:        "dummy message",
					Source:         "github://owner/repo/templates/.goreleaser.yaml",
					OriginalSource: "templates/.goreleaser.yaml",
				},
			},
		},
	}

	target := manifest.FileSetRepository{
		Name: "repo",
		Overrides: []manifest.FileEntry{
			{Path: ".goreleaser.yaml", Vars: map[string]string{"Description": "overwrite message"}},
		},
	}

	result := ResolveFiles(fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Content != "dummy message" {
		t.Errorf("Content = %q, want %q", result[0].Content, "dummy message")
	}
	if result[0].Source != "github://owner/repo/templates/.goreleaser.yaml" {
		t.Errorf("Source = %q, want %q", result[0].Source, "github://owner/repo/templates/.goreleaser.yaml")
	}
	if result[0].OriginalSource != "templates/.goreleaser.yaml" {
		t.Errorf("OriginalSource = %q, want %q", result[0].OriginalSource, "templates/.goreleaser.yaml")
	}
	if v := result[0].Vars["Description"]; v != "overwrite message" {
		t.Errorf("Vars.Description = %q, want %q", v, "overwrite message")
	}
}
