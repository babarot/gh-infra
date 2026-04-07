package repository

import (
	"context"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestToManifest(t *testing.T) {
	state := &CurrentState{
		Owner:       "myorg",
		Name:        "myrepo",
		Description: "A test repository",
		Homepage:    "https://example.com",
		Visibility:  "public",
		Topics:      []string{"go", "cli"},
		Features: CurrentFeatures{
			Issues:      true,
			Projects:    false,
			Wiki:        true,
			Discussions: false,
		},
		MergeStrategy: CurrentMergeStrategy{
			AllowMergeCommit:         true,
			AllowSquashMerge:         true,
			AllowRebaseMerge:         false,
			AutoDeleteHeadBranches:   true,
			MergeCommitTitle:         "MERGE_MESSAGE",
			MergeCommitMessage:       "PR_BODY",
			SquashMergeCommitTitle:   "PR_TITLE",
			SquashMergeCommitMessage: "COMMIT_MESSAGES",
		},
		BranchProtection: map[string]*CurrentBranchProtection{
			"main": {
				Pattern:                 "main",
				RequiredReviews:         2,
				DismissStaleReviews:     true,
				RequireCodeOwnerReviews: true,
				EnforceAdmins:           true,
				AllowForcePushes:        false,
				AllowDeletions:          false,
				RequireStatusChecks: &CurrentStatusChecks{
					Strict:   true,
					Contexts: []string{"ci/test"},
				},
			},
		},
		Secrets: []string{"SECRET1"}, // secrets are name-only, not exported to manifest
		Variables: map[string]string{
			"ENV": "production",
		},
	}

	repo := ToManifest(context.Background(), state, nil)

	// APIVersion and Kind
	if repo.APIVersion != manifest.APIVersion {
		t.Errorf("APIVersion = %q, want %q", repo.APIVersion, manifest.APIVersion)
	}
	if repo.Kind != manifest.KindRepository {
		t.Errorf("Kind = %q, want %q", repo.Kind, manifest.KindRepository)
	}

	// Metadata
	if repo.Metadata.Name != "myrepo" {
		t.Errorf("Metadata.Name = %q, want myrepo", repo.Metadata.Name)
	}
	if repo.Metadata.Owner != "myorg" {
		t.Errorf("Metadata.Owner = %q, want myorg", repo.Metadata.Owner)
	}

	// Spec basic fields
	if repo.Spec.Description == nil || *repo.Spec.Description != "A test repository" {
		t.Errorf("Description = %v, want 'A test repository'", repo.Spec.Description)
	}
	if repo.Spec.Homepage == nil || *repo.Spec.Homepage != "https://example.com" {
		t.Errorf("Homepage = %v, want 'https://example.com'", repo.Spec.Homepage)
	}
	if repo.Spec.Visibility == nil || *repo.Spec.Visibility != "public" {
		t.Errorf("Visibility = %v, want 'public'", repo.Spec.Visibility)
	}
	if len(repo.Spec.Topics) != 2 || repo.Spec.Topics[0] != "go" || repo.Spec.Topics[1] != "cli" {
		t.Errorf("Topics = %v, want [go cli]", repo.Spec.Topics)
	}

	// Features
	f := repo.Spec.Features
	if f == nil {
		t.Fatal("expected non-nil Features")
		return
	}
	assertBoolPtr(t, "Issues", f.Issues, true)
	assertBoolPtr(t, "Projects", f.Projects, false)
	assertBoolPtr(t, "Wiki", f.Wiki, true)
	assertBoolPtr(t, "Discussions", f.Discussions, false)

	// MergeStrategy
	ms := repo.Spec.MergeStrategy
	if ms == nil {
		t.Fatal("expected non-nil MergeStrategy")
		return
	}
	assertBoolPtr(t, "AllowMergeCommit", ms.AllowMergeCommit, true)
	assertBoolPtr(t, "AllowSquashMerge", ms.AllowSquashMerge, true)
	assertBoolPtr(t, "AllowRebaseMerge", ms.AllowRebaseMerge, false)
	assertBoolPtr(t, "AutoDeleteHeadBranches", ms.AutoDeleteHeadBranches, true)
	assertStringPtr(t, "MergeCommitTitle", ms.MergeCommitTitle, "MERGE_MESSAGE")
	assertStringPtr(t, "MergeCommitMessage", ms.MergeCommitMessage, "PR_BODY")
	assertStringPtr(t, "SquashMergeCommitTitle", ms.SquashMergeCommitTitle, "PR_TITLE")
	assertStringPtr(t, "SquashMergeCommitMessage", ms.SquashMergeCommitMessage, "COMMIT_MESSAGES")

	// Branch protection
	if len(repo.Spec.BranchProtection) != 1 {
		t.Fatalf("BranchProtection count = %d, want 1", len(repo.Spec.BranchProtection))
	}
	bp := repo.Spec.BranchProtection[0]
	if bp.Pattern != "main" {
		t.Errorf("BP Pattern = %q, want main", bp.Pattern)
	}
	if bp.RequiredReviews == nil || *bp.RequiredReviews != 2 {
		t.Errorf("BP RequiredReviews = %v, want 2", bp.RequiredReviews)
	}
	assertBoolPtr(t, "BP DismissStaleReviews", bp.DismissStaleReviews, true)
	assertBoolPtr(t, "BP RequireCodeOwnerReviews", bp.RequireCodeOwnerReviews, true)
	assertBoolPtr(t, "BP EnforceAdmins", bp.EnforceAdmins, true)
	assertBoolPtr(t, "BP AllowForcePushes", bp.AllowForcePushes, false)
	assertBoolPtr(t, "BP AllowDeletions", bp.AllowDeletions, false)
	if bp.RequireStatusChecks == nil {
		t.Fatal("expected non-nil RequireStatusChecks")
	}
	if !bp.RequireStatusChecks.Strict {
		t.Error("expected RequireStatusChecks.Strict = true")
	}
	if len(bp.RequireStatusChecks.Contexts) != 1 || bp.RequireStatusChecks.Contexts[0] != "ci/test" {
		t.Errorf("RequireStatusChecks.Contexts = %v, want [ci/test]", bp.RequireStatusChecks.Contexts)
	}

	// Variables
	if len(repo.Spec.Variables) != 1 {
		t.Fatalf("Variables count = %d, want 1", len(repo.Spec.Variables))
	}
	if repo.Spec.Variables[0].Name != "ENV" || repo.Spec.Variables[0].Value != "production" {
		t.Errorf("Variable = %+v, want {ENV production}", repo.Spec.Variables[0])
	}
}

func TestToManifest_EmptyHomepage(t *testing.T) {
	state := &CurrentState{
		Owner:      "myorg",
		Name:       "myrepo",
		Visibility: "private",
		Features:   CurrentFeatures{},
	}

	repo := ToManifest(context.Background(), state, nil)

	if repo.Spec.Homepage != nil {
		t.Errorf("expected nil Homepage for empty string, got %v", repo.Spec.Homepage)
	}
}

func TestToManifest_NoBranchProtection(t *testing.T) {
	state := &CurrentState{
		Owner:            "myorg",
		Name:             "myrepo",
		BranchProtection: map[string]*CurrentBranchProtection{},
		Variables:        map[string]string{},
	}

	repo := ToManifest(context.Background(), state, nil)

	if len(repo.Spec.BranchProtection) != 0 {
		t.Errorf("expected 0 branch protections, got %d", len(repo.Spec.BranchProtection))
	}
	if len(repo.Spec.Variables) != 0 {
		t.Errorf("expected 0 variables, got %d", len(repo.Spec.Variables))
	}
}

func TestToManifest_Actions(t *testing.T) {
	state := &CurrentState{
		Owner: "myorg",
		Name:  "myrepo",
		Actions: CurrentActions{
			Enabled:            true,
			AllowedActions:     "all",
			SHAPinningRequired: true,
		},
	}

	repo := ToManifest(context.Background(), state, nil)

	if repo.Spec.Actions == nil {
		t.Fatal("expected actions to be exported")
	}
	assertBoolPtr(t, "Actions.Enabled", repo.Spec.Actions.Enabled, true)
	assertStringPtr(t, "Actions.AllowedActions", repo.Spec.Actions.AllowedActions, "all")
	assertBoolPtr(t, "Actions.SHAPinningRequired", repo.Spec.Actions.SHAPinningRequired, true)
}

func TestToManifest_NilStatusChecks(t *testing.T) {
	state := &CurrentState{
		Owner: "myorg",
		Name:  "myrepo",
		BranchProtection: map[string]*CurrentBranchProtection{
			"main": {
				Pattern:             "main",
				RequireStatusChecks: nil,
			},
		},
	}

	repo := ToManifest(context.Background(), state, nil)

	if len(repo.Spec.BranchProtection) != 1 {
		t.Fatalf("expected 1 branch protection, got %d", len(repo.Spec.BranchProtection))
	}
	if repo.Spec.BranchProtection[0].RequireStatusChecks != nil {
		t.Error("expected nil RequireStatusChecks in manifest")
	}
}

func TestToManifest_Rulesets(t *testing.T) {
	state := &CurrentState{
		Owner: "myorg",
		Name:  "myrepo",
		Rulesets: map[string]*CurrentRuleset{
			"protect-main": {
				ID:          42,
				Name:        "protect-main",
				Target:      "branch",
				Enforcement: "active",
				BypassActors: []CurrentRulesetBypassActor{
					{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"},
				},
				Conditions: &CurrentRulesetConditions{
					RefName: &CurrentRulesetRefCondition{
						Include: []string{"refs/heads/main"},
						Exclude: []string{},
					},
				},
				Rules: CurrentRulesetRules{
					NonFastForward: true,
					Deletion:       false,
					PullRequest: &CurrentRulesetPullRequest{
						RequiredApprovingReviewCount: 1,
						DismissStaleReviewsOnPush:    true,
					},
					RequiredStatusChecks: &CurrentRulesetStatusChecks{
						StrictRequiredStatusChecksPolicy: true,
						Contexts: []CurrentRulesetStatusCheck{
							{Context: "ci/test", IntegrationID: 123},
						},
					},
				},
			},
		},
	}

	repo := ToManifest(context.Background(), state, nil)

	if len(repo.Spec.Rulesets) != 1 {
		t.Fatalf("expected 1 ruleset, got %d", len(repo.Spec.Rulesets))
	}

	rs := repo.Spec.Rulesets[0]
	if rs.Name != "protect-main" {
		t.Errorf("name: got %q, want %q", rs.Name, "protect-main")
	}
	assertStringPtr(t, "target", rs.Target, "branch")
	assertStringPtr(t, "enforcement", rs.Enforcement, "active")
	assertBoolPtr(t, "non_fast_forward", rs.Rules.NonFastForward, true)
	assertBoolPtr(t, "deletion", rs.Rules.Deletion, false)

	if rs.Rules.PullRequest == nil {
		t.Fatal("expected pull_request rule")
	}
	if *rs.Rules.PullRequest.RequiredApprovingReviewCount != 1 {
		t.Errorf("review count: got %d, want 1", *rs.Rules.PullRequest.RequiredApprovingReviewCount)
	}
	assertBoolPtr(t, "dismiss_stale_reviews", rs.Rules.PullRequest.DismissStaleReviewsOnPush, true)

	if rs.Rules.RequiredStatusChecks == nil {
		t.Fatal("expected required_status_checks rule")
	}
	assertBoolPtr(t, "strict", rs.Rules.RequiredStatusChecks.StrictRequiredStatusChecksPolicy, true)
	if len(rs.Rules.RequiredStatusChecks.Contexts) != 1 {
		t.Fatalf("expected 1 status check context, got %d", len(rs.Rules.RequiredStatusChecks.Contexts))
	}
	if rs.Rules.RequiredStatusChecks.Contexts[0].Context != "ci/test" {
		t.Errorf("context: got %q, want %q", rs.Rules.RequiredStatusChecks.Contexts[0].Context, "ci/test")
	}
	// With nil resolver, status check app is not resolved (context only)
	if rs.Rules.RequiredStatusChecks.Contexts[0].App != "" {
		t.Errorf("expected empty app with nil resolver, got %q", rs.Rules.RequiredStatusChecks.Contexts[0].App)
	}

	if len(rs.BypassActors) != 1 {
		t.Fatalf("expected 1 bypass actor, got %d", len(rs.BypassActors))
	}
	// With nil resolver, RepositoryRole 5 → "admin" via RoleNameFromID fallback
	if rs.BypassActors[0].Role != "admin" {
		t.Errorf("bypass actor role: got %q, want %q", rs.BypassActors[0].Role, "admin")
	}

	if rs.Conditions == nil || rs.Conditions.RefName == nil {
		t.Fatal("expected conditions with ref_name")
	}
	if len(rs.Conditions.RefName.Include) != 1 || rs.Conditions.RefName.Include[0] != "refs/heads/main" {
		t.Errorf("conditions include: got %v", rs.Conditions.RefName.Include)
	}
}

// helpers

func assertBoolPtr(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %v, got nil", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %v, want %v", name, *got, want)
	}
}

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: expected %q, got nil", name, want)
		return
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", name, *got, want)
	}
}

func TestToManifest_Labels(t *testing.T) {
	state := &CurrentState{
		Owner: "org",
		Name:  "repo",
		Labels: map[string]*CurrentLabel{
			"bug":          {Name: "bug", Color: "d73a4a", Description: "A bug"},
			"kind/feature": {Name: "kind/feature", Color: "425df5", Description: ""},
		},
	}

	repo := ToManifest(context.Background(), state, nil)

	if len(repo.Spec.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(repo.Spec.Labels))
	}

	labelMap := make(map[string]manifest.Label)
	for _, l := range repo.Spec.Labels {
		labelMap[l.Name] = l
	}

	bug, ok := labelMap["bug"]
	if !ok {
		t.Fatal("missing label 'bug'")
	}
	if bug.Color != "d73a4a" {
		t.Errorf("bug.Color = %q, want %q", bug.Color, "d73a4a")
	}
	if bug.Description != "A bug" {
		t.Errorf("bug.Description = %q, want %q", bug.Description, "A bug")
	}

	feat, ok := labelMap["kind/feature"]
	if !ok {
		t.Fatal("missing label 'kind/feature'")
	}
	if feat.Color != "425df5" {
		t.Errorf("kind/feature.Color = %q, want %q", feat.Color, "425df5")
	}
}
