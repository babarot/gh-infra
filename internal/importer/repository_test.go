package importer

import (
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestPlanRepository_NoDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: manifest.Ptr("my repo"),
		Visibility:  manifest.Ptr("public"),
		Topics:      []string{"go", "cli"},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: manifest.Ptr("my repo"),
			Visibility:  manifest.Ptr("public"),
			Topics:      []string{"cli", "go"}, // same set, different order
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: my repo
  visibility: public
  topics:
    - go
    - cli
`)

	rp, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}
	if rp.HasChanges() {
		t.Errorf("expected no changes, got %d diffs: %+v", len(rp.Diffs), rp.Diffs)
	}
}

func TestPlanRepository_ScalarDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: manifest.Ptr("old desc"),
		Visibility:  manifest.Ptr("private"),
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: manifest.Ptr("new desc"),
			Visibility:  manifest.Ptr("public"),
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: old desc
  visibility: private
`)

	rp, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}
	if !rp.HasChanges() {
		t.Fatal("expected changes")
	}

	found := make(map[string]bool)
	for _, d := range rp.Diffs {
		found[d.Field] = true
	}
	if !found["description"] {
		t.Error("expected diff for description")
	}
	if !found["visibility"] {
		t.Error("expected diff for visibility")
	}
}

func TestPlanRepository_TopicsDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Topics: []string{"go"},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Topics: []string{"go", "cli"},
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  topics:
    - go
`)

	rp, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	found := false
	for _, d := range rp.Diffs {
		if d.Field == "topics" {
			found = true
		}
	}
	if !found {
		t.Error("expected diff for topics")
	}
}

func TestPlanRepository_FeaturesDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: manifest.Ptr(true),
			Wiki:   manifest.Ptr(true),
		},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Features: &manifest.Features{
				Issues: manifest.Ptr(true),
				Wiki:   manifest.Ptr(false), // changed
			},
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  features:
    issues: true
    wiki: true
`)

	rp, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	found := false
	for _, d := range rp.Diffs {
		if d.Field == "features.wiki" {
			found = true
		}
	}
	if !found {
		t.Error("expected diff for features.wiki")
	}

	// features.issues should NOT have a diff (unchanged)
	for _, d := range rp.Diffs {
		if d.Field == "features.issues" {
			t.Error("features.issues should not have a diff (unchanged)")
		}
	}
}

func TestPlanRepository_ManifestBytesUpdated(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: manifest.Ptr("old"),
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: manifest.Ptr("new"),
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: old
`)

	mb := map[string][]byte{"/tmp/test.yaml": yamlData}
	_, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: mb,
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	// manifestBytes should be updated in-place
	updated := string(mb["/tmp/test.yaml"])
	if updated == string(yamlData) {
		t.Error("manifestBytes should have been updated")
	}
}

func TestDiffRepository_PatchesOnlyChangedFields(t *testing.T) {
	local := manifest.RepositorySpec{
		Topics: []string{"gist", "go", "cli", "gist-editor"},
		Features: &manifest.Features{
			Issues:      manifest.Ptr(true),
			Wiki:        manifest.Ptr(false),
			Projects:    manifest.Ptr(false),
			Discussions: manifest.Ptr(false),
		},
		MergeStrategy: &manifest.MergeStrategy{
			AllowSquashMerge:         manifest.Ptr(true),
			AllowMergeCommit:         manifest.Ptr(true),
			AllowRebaseMerge:         manifest.Ptr(false),
			AutoDeleteHeadBranches:   manifest.Ptr(true),
			SquashMergeCommitTitle:   manifest.Ptr("COMMIT_OR_PR_TITLE"),
			SquashMergeCommitMessage: manifest.Ptr("COMMIT_MESSAGES"),
			MergeCommitTitle:         manifest.Ptr("PR_TITLE"),
			MergeCommitMessage:       manifest.Ptr("PR_BODY"),
		},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Topics: []string{"go", "cli", "gist-editor", "gist"},
			Features: &manifest.Features{
				Issues:      manifest.Ptr(true),
				Wiki:        manifest.Ptr(false),
				Projects:    manifest.Ptr(false),
				Discussions: manifest.Ptr(false),
			},
			MergeStrategy: &manifest.MergeStrategy{
				AllowSquashMerge:         manifest.Ptr(true),
				AllowMergeCommit:         manifest.Ptr(true),
				AllowRebaseMerge:         manifest.Ptr(false),
				AutoDeleteHeadBranches:   manifest.Ptr(true),
				SquashMergeCommitTitle:   manifest.Ptr("COMMIT_OR_PR_TITLE"),
				SquashMergeCommitMessage: manifest.Ptr("COMMIT_MESSAGES"),
				MergeCommitTitle:         manifest.Ptr("MERGE_MESSAGE"),
				MergeCommitMessage:       manifest.Ptr("PR_TITLE"),
			},
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  topics: [gist, go, cli, gist-editor]
  features:
    issues: true
    wiki: false
    projects: false
    discussions: false
  merge_strategy:
    allow_squash_merge: true
    allow_merge_commit: true
    allow_rebase_merge: false
    auto_delete_head_branches: true
    merge_commit_title: PR_TITLE
    merge_commit_message: PR_BODY
    squash_merge_commit_title: COMMIT_OR_PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES
`)

	mb := map[string][]byte{"/tmp/test.yaml": yamlData}
	_, err := DiffRepository(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: mb,
	})
	if err != nil {
		t.Fatalf("DiffRepository error: %v", err)
	}

	updated := string(mb["/tmp/test.yaml"])
	if !strings.Contains(updated, "merge_commit_title: MERGE_MESSAGE") {
		t.Fatalf("expected merge_commit_title to be updated:\n%s", updated)
	}
	if !strings.Contains(updated, "merge_commit_message: PR_TITLE") {
		t.Fatalf("expected merge_commit_message to be updated:\n%s", updated)
	}
	if !strings.Contains(updated, "topics: [gist, go, cli, gist-editor]") {
		t.Fatalf("expected topics formatting/order to be preserved:\n%s", updated)
	}
	if !strings.Contains(updated, "wiki: false") {
		t.Fatalf("expected unchanged features.wiki to remain untouched:\n%s", updated)
	}
	if strings.Contains(updated, "- gist") {
		t.Fatalf("expected topics flow style to be preserved without rewrite:\n%s", updated)
	}
}

func TestMinimalOverride_AllSameAsDefaults(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Visibility: manifest.Ptr("private"),
		Features: &manifest.Features{
			Issues: manifest.Ptr(true),
		},
	}

	// imported matches defaults exactly
	imported := manifest.RepositorySpec{
		Visibility: manifest.Ptr("private"),
		Features: &manifest.Features{
			Issues: manifest.Ptr(true),
		},
	}

	override := minimalOverride(defaults, imported)

	if override.Visibility != nil {
		t.Errorf("Visibility should be nil (same as defaults), got %v", *override.Visibility)
	}
	if override.Features != nil {
		t.Errorf("Features should be nil (same as defaults), got %+v", override.Features)
	}
}

func TestMinimalOverride_ScalarOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Visibility: manifest.Ptr("private"),
	}

	imported := manifest.RepositorySpec{
		Visibility: manifest.Ptr("public"),
	}

	override := minimalOverride(defaults, imported)

	if override.Visibility == nil || *override.Visibility != "public" {
		t.Errorf("Visibility should be 'public', got %v", override.Visibility)
	}
}

func TestMinimalOverride_FeaturePartialOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: manifest.Ptr(true),
			Wiki:   manifest.Ptr(true),
		},
	}

	imported := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: manifest.Ptr(true),  // same
			Wiki:   manifest.Ptr(false), // different
		},
	}

	override := minimalOverride(defaults, imported)

	if override.Features == nil {
		t.Fatal("Features should not be nil")
	}
	if override.Features.Issues != nil {
		t.Error("Issues should be nil (same as defaults)")
	}
	if override.Features.Wiki == nil || *override.Features.Wiki != false {
		t.Error("Wiki should be false (different from defaults)")
	}
}

func TestMinimalOverride_TopicsOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Topics: []string{"go"},
	}

	imported := manifest.RepositorySpec{
		Topics: []string{"go", "cli"},
	}

	override := minimalOverride(defaults, imported)

	if len(override.Topics) != 2 {
		t.Errorf("Topics should have 2 items (different from defaults), got %d", len(override.Topics))
	}
}

func TestCompareSpecs_NoDiff(t *testing.T) {
	spec := manifest.RepositorySpec{
		Description: manifest.Ptr("test"),
		Visibility:  manifest.Ptr("public"),
	}

	diffs := compareSpecs(spec, spec)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %+v", len(diffs), diffs)
	}
}

// --- DiffRepositorySet tests ---

func TestDiffRepositorySet_NoDiff(t *testing.T) {
	// When the imported spec matches defaults+override exactly, no diffs.
	defaults := &manifest.RepositorySetDefaults{
		Spec: manifest.RepositorySpec{
			Visibility: manifest.Ptr("private"),
			Features:   &manifest.Features{Issues: manifest.Ptr(true)},
		},
	}
	originalEntry := &manifest.RepositorySpec{} // no override

	doc := &manifest.RepositoryDocument{
		Resource: &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Name: "repo", Owner: "org"},
			Spec: manifest.RepositorySpec{
				Visibility: manifest.Ptr("private"),
				Features:   &manifest.Features{Issues: manifest.Ptr(true)},
			},
		},
		SourcePath:        "/tmp/set.yaml",
		DocIndex:          0,
		FromSet:           true,
		SetEntryIndex:     0,
		DefaultsSpec:      defaults,
		OriginalEntrySpec: originalEntry,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: org
defaults:
  spec:
    visibility: private
    features:
      issues: true
repositories:
  - name: repo
    spec: {}
`)

	imported := &manifest.Repository{
		Spec: manifest.RepositorySpec{
			Visibility: manifest.Ptr("private"),
			Features:   &manifest.Features{Issues: manifest.Ptr(true)},
		},
	}

	rp, err := DiffRepositorySet(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      imported,
		ManifestBytes: map[string][]byte{"/tmp/set.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("DiffRepositorySet error: %v", err)
	}
	if rp.HasChanges() {
		t.Errorf("expected no changes, got %d diffs: %+v", len(rp.Diffs), rp.Diffs)
	}
}

func TestDiffRepositorySet_OverrideChange(t *testing.T) {
	// When the imported spec differs from defaults, the override should contain only the diff.
	defaults := &manifest.RepositorySetDefaults{
		Spec: manifest.RepositorySpec{
			Visibility: manifest.Ptr("private"),
		},
	}
	originalEntry := &manifest.RepositorySpec{}

	doc := &manifest.RepositoryDocument{
		Resource: &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Name: "repo", Owner: "org"},
			Spec: manifest.RepositorySpec{
				Visibility: manifest.Ptr("private"),
			},
		},
		SourcePath:        "/tmp/set.yaml",
		DocIndex:          0,
		FromSet:           true,
		SetEntryIndex:     0,
		DefaultsSpec:      defaults,
		OriginalEntrySpec: originalEntry,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: org
defaults:
  spec:
    visibility: private
repositories:
  - name: repo
    spec:
      visibility: private
`)

	imported := &manifest.Repository{
		Spec: manifest.RepositorySpec{
			Visibility:  manifest.Ptr("public"), // differs from defaults
			Description: manifest.Ptr("hello"),  // not in defaults
		},
	}

	rp, err := DiffRepositorySet(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      imported,
		ManifestBytes: map[string][]byte{"/tmp/set.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("DiffRepositorySet error: %v", err)
	}
	if !rp.HasChanges() {
		t.Fatal("expected changes")
	}

	found := make(map[string]bool)
	for _, d := range rp.Diffs {
		found[d.Field] = true
	}
	if !found["visibility"] {
		t.Error("expected diff for visibility")
	}
	if !found["description"] {
		t.Error("expected diff for description")
	}
}

func TestDiffRepositorySet_DefaultsNil(t *testing.T) {
	// When DefaultsSpec is nil, the doc should be skipped.
	doc := &manifest.RepositoryDocument{
		Resource: &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Name: "repo", Owner: "org"},
			Spec:     manifest.RepositorySpec{},
		},
		SourcePath:   "/tmp/set.yaml",
		DocIndex:     0,
		FromSet:      true,
		DefaultsSpec: nil, // nil defaults
	}

	imported := &manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: manifest.Ptr("new"),
		},
	}

	rp, err := DiffRepositorySet(DiffInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      imported,
		ManifestBytes: map[string][]byte{"/tmp/set.yaml": []byte("ignored")},
	})
	if err != nil {
		t.Fatalf("DiffRepositorySet error: %v", err)
	}
	if rp.HasChanges() {
		t.Errorf("expected no changes when DefaultsSpec is nil, got %d diffs", len(rp.Diffs))
	}
}

// --- compareBranchProtection tests ---

func TestCompareBranchProtection_Update(t *testing.T) {
	local := []manifest.BranchProtection{
		{Pattern: "main", RequiredReviews: manifest.Ptr(1)},
	}
	imported := []manifest.BranchProtection{
		{Pattern: "main", RequiredReviews: manifest.Ptr(2)},
	}

	diffs := compareBranchProtection(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	if diffs[0].Field != "branch_protection.main" {
		t.Errorf("Field = %q, want %q", diffs[0].Field, "branch_protection.main")
	}
}

func TestCompareBranchProtection_NewOnGitHub(t *testing.T) {
	local := []manifest.BranchProtection{}
	imported := []manifest.BranchProtection{
		{Pattern: "develop", RequiredReviews: manifest.Ptr(1)},
	}

	diffs := compareBranchProtection(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Old != nil {
		t.Error("Old should be nil for new branch protection")
	}
}

func TestCompareBranchProtection_DeletedOnGitHub(t *testing.T) {
	local := []manifest.BranchProtection{
		{Pattern: "release", RequiredReviews: manifest.Ptr(1)},
	}
	imported := []manifest.BranchProtection{}

	diffs := compareBranchProtection(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].New != nil {
		t.Error("New should be nil for deleted branch protection")
	}
}

func TestCompareBranchProtection_Empty(t *testing.T) {
	diffs := compareBranchProtection(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d", len(diffs))
	}
}

// --- compareRulesets tests ---

func TestCompareRulesets_Update(t *testing.T) {
	local := []manifest.Ruleset{
		{Name: "protect-main", Enforcement: manifest.Ptr("active")},
	}
	imported := []manifest.Ruleset{
		{Name: "protect-main", Enforcement: manifest.Ptr("evaluate")},
	}

	diffs := compareRulesets(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	if diffs[0].Field != "rulesets.protect-main" {
		t.Errorf("Field = %q, want %q", diffs[0].Field, "rulesets.protect-main")
	}
}

func TestCompareRulesets_NewOnGitHub(t *testing.T) {
	local := []manifest.Ruleset{}
	imported := []manifest.Ruleset{
		{Name: "new-rule", Enforcement: manifest.Ptr("active")},
	}

	diffs := compareRulesets(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Old != nil {
		t.Error("Old should be nil for new ruleset")
	}
}

func TestCompareRulesets_Empty(t *testing.T) {
	diffs := compareRulesets(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d", len(diffs))
	}
}

// --- compareVariables tests ---

func TestCompareVariables_Diff(t *testing.T) {
	local := []manifest.Variable{
		{Name: "ENV", Value: "dev"},
	}
	imported := []manifest.Variable{
		{Name: "ENV", Value: "prod"},
	}

	diffs := compareVariables(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	if diffs[0].Field != "variables.ENV" {
		t.Errorf("Field = %q, want %q", diffs[0].Field, "variables.ENV")
	}
}

func TestCompareVariables_NewOnGitHub(t *testing.T) {
	local := []manifest.Variable{}
	imported := []manifest.Variable{
		{Name: "NEW_VAR", Value: "val"},
	}

	diffs := compareVariables(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Field != "variables.NEW_VAR" {
		t.Errorf("Field = %q, want %q", diffs[0].Field, "variables.NEW_VAR")
	}
}

func TestCompareVariables_DeletedOnGitHub(t *testing.T) {
	local := []manifest.Variable{
		{Name: "OLD_VAR", Value: "val"},
	}
	imported := []manifest.Variable{}

	diffs := compareVariables(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].New != nil {
		t.Error("New should be nil for deleted variable")
	}
}

func TestCompareVariables_NoDiff(t *testing.T) {
	vars := []manifest.Variable{
		{Name: "A", Value: "1"},
		{Name: "B", Value: "2"},
	}

	diffs := compareVariables(vars, vars)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %+v", len(diffs), diffs)
	}
}

// --- compareMergeStrategy tests ---

func TestCompareMergeStrategy_Diff(t *testing.T) {
	local := &manifest.MergeStrategy{
		AllowMergeCommit: manifest.Ptr(true),
		AllowSquashMerge: manifest.Ptr(false),
	}
	imported := &manifest.MergeStrategy{
		AllowMergeCommit: manifest.Ptr(false),
		AllowSquashMerge: manifest.Ptr(true),
	}

	diffs := compareMergeStrategy(local, imported)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d: %+v", len(diffs), diffs)
	}

	found := make(map[string]bool)
	for _, d := range diffs {
		found[d.Field] = true
	}
	if !found["merge_strategy.allow_merge_commit"] {
		t.Error("expected diff for allow_merge_commit")
	}
	if !found["merge_strategy.allow_squash_merge"] {
		t.Error("expected diff for allow_squash_merge")
	}
}

func TestCompareMergeStrategy_BothNil(t *testing.T) {
	diffs := compareMergeStrategy(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d", len(diffs))
	}
}

// --- compareActions tests ---

func TestCompareActions_Diff(t *testing.T) {
	local := &manifest.Actions{
		Enabled: manifest.Ptr(true),
	}
	imported := &manifest.Actions{
		Enabled: manifest.Ptr(false),
	}

	diffs := compareActions(local, imported)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	if diffs[0].Field != "actions.enabled" {
		t.Errorf("Field = %q, want %q", diffs[0].Field, "actions.enabled")
	}
}

func TestCompareActions_BothNil(t *testing.T) {
	diffs := compareActions(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d", len(diffs))
	}
}

func TestCompareActions_SelectedActions(t *testing.T) {
	local := &manifest.Actions{
		SelectedActions: &manifest.SelectedActions{
			GithubOwnedAllowed: manifest.Ptr(true),
			PatternsAllowed:    []string{"actions/checkout@*"},
		},
	}
	imported := &manifest.Actions{
		SelectedActions: &manifest.SelectedActions{
			GithubOwnedAllowed: manifest.Ptr(false),
			PatternsAllowed:    []string{"actions/checkout@*", "actions/setup-go@*"},
		},
	}

	diffs := compareActions(local, imported)

	found := make(map[string]bool)
	for _, d := range diffs {
		found[d.Field] = true
	}
	if !found["actions.selected_actions.github_owned_allowed"] {
		t.Error("expected diff for github_owned_allowed")
	}
	if !found["actions.selected_actions.patterns_allowed"] {
		t.Error("expected diff for patterns_allowed")
	}
}
