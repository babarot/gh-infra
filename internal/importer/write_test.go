package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

func TestWrite_ManifestEdits(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	data := []byte("apiVersion: gh-infra/v1\nkind: Repository\nspec:\n  description: updated\n")

	plan := &Result{
		ManifestEdits: map[string][]byte{
			manifestPath: data,
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("manifest content = %q, want %q", string(got), string(data))
	}
}

func TestWrite_WriteSource(t *testing.T) {
	dir := t.TempDir()
	localTarget := filepath.Join(dir, "ci.yaml")

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:        fileset.ChangeUpdate,
				WriteMode:   WriteSource,
				LocalTarget: localTarget,
				Desired:     "name: CI\non: push\n",
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := os.ReadFile(localTarget)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "name: CI\non: push\n" {
		t.Errorf("source content = %q, want %q", string(got), "name: CI\non: push\n")
	}
}

func TestWrite_WriteInline(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  files:
    - path: test.txt
      content: old content
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:         fileset.ChangeUpdate,
				WriteMode:    WriteInline,
				ManifestPath: manifestPath,
				DocIndex:     0,
				YAMLPath:     "$.spec.files[0].content",
				Desired:      "new content",
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if !strings.Contains(string(got), "new content") {
		t.Errorf("manifest should contain 'new content', got:\n%s", string(got))
	}
}

func TestWrite_WriteSkip(t *testing.T) {
	dir := t.TempDir()
	localTarget := filepath.Join(dir, "should-not-exist.txt")

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:        fileset.ChangeUpdate,
				WriteMode:   WriteSkip,
				LocalTarget: localTarget,
				Desired:     "should not be written",
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if _, err := os.Stat(localTarget); !os.IsNotExist(err) {
		t.Error("WriteSkip entry should not create a file")
	}
}

func TestWrite_NoOpSkipped(t *testing.T) {
	dir := t.TempDir()
	localTarget := filepath.Join(dir, "noop-file.txt")

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:        fileset.ChangeNoOp,
				WriteMode:   WriteSource,
				LocalTarget: localTarget,
				Desired:     "should not be written",
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if _, err := os.Stat(localTarget); !os.IsNotExist(err) {
		t.Error("NoOp entry should not create a file")
	}
}

func TestWrite_WritePatch_PreservesExistingEntryShape(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: VERSION
      source: ./templates/common/VERSION
      sync_mode: create_only
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:         fileset.ChangeUpdate,
				WriteMode:    WritePatch,
				ManifestPath: manifestPath,
				DocIndex:     0,
				YAMLPath:     "$.spec.files[0]",
				Path:         "VERSION",
				PatchContent: "--- a/VERSION\n+++ b/VERSION\n@@ -1 +1 @@\n-0.1.0\n+v1.2.6\n",
				PatchEntry:   &manifest.FileEntry{Path: "VERSION"},
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "sync_mode: create_only") {
		t.Fatalf("expected deprecated sync_mode to be preserved:\n%s", out)
	}
	if strings.Contains(out, "reconcile: create_only") {
		t.Fatalf("did not expect reconcile to be introduced:\n%s", out)
	}
	if !strings.Contains(out, "source: ./templates/common/VERSION") {
		t.Fatalf("expected source to be preserved:\n%s", out)
	}
	if !strings.Contains(out, "patches:") {
		t.Fatalf("expected patches field to be added:\n%s", out)
	}
	if !strings.Contains(out, "+v1.2.6") {
		t.Fatalf("expected patch content to be written:\n%s", out)
	}
}

func TestWrite_WritePatch_DeleteOnlyPatchesField(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: VERSION
      source: ./templates/common/VERSION
      sync_mode: create_only
      patches:
        - |
          --- a/VERSION
          +++ b/VERSION
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{
			{
				Type:         fileset.ChangeUpdate,
				WriteMode:    WritePatch,
				ManifestPath: manifestPath,
				DocIndex:     0,
				YAMLPath:     "$.spec.files[0]",
				Path:         "VERSION",
				PatchContent: "",
				PatchEntry:   &manifest.FileEntry{Path: "VERSION"},
			},
		},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	out := string(got)
	if strings.Contains(out, "patches:") {
		t.Fatalf("expected only patches to be removed:\n%s", out)
	}
	if !strings.Contains(out, "sync_mode: create_only") {
		t.Fatalf("expected sync_mode to remain:\n%s", out)
	}
	if !strings.Contains(out, "source: ./templates/common/VERSION") {
		t.Fatalf("expected source to remain:\n%s", out)
	}
}
