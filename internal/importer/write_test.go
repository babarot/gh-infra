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

func TestWrite_WritePatch_OnRepositoryOverride(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: repo-a
      overrides:
        - path: VERSION
          source: ./templates/common/VERSION
          sync_mode: create_only
  files:
    - path: VERSION
      source: ./templates/common/VERSION
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
				YAMLPath:     "$.spec.repositories[0].overrides[0]",
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
	if !strings.Contains(out, "overrides:") {
		t.Fatalf("expected overrides block to remain:\n%s", out)
	}
	if !strings.Contains(out, "sync_mode: create_only") {
		t.Fatalf("expected deprecated sync_mode in override to be preserved:\n%s", out)
	}
	if !strings.Contains(out, "patches:") {
		t.Fatalf("expected patches to be added under override entry:\n%s", out)
	}
}

func TestWrite_MixedInlineAndPatchInSameManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  files:
    - path: VERSION
      source: ./templates/common/VERSION
      sync_mode: create_only
    - path: config.txt
      content: old
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
			{
				Type:         fileset.ChangeUpdate,
				WriteMode:    WriteInline,
				ManifestPath: manifestPath,
				DocIndex:     0,
				YAMLPath:     "$.spec.files[1].content",
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
	out := string(got)
	if !strings.Contains(out, "patches:") {
		t.Fatalf("expected patch entry to be written:\n%s", out)
	}
	if !strings.Contains(out, "content: new content") {
		t.Fatalf("expected inline edit to be written:\n%s", out)
	}
	if !strings.Contains(out, "sync_mode: create_only") {
		t.Fatalf("expected unrelated patch entry fields to remain:\n%s", out)
	}
}

func TestPatchEditOps_AddPatchSequence(t *testing.T) {
	data := []byte(`kind: File
spec:
  files:
    - path: VERSION
      source: ./templates/common/VERSION
`)

	ops, err := patchEditOps(data, Change{
		DocIndex:     0,
		YAMLPath:     "$.spec.files[0]",
		PatchContent: "--- a/VERSION\n+++ b/VERSION\n@@ -1 +1 @@\n-0.1.0\n+v1.2.6\n",
	})
	if err != nil {
		t.Fatalf("patchEditOps error: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 edit ops, got %d", len(ops))
	}
	if ops[0].kind != editMerge {
		t.Fatalf("expected first op to merge patches field, got %s", ops[0].kind)
	}
	if ops[1].kind != editSetLiteral {
		t.Fatalf("expected second op to set literal patch content, got %s", ops[1].kind)
	}
}

func TestApplyEditOps_PatchAndInline(t *testing.T) {
	data := []byte(`kind: FileSet
spec:
  files:
    - path: VERSION
      source: ./templates/common/VERSION
    - path: config.txt
      content: old
`)

	updated, err := applyEditOps(data, []editOp{
		{
			kind:     editMerge,
			docIndex: 0,
			path:     "$.spec.files[0]",
			value: map[string]any{
				"patches": []string{"--- a/VERSION\n+++ b/VERSION\n"},
			},
		},
		{
			kind:     editSetLiteral,
			docIndex: 0,
			path:     "$.spec.files[0].patches[0]",
			value:    "--- a/VERSION\n+++ b/VERSION\n",
		},
		{
			kind:     editSet,
			docIndex: 0,
			path:     "$.spec.files[1].content",
			value:    "new content",
		},
	})
	if err != nil {
		t.Fatalf("applyEditOps error: %v", err)
	}

	out := string(updated)
	if !strings.Contains(out, "patches:") {
		t.Fatalf("expected patches field to be added:\n%s", out)
	}
	if !strings.Contains(out, "content: new content") {
		t.Fatalf("expected inline content to be updated:\n%s", out)
	}
}

func TestWrite_WritePatch_UpdatesExistingPatchWithoutChangingDeprecatedField(t *testing.T) {
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
          @@ -1 +1 @@
          -0.1.0
          +v1.2.5
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{{
			Type:         fileset.ChangeUpdate,
			WriteMode:    WritePatch,
			ManifestPath: manifestPath,
			DocIndex:     0,
			YAMLPath:     "$.spec.files[0]",
			Path:         "VERSION",
			PatchContent: "--- a/VERSION\n+++ b/VERSION\n@@ -1 +1 @@\n-0.1.0\n+v1.2.6\n",
			PatchEntry:   &manifest.FileEntry{Path: "VERSION"},
		}},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	out, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	result := string(out)
	if !strings.Contains(result, "sync_mode: create_only") {
		t.Fatalf("expected deprecated field to remain untouched:\n%s", result)
	}
	if strings.Contains(result, "+v1.2.5") {
		t.Fatalf("expected old patch content to be replaced:\n%s", result)
	}
	if !strings.Contains(result, "+v1.2.6") {
		t.Fatalf("expected new patch content to be written:\n%s", result)
	}
}

func TestWrite_WritePatch_PreservesSiblingOverrideFields(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: repo-a
      overrides:
        - path: VERSION
          source: ./templates/common/VERSION
          sync_mode: create_only
          vars: {version: current}
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{{
			Type:         fileset.ChangeUpdate,
			WriteMode:    WritePatch,
			ManifestPath: manifestPath,
			DocIndex:     0,
			YAMLPath:     "$.spec.repositories[0].overrides[0]",
			Path:         "VERSION",
			PatchContent: "--- a/VERSION\n+++ b/VERSION\n@@ -1 +1 @@\n-0.1.0\n+v1.2.6\n",
			PatchEntry:   &manifest.FileEntry{Path: "VERSION"},
		}},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	out, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	result := string(out)
	if !strings.Contains(result, "vars: {version: current}") {
		t.Fatalf("expected sibling override field to remain untouched:\n%s", result)
	}
	if !strings.Contains(result, "patches:") {
		t.Fatalf("expected patches field to be added:\n%s", result)
	}
}

func TestWrite_WritePatch_UpdatesExistingOverridePatch(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: repo-a
      overrides:
        - path: VERSION
          source: ./templates/common/VERSION
          sync_mode: create_only
          patches:
            - |
              --- a/VERSION
              +++ b/VERSION
              @@ -1 +1 @@
              -0.1.0
              +v1.2.5
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{{
			Type:         fileset.ChangeUpdate,
			WriteMode:    WritePatch,
			ManifestPath: manifestPath,
			DocIndex:     0,
			YAMLPath:     "$.spec.repositories[0].overrides[0]",
			Path:         "VERSION",
			PatchContent: "--- a/VERSION\n+++ b/VERSION\n@@ -1 +1 @@\n-0.1.0\n+v1.2.6\n",
			PatchEntry:   &manifest.FileEntry{Path: "VERSION"},
		}},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	out, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	result := string(out)
	if strings.Contains(result, "+v1.2.5") {
		t.Fatalf("expected old override patch content to be replaced:\n%s", result)
	}
	if !strings.Contains(result, "+v1.2.6") {
		t.Fatalf("expected new override patch content to be written:\n%s", result)
	}
	if !strings.Contains(result, "sync_mode: create_only") {
		t.Fatalf("expected deprecated field to remain:\n%s", result)
	}
}

func TestWrite_MultiDocumentTargetsCorrectDocument(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo-a
spec:
  files:
    - path: a.txt
      content: old-a
---
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo-b
spec:
  files:
    - path: b.txt
      content: old-b
`)
	if err := os.WriteFile(manifestPath, yamlData, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	plan := &Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []Change{{
			Type:         fileset.ChangeUpdate,
			WriteMode:    WriteInline,
			ManifestPath: manifestPath,
			DocIndex:     1,
			YAMLPath:     "$.spec.files[0].content",
			Desired:      "new-b",
		}},
	}

	if err := Write(plan); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	out, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	result := string(out)
	if !strings.Contains(result, "content: old-a") {
		t.Fatalf("expected first document to remain unchanged:\n%s", result)
	}
	if !strings.Contains(result, "content: new-b") {
		t.Fatalf("expected second document to be updated:\n%s", result)
	}
}
