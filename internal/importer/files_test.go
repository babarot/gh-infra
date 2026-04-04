package importer

import (
	"context"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

func TestPlanImportEntry_WriteSource(t *testing.T) {
	file := manifest.FileEntry{
		Path:           ".github/workflows/ci.yaml",
		Content:        "old content",
		OriginalSource: "/tmp/local/ci.yaml",
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
		DocIndex:   0,
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	if change.WriteMode != WriteSource {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
	if change.LocalTarget != "/tmp/local/ci.yaml" {
		t.Errorf("LocalTarget = %q, want %q", change.LocalTarget, "/tmp/local/ci.yaml")
	}
}

func TestPlanImportEntry_WriteInline(t *testing.T) {
	file := manifest.FileEntry{
		Path:    ".github/dependabot.yaml",
		Content: "inline content",
		// No OriginalSource → inline
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
		DocIndex:   2,
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 3, doc, 1)

	if change.WriteMode != WriteInline {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteInline)
	}
	if change.ManifestPath != "/tmp/manifest.yaml" {
		t.Errorf("ManifestPath = %q, want %q", change.ManifestPath, "/tmp/manifest.yaml")
	}
	if change.DocIndex != 2 {
		t.Errorf("DocIndex = %d, want 2", change.DocIndex)
	}
	if change.YAMLPath != "$.spec.files[3].content" {
		t.Errorf("YAMLPath = %q, want %q", change.YAMLPath, "$.spec.files[3].content")
	}
}

func TestPlanImportEntry_SkipGitHubSource(t *testing.T) {
	file := manifest.FileEntry{
		Path:   ".github/workflows/ci.yaml",
		Source: "github://other-org/templates/ci.yaml",
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
	if change.Reason == "" {
		t.Error("Reason should be set for skipped entries")
	}
}

func TestPlanImportEntry_SkipVars(t *testing.T) {
	file := manifest.FileEntry{
		Path:    "README.md",
		Content: "{{ .repo_name }}",
		Vars:    map[string]string{"repo_name": "test"},
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
	if change.Reason != "uses template variables" {
		t.Errorf("Reason = %q, want 'uses template variables'", change.Reason)
	}
}

func TestPlanImportEntry_SkipPatches(t *testing.T) {
	file := manifest.FileEntry{
		Path:    "config.yaml",
		Content: "base content",
		Patches: []string{"--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new"},
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
}

func TestPlanImportEntry_CreateOnly_NotSkipped(t *testing.T) {
	file := manifest.FileEntry{
		Path:           "CODEOWNERS",
		Content:        "* @team",
		Reconcile:      manifest.ReconcileCreateOnly,
		OriginalSource: "/tmp/CODEOWNERS",
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	// create_only should NOT be skipped — importing updates the local master template.
	if change.WriteMode == WriteSkip {
		t.Errorf("WriteMode should not be WriteSkip for create_only, got %q", change.WriteMode)
	}
	if change.WriteMode != WriteSource {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
}

func TestPlanImportEntry_SharedSourceWarning(t *testing.T) {
	file := manifest.FileEntry{
		Path:           ".github/workflows/ci.yaml",
		Content:        "content",
		OriginalSource: "/tmp/shared/ci.yaml",
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	// repoCount = 3 → shared source warning
	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 3)

	if change.WriteMode != WriteSource {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
	if len(change.Warnings) == 0 {
		t.Error("expected warning for shared source")
	}
	found := false
	for _, w := range change.Warnings {
		if w == "shared source: affects 3 repositories" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'shared source: affects 3 repositories' warning, got %v", change.Warnings)
	}
}

func TestPlanImportEntry_NoDiff(t *testing.T) {
	// When GitHub content matches local, Type should be NoOp.
	// We can't easily test with a real runner, but we can verify that
	// when the GitHub fetch fails (nil runner), it's also NoOp.
	file := manifest.FileEntry{
		Path:           "test.txt",
		Content:        "local content",
		OriginalSource: "/tmp/test.txt",
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	// nil runner → fetch will fail → NoOp
	change := planImportEntry(context.TODO(), nil, "org/repo", file, 0, doc, 1)

	if change.Type != fileset.ChangeNoOp {
		t.Errorf("Type = %q, want %q", change.Type, fileset.ChangeNoOp)
	}
}
