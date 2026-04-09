package importer

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// helper to call planImportEntry with default repo values for tests that don't need them.
func callPlan(ctx context.Context, runner gh.Runner, fullName string, file manifest.FileEntry, doc *manifest.FileDocument, repoCount int) Change {
	repo := manifest.FileSetRepository{Name: "repo"}
	return planImportEntry(ctx, runner, fullName, file, importEntryContext{
		Doc:       doc,
		RepoIndex: 0,
		Repo:      repo,
		RepoCount: repoCount,
	})
}

// callPlanShared is like callPlan but marks the source as shared.
func callPlanShared(ctx context.Context, runner gh.Runner, fullName string, file manifest.FileEntry, doc *manifest.FileDocument, repoCount int) Change {
	repo := manifest.FileSetRepository{Name: "repo"}
	return planImportEntry(ctx, runner, fullName, file, importEntryContext{
		Doc:       doc,
		RepoIndex: 0,
		Repo:      repo,
		RepoCount: repoCount,
		Shared:    true,
	})
}

func TestPlanImportEntry_ExclusiveSourceUsesWriteSource(t *testing.T) {
	// A source used by only one entry should use WriteSource (direct update).
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

	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSource {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
}

func TestPlanImportEntry_WriteInline(t *testing.T) {
	file := manifest.FileEntry{
		Path:    ".github/dependabot.yaml",
		Content: "inline content",
		// No OriginalSource → inline
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{
					{Path: "other.yaml"},
					{Path: "another.yaml"},
					{Path: "third.yaml"},
					{Path: ".github/dependabot.yaml"},
				},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
		DocIndex:   2,
	}

	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

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

	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
	if change.Reason == "" {
		t.Error("Reason should be set for skipped entries")
	}
}

func TestPlanImportEntry_UnsupportedTemplateSyntaxSkips(t *testing.T) {
	file := manifest.FileEntry{
		Path:    "README.md",
		Content: "<% if .Repo.Name %>enabled<% end %>\n",
		Vars:    map[string]string{"repo_name": "test"},
	}
	doc := &manifest.FileDocument{
		Resource:   &manifest.FileSet{},
		SourcePath: "/tmp/manifest.yaml",
	}

	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
	if change.Reason != "cannot safely write back to template" {
		t.Errorf("Reason = %q, want 'cannot safely write back to template'", change.Reason)
	}
}

func TestPlanImportEntry_PatchesUsesWritePatch(t *testing.T) {
	// Files with patches should use WritePatch mode, not WriteSkip.
	file := manifest.FileEntry{
		Path:           "config.yaml",
		Content:        "base content\n",
		Patches:        []string{"--- a/config.yaml\n+++ b/config.yaml\n@@ -1 +1 @@\n-base content\n+patched content\n"},
		OriginalSource: "/tmp/template/config.yaml",
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{{Path: "config.yaml"}},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
	}

	// nil runner → fetch will fail → NoOp, but WriteMode should be WritePatch
	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode != WritePatch {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WritePatch)
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

	// Non-shared source + create_only → WriteSource (direct update).
	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode == WriteSkip {
		t.Errorf("WriteMode should not be WriteSkip for create_only, got %q", change.WriteMode)
	}
	if change.WriteMode != WriteSource {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
}

func TestPlanImportEntry_SharedSourceUsesWritePatch(t *testing.T) {
	file := manifest.FileEntry{
		Path:           ".github/workflows/ci.yaml",
		Content:        "content",
		OriginalSource: "/tmp/shared/ci.yaml",
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{{Path: ".github/workflows/ci.yaml"}},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
	}

	// shared=true → should use WritePatch instead of WriteSource
	change := callPlanShared(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.WriteMode != WritePatch {
		t.Errorf("WriteMode = %q, want %q", change.WriteMode, WritePatch)
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
	change := callPlan(context.TODO(), nil, "org/repo", file, doc, 1)

	if change.Type != fileset.ChangeNoOp {
		t.Errorf("Type = %q, want %q", change.Type, fileset.ChangeNoOp)
	}
}

func TestDiffFiles_ReportsPerFileStatus(t *testing.T) {
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Metadata: manifest.FileSetMetadata{Owner: "org"},
			Spec: manifest.FileSetSpec{
				Repositories: []manifest.FileSetRepository{{Name: "repo"}},
				Files: []manifest.FileEntry{
					{Path: "a.txt", Content: "a"},
					{Path: "b.txt", Content: "b"},
				},
			},
		},
	}

	var statuses []string
	_, err := DiffFiles(context.TODO(), nil, []*manifest.FileDocument{doc}, DiffFilesOptions{
		FilterRepo:     "org/repo",
		SourceRefCount: map[string]int{},
		OnStatus: func(status string) {
			statuses = append(statuses, status)
		},
	})
	if err != nil {
		t.Fatalf("DiffFiles error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d: %v", len(statuses), statuses)
	}
	if statuses[0] != "fetching file a.txt..." {
		t.Fatalf("first status = %q, want %q", statuses[0], "fetching file a.txt...")
	}
	if statuses[1] != "fetching file b.txt..." {
		t.Fatalf("second status = %q, want %q", statuses[1], "fetching file b.txt...")
	}
	if strings.Contains(strings.Join(statuses, "\n"), "comparing files") {
		t.Fatalf("unexpected coarse status in statuses: %v", statuses)
	}
}

func TestPlanImportEntry_TemplateSyntaxImportsLiteralDiff(t *testing.T) {
	file := manifest.FileEntry{
		Path:           "go.mod",
		Content:        "module github.com/<% .Repo.FullName %>\n\ngo 1.26.0\n",
		OriginalSource: "/tmp/templates/go.mod",
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{{Path: "go.mod"}},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
	}
	runner := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/org/repo/contents/go.mod": []byte(`{"content":"` + base64.StdEncoding.EncodeToString([]byte("module github.com/org/repo\n\ngo 1.27.0\n\nrequire example.com/foo v1.2.3\n")) + `","encoding":"base64"}`),
		},
	}

	change := callPlan(context.TODO(), runner, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSource {
		t.Fatalf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
	if change.Type != fileset.ChangeUpdate {
		t.Fatalf("Type = %q, want %q", change.Type, fileset.ChangeUpdate)
	}
	want := "module github.com/<% .Repo.FullName %>\n\ngo 1.27.0\n\nrequire example.com/foo v1.2.3\n"
	if change.Desired != want {
		t.Fatalf("Desired = %q, want %q", change.Desired, want)
	}
	if change.Reason != "" {
		t.Fatalf("Reason = %q, want empty", change.Reason)
	}
}

func TestPlanImportEntry_ChangedVarsPlaceholderSkips(t *testing.T) {
	file := manifest.FileEntry{
		Path:           "Makefile",
		Content:        "GO_VERSION=<% .Vars.go_version %>\nTOOL=old\n",
		Vars:           map[string]string{"go_version": "1.26.1"},
		OriginalSource: "/tmp/templates/Makefile",
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{{Path: "Makefile"}},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
	}
	runner := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/org/repo/contents/Makefile": []byte(`{"content":"` + base64.StdEncoding.EncodeToString([]byte("GO_VERSION=1.27.3\nTOOL=new\n")) + `","encoding":"base64"}`),
		},
	}

	change := callPlan(context.TODO(), runner, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSkip {
		t.Fatalf("WriteMode = %q, want %q", change.WriteMode, WriteSkip)
	}
	if change.Reason != "cannot safely write back to template" {
		t.Fatalf("Reason = %q, want %q", change.Reason, "cannot safely write back to template")
	}
}

func TestPlanImportEntry_TemplateLineLiteralChangeAroundPlaceholder(t *testing.T) {
	file := manifest.FileEntry{
		Path:           ".gitignore",
		Content:        "/<% .Repo.Name %>\ncoverage.out\n",
		OriginalSource: "/tmp/templates/.gitignore",
	}
	doc := &manifest.FileDocument{
		Resource: &manifest.FileSet{
			Spec: manifest.FileSetSpec{
				Files: []manifest.FileEntry{{Path: ".gitignore"}},
			},
		},
		SourcePath: "/tmp/manifest.yaml",
	}
	runner := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/org/repo/contents/.gitignore": []byte(`{"content":"` + base64.StdEncoding.EncodeToString([]byte("repo*\ndist\ncoverage.out\n")) + `","encoding":"base64"}`),
		},
	}

	change := callPlan(context.TODO(), runner, "org/repo", file, doc, 1)

	if change.WriteMode != WriteSource {
		t.Fatalf("WriteMode = %q, want %q", change.WriteMode, WriteSource)
	}
	if change.Type != fileset.ChangeUpdate {
		t.Fatalf("Type = %q, want %q", change.Type, fileset.ChangeUpdate)
	}
	want := "<% .Repo.Name %>*\ndist\ncoverage.out\n"
	if change.Desired != want {
		t.Fatalf("Desired = %q, want %q", change.Desired, want)
	}
	if change.Reason != "" {
		t.Fatalf("Reason = %q, want empty", change.Reason)
	}
}
