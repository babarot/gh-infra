package infra

import (
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

// --- parseArgs tests ---

func TestParseArgs_Valid(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []struct{ owner, repo string }
	}{
		{
			name: "single repo",
			args: []string{"org/repo"},
			want: []struct{ owner, repo string }{{"org", "repo"}},
		},
		{
			name: "multiple repos",
			args: []string{"org/repo1", "other/repo2"},
			want: []struct{ owner, repo string }{{"org", "repo1"}, {"other", "repo2"}},
		},
		{
			name: "repo with dots",
			args: []string{"org/my.repo"},
			want: []struct{ owner, repo string }{{"org", "my.repo"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d targets, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].Target.Owner != w.owner || got[i].Target.Name != w.repo {
					t.Errorf("target[%d] = %s/%s, want %s/%s", i, got[i].Target.Owner, got[i].Target.Name, w.owner, w.repo)
				}
			}
		})
	}
}

func TestParseArgs_Invalid(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no slash", args: []string{"noslash"}},
		{name: "empty owner", args: []string{"/repo"}},
		{name: "empty repo", args: []string{"owner/"}},
		{name: "just slash", args: []string{"/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.args)
			if err == nil {
				t.Error("expected error for invalid arg")
			}
		})
	}
}

// --- localPath tests ---

func TestLocalPath_WriteSource(t *testing.T) {
	c := importer.Change{
		LocalTarget:  "/tmp/local/ci.yaml",
		ManifestPath: "/tmp/manifest.yaml",
		Path:         ".github/workflows/ci.yaml",
	}
	got := localPath(c)
	if got != "/tmp/local/ci.yaml" {
		t.Errorf("localPath = %q, want %q", got, "/tmp/local/ci.yaml")
	}
}

func TestLocalPath_WriteInline(t *testing.T) {
	c := importer.Change{
		ManifestPath: "/tmp/manifest.yaml",
		Path:         ".github/dependabot.yaml",
	}
	got := localPath(c)
	if got != "/tmp/manifest.yaml:.github/dependabot.yaml" {
		t.Errorf("localPath = %q, want %q", got, "/tmp/manifest.yaml:.github/dependabot.yaml")
	}
}

func TestLocalPath_FallbackToPath(t *testing.T) {
	c := importer.Change{
		Path: "some/file.txt",
	}
	got := localPath(c)
	if got != "some/file.txt" {
		t.Errorf("localPath = %q, want %q", got, "some/file.txt")
	}
}

// --- formatImportValue tests ---

func TestFormatImportValue_String(t *testing.T) {
	got := formatImportValue("hello")
	if got != "hello" {
		t.Errorf("formatImportValue = %q, want %q", got, "hello")
	}
}

func TestFormatImportValue_Bool(t *testing.T) {
	if got := formatImportValue(true); got != "true" {
		t.Errorf("formatImportValue(true) = %q, want %q", got, "true")
	}
	if got := formatImportValue(false); got != "false" {
		t.Errorf("formatImportValue(false) = %q, want %q", got, "false")
	}
}

func TestFormatImportValue_Nil(t *testing.T) {
	got := formatImportValue(nil)
	if got != "(none)" {
		t.Errorf("formatImportValue(nil) = %q, want %q", got, "(none)")
	}
}

func TestFormatImportValue_Struct(t *testing.T) {
	bp := manifest.BranchProtection{
		Pattern:         "main",
		RequiredReviews: manifest.Ptr(2),
	}
	got := formatImportValue(bp)
	if got == "" || got == "(none)" {
		t.Errorf("formatImportValue(struct) should produce YAML, got %q", got)
	}
}

func TestFormatImportValue_Slice(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := formatImportValue(items)
	if got == "" || got == "(none)" {
		t.Errorf("formatImportValue(slice) should produce YAML, got %q", got)
	}
}

// --- ImportDiff.DiffEntries tests ---

func TestImportDiff_DiffEntries_FiltersSkip(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
			FileChanges: []importer.Change{
				{
					Target:      "org/repo",
					Path:        "skipped.txt",
					Type:        fileset.ChangeUpdate,
					WriteMode:   importer.WriteSkip,
					LocalTarget: "/tmp/skipped.txt",
					Current:     "old",
					Desired:     "new",
				},
			},
		},
	}

	entries := diff.DiffEntries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (skip filtered), got %d", len(entries))
	}
}

func TestImportDiff_DiffEntries_IncludesUpdate(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
			FileChanges: []importer.Change{
				{
					Target:      "org/repo",
					Path:        "ci.yaml",
					Type:        fileset.ChangeUpdate,
					WriteMode:   importer.WriteSource,
					LocalTarget: "/tmp/ci.yaml",
					Current:     "old",
					Desired:     "new",
				},
			},
		},
	}

	entries := diff.DiffEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Path != "/tmp/ci.yaml" {
		t.Errorf("Path = %q, want %q", entries[0].Path, "/tmp/ci.yaml")
	}
	if entries[0].Current != "old" {
		t.Errorf("Current = %q, want %q", entries[0].Current, "old")
	}
	if entries[0].Desired != "new" {
		t.Errorf("Desired = %q, want %q", entries[0].Desired, "new")
	}
}

func TestImportDiff_DiffEntries_NilPlan(t *testing.T) {
	diff := &ImportDiff{Plan: nil}
	entries := diff.DiffEntries()
	if entries != nil {
		t.Errorf("expected nil entries for nil Plan, got %v", entries)
	}
}

// --- ImportDiff.MarkSkips tests ---

func TestImportDiff_MarkSkips(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
			FileChanges: []importer.Change{
				{
					Target:      "org/repo",
					Path:        "ci.yaml",
					Type:        fileset.ChangeUpdate,
					WriteMode:   importer.WriteSource,
					LocalTarget: "/tmp/ci.yaml",
				},
				{
					Target:      "org/repo",
					Path:        "lint.yaml",
					Type:        fileset.ChangeUpdate,
					WriteMode:   importer.WriteSource,
					LocalTarget: "/tmp/lint.yaml",
				},
			},
		},
	}

	// Mark the first entry as skipped via DiffEntry
	entries := []ui.DiffEntry{
		{Target: "org/repo", Path: "/tmp/ci.yaml", Skip: true},
	}
	diff.MarkSkips(entries)

	if diff.Plan.FileChanges[0].Type != fileset.ChangeNoOp {
		t.Errorf("FileChanges[0].Type = %q, want %q", diff.Plan.FileChanges[0].Type, fileset.ChangeNoOp)
	}
	if diff.Plan.FileChanges[1].Type != fileset.ChangeUpdate {
		t.Errorf("FileChanges[1].Type = %q, want %q (should not be skipped)", diff.Plan.FileChanges[1].Type, fileset.ChangeUpdate)
	}
}

// --- ImportDiff.HasChanges tests ---

func TestImportDiff_HasChanges_True(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			RepoDiffs: []importer.FieldDiff{
				{Field: "description", Old: "old", New: "new"},
			},
			ManifestEdits: make(map[string][]byte),
		},
	}
	if !diff.HasChanges() {
		t.Error("expected HasChanges to be true")
	}
}

func TestImportDiff_HasChanges_False(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
		},
	}
	if diff.HasChanges() {
		t.Error("expected HasChanges to be false")
	}
}

func TestImportDiff_HasChanges_NilPlan(t *testing.T) {
	diff := &ImportDiff{Plan: nil}
	if diff.HasChanges() {
		t.Error("expected HasChanges to be false with nil Plan")
	}
}
