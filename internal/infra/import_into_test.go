package infra

import (
	"bytes"
	"strings"
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

// --- allRepoFullNames tests ---

func TestAllRepoFullNames_ReposAndFileSets(t *testing.T) {
	parsed := &manifest.ParseResult{
		RepositoryDocs: []*manifest.RepositoryDocument{
			{Resource: &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-a"}}},
			{Resource: &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-b"}}},
		},
		FileDocs: []*manifest.FileDocument{
			{Resource: &manifest.FileSet{
				Metadata: manifest.FileSetMetadata{Owner: "org"},
				Spec: manifest.FileSetSpec{
					Repositories: []manifest.FileSetRepository{
						{Name: "repo-b"}, // duplicate with RepositoryDocs
						{Name: "repo-c"}, // new
					},
				},
			}},
		},
	}

	names := allRepoFullNames(parsed)

	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d: %v", len(names), names)
	}
	want := map[string]bool{"org/repo-a": true, "org/repo-b": true, "org/repo-c": true}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected name: %q", n)
		}
	}
}

func TestAllRepoFullNames_Empty(t *testing.T) {
	parsed := &manifest.ParseResult{}
	names := allRepoFullNames(parsed)
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

// --- fieldDiffsToDiffGroups tests ---

func TestFieldDiffsToDiffGroups_BareFields(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "description", Old: `""`, New: "My repo"},
		{Field: "visibility", Old: "private", New: "public"},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.Header != "" {
			t.Errorf("bare field should have empty header, got %q", g.Header)
		}
	}
}

func TestFieldDiffsToDiffGroups_NestedObject(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "features.issues", Old: false, New: true},
		{Field: "features.wiki", Old: true, New: false},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Header != "features" {
		t.Errorf("expected header 'features', got %q", groups[0].Header)
	}
	if len(groups[0].Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(groups[0].Items))
	}
	if groups[0].Items[0].Field != "issues" {
		t.Errorf("expected field 'issues', got %q", groups[0].Items[0].Field)
	}
}

func TestFieldDiffsToDiffGroups_KeyedCollection(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "branch_protection.main.required_reviews", Old: nil, New: 1},
		{Field: "branch_protection.develop.required_reviews", Old: nil, New: 2},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (one per key), got %d", len(groups))
	}
	if groups[0].Header != `branch protection "main"` {
		t.Errorf("expected human branch protection header, got %q", groups[0].Header)
	}
	if groups[0].Items[0].Field != "required_reviews" {
		t.Errorf("expected child field required_reviews, got %q", groups[0].Items[0].Field)
	}
	if groups[1].Header != `branch protection "develop"` {
		t.Errorf("expected human branch protection header, got %q", groups[1].Header)
	}
}

func TestFieldDiffsToDiffGroups_FlatCollection(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "labels.kind/bug", Old: nil, New: `#d73a4a "A bug"`},
		{Field: "labels.kind/feature", Old: nil, New: `#425df5 "A feature"`},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Header != "labels" {
		t.Errorf("expected header 'labels', got %q", groups[0].Header)
	}
	if len(groups[0].Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(groups[0].Items))
	}
	if groups[0].Items[0].Field != "kind/bug" {
		t.Errorf("expected field 'kind/bug', got %q", groups[0].Items[0].Field)
	}
}

func TestFieldDiffsToDiffGroups_DeepNesting(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "actions.enabled", Old: false, New: true},
		{Field: "actions.selected_actions.github_owned_allowed", Old: false, New: true},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Header != "actions" {
		t.Errorf("expected header 'actions', got %q", groups[0].Header)
	}
	if groups[0].Items[1].Field != "selected_actions.github_owned_allowed" {
		t.Errorf("expected deep field, got %q", groups[0].Items[1].Field)
	}
}

func TestFieldDiffsToDiffGroups_Mixed(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "description", Old: `""`, New: "test"},
		{Field: "features.issues", Old: false, New: true},
		{Field: "branch_protection.main", Old: nil, New: "reviews: 1"},
		{Field: "labels.bug", Old: nil, New: "#d73a4a"},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(groups))
	}
	headers := []string{"", "features", `branch protection "main"`, "labels"}
	for i, want := range headers {
		if groups[i].Header != want {
			t.Errorf("group[%d] header = %q, want %q", i, groups[i].Header, want)
		}
	}
}

func TestFieldDiffsToDiffGroups_UnknownPrefix(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "future_resource.field1", Old: nil, New: "value"},
		{Field: "future_resource.field2", Old: nil, New: "value"},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Header != "future_resource" {
		t.Errorf("expected header 'future_resource', got %q", groups[0].Header)
	}
	if len(groups[0].Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(groups[0].Items))
	}
}

func TestFieldDiffsToDiffGroups_Empty(t *testing.T) {
	groups := fieldDiffsToDiffGroups(nil)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestFieldDiffsToDiffGroups_Icons(t *testing.T) {
	diffs := []importer.FieldDiff{
		{Field: "labels.new-label", Old: nil, New: "#FF0000"},
		{Field: "labels.removed", Old: "#00FF00", New: nil},
	}
	groups := fieldDiffsToDiffGroups(diffs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Icon != ui.IconChange {
		t.Errorf("mixed add/remove should be IconChange, got %q", groups[0].Icon)
	}
	if groups[0].Items[0].Icon != ui.IconAdd {
		t.Errorf("new label should be IconAdd, got %q", groups[0].Items[0].Icon)
	}
	if groups[0].Items[1].Icon != ui.IconRemove {
		t.Errorf("removed label should be IconRemove, got %q", groups[0].Items[1].Icon)
	}
	if groups[0].Items[0].Value != "#FF0000" {
		t.Errorf("new label Value = %v, want #FF0000", groups[0].Items[0].Value)
	}
	if groups[0].Items[1].Value != "#00FF00" {
		t.Errorf("removed label Value = %v, want #00FF00", groups[0].Items[1].Value)
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

func TestImportDiff_DiffEntries_CreateOnlyDefaultsToSkip(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
			FileChanges: []importer.Change{
				{
					Target:             "org/repo",
					Path:               "VERSION",
					Type:               fileset.ChangeUpdate,
					WriteMode:          importer.WriteSource,
					SuggestedWriteMode: importer.WriteSource,
					AvailableModes:     []importer.WriteMode{importer.WriteSource, importer.WritePatch},
					LocalTarget:        "/tmp/VERSION",
					Current:            "0.1.0\n",
					WriteCurrent:       "0.1.0\n",
					PatchCurrent:       "0.1.0\n",
					Desired:            "v1.2.6\n",
					CreateOnly:         true,
				},
			},
		},
	}

	entries := diff.DiffEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "skip" {
		t.Fatalf("Action = %q, want skip", entries[0].Action)
	}
}

func TestImportDiff_DiffEntries_CreateOnlyWithExistingPatchDefaultsToPatch(t *testing.T) {
	diff := &ImportDiff{
		Plan: &importer.Result{
			ManifestEdits: make(map[string][]byte),
			FileChanges: []importer.Change{
				{
					Target:             "org/repo",
					Path:               "VERSION",
					Type:               fileset.ChangeUpdate,
					WriteMode:          importer.WritePatch,
					SuggestedWriteMode: importer.WritePatch,
					AvailableModes:     []importer.WriteMode{importer.WriteSource, importer.WritePatch},
					LocalTarget:        "/tmp/VERSION",
					ManifestPath:       "/tmp/gist.yaml",
					PatchYAMLPath:      "$.spec.files[0]",
					Current:            "0.1.0\n",
					WriteCurrent:       "0.1.0\n",
					PatchCurrent:       "v1.2.5\n",
					Desired:            "v1.2.6\n",
					CreateOnly:         true,
					HasExistingPatches: true,
				},
			},
		},
	}

	entries := diff.DiffEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "patch" {
		t.Fatalf("Action = %q, want patch", entries[0].Action)
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

func TestPrintImportPlan_CreateOnlyDefaultSkipReason(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	printImportPlan(p, &importer.Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []importer.Change{
			{
				Target:             "org/repo",
				Path:               "VERSION",
				Type:               fileset.ChangeUpdate,
				WriteMode:          importer.WriteSource,
				SuggestedWriteMode: importer.WriteSource,
				AvailableModes:     []importer.WriteMode{importer.WriteSource, importer.WritePatch},
				LocalTarget:        "templates/common/VERSION",
				Current:            "0.1.0\n",
				WriteCurrent:       "0.1.0\n",
				PatchCurrent:       "0.1.0\n",
				Desired:            "v1.2.6\n",
				CreateOnly:         true,
			},
		},
	})

	out := buf.String()
	if !strings.Contains(out, "skip: reconcile:create_only (Tab to change)") {
		t.Fatalf("expected create_only skip reason in output:\n%s", out)
	}
}

func TestPrintImportPlan_TemplateSkipReason(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	printImportPlan(p, &importer.Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []importer.Change{
			{
				Target:             "org/repo",
				Path:               ".gitignore",
				Type:               fileset.ChangeNoOp,
				WriteMode:          importer.WriteSkip,
				SuggestedWriteMode: importer.WriteSkip,
				Reason:             "cannot safely write back to template",
			},
		},
	})

	out := buf.String()
	if !strings.Contains(out, "skip: cannot safely write back to template") {
		t.Fatalf("expected template skip reason in output:\n%s", out)
	}
}

func TestPrintImportPlan_TemplateSkipReasonOverridesCreateOnly(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	printImportPlan(p, &importer.Result{
		ManifestEdits: make(map[string][]byte),
		FileChanges: []importer.Change{
			{
				Target:             "org/repo",
				Path:               "templates/go/Makefile",
				Type:               fileset.ChangeNoOp,
				WriteMode:          importer.WriteSkip,
				SuggestedWriteMode: importer.WriteSkip,
				CreateOnly:         true,
				Reason:             "cannot safely write back to template",
			},
		},
	})

	out := buf.String()
	if !strings.Contains(out, "skip: cannot safely write back to template") {
		t.Fatalf("expected template skip reason in output:\n%s", out)
	}
	if strings.Contains(out, "reconcile:create_only") {
		t.Fatalf("did not expect create_only to override template skip reason:\n%s", out)
	}
}
