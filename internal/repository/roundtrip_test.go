package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/manifest"
)

// The round trip behind `import` then `plan`: a manifest exported from the
// current state (what import writes) must plan no changes against that
// state, directly and after writing it to YAML and parsing it back (what
// plan reads).
//
// Not covered on purpose:
//   - secrets: their values cannot be read, so export does not write them
//   - Actions disabled with nothing else set: export writes no actions block
//   - a ruleset whose include is empty: export writes no conditions, so an
//     exclude list alone is not exported (plan does not compare conditions
//     it is not given)

// roundTripRunner answers the name lookups of the resolver: team platform
// (11) and App ci (7) of owner o.
var roundTripRunner = substringRunner{rules: [][2]string{
	{"orgs/o/teams/platform", `{"id":11}`},
	{"orgs/o/teams", `[{"id":11,"slug":"platform"}]`},
	{"apps/ci", `{"id":7}`},
	{"commits/HEAD/check-runs", `[{"app":{"id":7,"slug":"ci"}}]`},
}}

// roundTripState is a public repository with every setting at a non-default value
// GitHub accepts together.
func roundTripState() *CurrentState {
	return &CurrentState{
		Owner: "o", Name: "r",
		Description: "a repository", Homepage: "https://example.com", Visibility: "public",
		Topics:   []string{"cli", "go"},
		Features: CurrentFeatures{Issues: true, Projects: false, Wiki: true, Discussions: true, PullRequests: true, PullRequestCreation: manifest.PullRequestCreationAll},
		MergeStrategy: CurrentMergeStrategy{
			AllowMergeCommit: true, AllowSquashMerge: true, AllowRebaseMerge: false, AllowAutoMerge: true, AutoDeleteHeadBranches: true,
			MergeCommitTitle: "MERGE_MESSAGE", MergeCommitMessage: "PR_TITLE",
			SquashMergeCommitTitle: "COMMIT_OR_PR_TITLE", SquashMergeCommitMessage: "COMMIT_MESSAGES",
		},
		ReleaseImmutability: true,
		Security:            CurrentSecurity{VulnerabilityAlerts: true, AutomatedSecurityFixes: true, PrivateVulnerabilityReporting: true},
		Actions:             CurrentActions{Enabled: true, AllowedActions: "all", SHAPinningRequired: true, WorkflowPermissions: "read", CanApprovePullRequests: false, ForkPRApproval: "all_external_contributors"},
	}
}

func TestRoundTrip_ExportThenPlan(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*CurrentState)
	}{
		{"base", func(*CurrentState) {}},
		{"empty description, homepage and topics", func(s *CurrentState) {
			s.Description, s.Homepage, s.Topics = "", "", nil
		}},
		{"private, Actions disabled", func(s *CurrentState) {
			s.Visibility = "private"
			s.Actions = CurrentActions{WorkflowPermissions: "read"}
		}},
		{"commit settings unreadable", func(s *CurrentState) {
			// GET /repos failed: plan keeps the defaults, which are empty.
			s.MergeStrategy.MergeCommitTitle, s.MergeStrategy.MergeCommitMessage = "", ""
			s.MergeStrategy.SquashMergeCommitTitle, s.MergeStrategy.SquashMergeCommitMessage = "", ""
		}},
		{"pull requests disabled for collaborators", func(s *CurrentState) {
			s.Features.PullRequests = false
			s.Features.PullRequestCreation = manifest.PullRequestCreationCollaboratorsOnly
		}},
		{"security off", func(s *CurrentState) { s.Security = CurrentSecurity{} }},
		{"Actions disabled with allowed actions left", func(s *CurrentState) {
			s.Visibility = "private"
			s.Actions = CurrentActions{Enabled: false, AllowedActions: "all", WorkflowPermissions: "read"}
		}},
		{"selected actions", func(s *CurrentState) {
			s.Actions = CurrentActions{Enabled: true, AllowedActions: "selected", WorkflowPermissions: "write", CanApprovePullRequests: true,
				SelectedActions: &CurrentSelectedActions{GithubOwnedAllowed: true, VerifiedAllowed: false, PatternsAllowed: []string{"babarot/*"}}}
		}},
		{"branch protection", func(s *CurrentState) {
			s.BranchProtection = map[string]*CurrentBranchProtection{
				"main":      {Pattern: "main", RequiredReviews: 2, DismissStaleReviews: true, RequireCodeOwnerReviews: true, EnforceAdmins: true, RestrictPushes: true, RequireStatusChecks: &CurrentStatusChecks{Strict: true, Contexts: []string{"ci"}}},
				"release/*": {Pattern: "release/*", RequiredReviews: 1, AllowForcePushes: true, AllowDeletions: true, RequireStatusChecks: &CurrentStatusChecks{Contexts: []string{}}},
				"dev":       {Pattern: "dev"},
			}
		}},
		{"rulesets", func(s *CurrentState) {
			s.Rulesets = map[string]*CurrentRuleset{
				"main": {ID: 1, Name: "main", Target: "branch", Enforcement: "active",
					BypassActors: []CurrentRulesetBypassActor{
						{ActorID: 11, ActorType: "Team", BypassMode: "pull_request"},
						{ActorID: 7, ActorType: "Integration", BypassMode: "always"},
						{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"},
						{ActorID: 1, ActorType: "OrganizationAdmin", BypassMode: "always"},
						{ActorID: 99, ActorType: "RepositoryRole", BypassMode: "pull_request"}, // a custom role, exported as id:99
					},
					Conditions: &CurrentRulesetConditions{RefName: &CurrentRulesetRefCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{"refs/heads/tmp/*"}}},
					Rules: CurrentRulesetRules{
						PullRequest:          &CurrentRulesetPullRequest{RequiredApprovingReviewCount: 1, DismissStaleReviewsOnPush: true, RequireLastPushApproval: true},
						RequiredStatusChecks: &CurrentRulesetStatusChecks{StrictRequiredStatusChecksPolicy: true, Contexts: []CurrentRulesetStatusCheck{{Context: "build", IntegrationID: 7}, {Context: "lint"}}},
						NonFastForward:       true, Deletion: true, RequiredLinearHistory: true,
						Update: &CurrentRulesetUpdate{AllowsFetchAndMerge: manifest.Ptr(true)},
					}},
				"tags": {ID: 2, Name: "tags", Target: "tag", Enforcement: "evaluate",
					Conditions: &CurrentRulesetConditions{RefName: &CurrentRulesetRefCondition{Include: []string{"refs/tags/v*"}, Exclude: []string{}}},
					Rules:      CurrentRulesetRules{Deletion: true, Creation: true, Update: &CurrentRulesetUpdate{}}},
				"empty checks": {ID: 3, Name: "empty checks", Target: "branch", Enforcement: "disabled",
					Conditions: &CurrentRulesetConditions{RefName: &CurrentRulesetRefCondition{Include: []string{"~ALL"}, Exclude: []string{}}},
					Rules:      CurrentRulesetRules{RequiredStatusChecks: &CurrentRulesetStatusChecks{Contexts: []CurrentRulesetStatusCheck{}}}},
				"no target branch": {ID: 4, Name: "no target branch", Target: "branch", Enforcement: "disabled",
					Conditions: &CurrentRulesetConditions{RefName: &CurrentRulesetRefCondition{Include: []string{}, Exclude: []string{}}},
					Rules:      CurrentRulesetRules{NonFastForward: true}},
			}
		}},
		{"labels", func(s *CurrentState) {
			s.Labels = map[string]*CurrentLabel{
				"bug":         {Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
				"Help Wanted": {Name: "Help Wanted", Color: "008672"},
				"123456":      {Name: "123456", Color: "123456"},
			}
		}},
		{"milestones", func(s *CurrentState) {
			s.Milestones = map[string]*CurrentMilestone{
				"v1": {Number: 1, Title: "v1", State: "open", DueOn: "2025-12-31", Description: "first"},
				"v0": {Number: 2, Title: "v0", State: "closed"},
			}
		}},
		{"variables", func(s *CurrentState) {
			s.Variables = map[string]string{"ENV": "prod", "EMPTY": ""}
		}},
		{"environments", func(s *CurrentState) {
			s.Environments = map[string]*CurrentEnvironment{
				"production": {Name: "production", CanAdminsBypass: false, WaitTimer: 10, HasReviewersRule: true, PreventSelfReview: true,
					Reviewers:    []CurrentEnvironmentReviewer{{Type: "User", ID: 2, Name: "alice"}, {Type: "Team", ID: 11, Name: "platform"}},
					BranchPolicy: manifest.DeploymentBranchesCustom,
					BranchRules:  []CurrentBranchRule{{ID: 1, Name: "main", Type: "branch"}, {ID: 2, Name: "v*", Type: "tag"}},
					Variables:    map[string]string{"REGION": "eu", "EMPTY": ""}},
				"frozen":       {Name: "frozen", CanAdminsBypass: true, BranchPolicy: manifest.DeploymentBranchesCustom},
				"qa":           {Name: "qa", CanAdminsBypass: true, BranchPolicy: manifest.DeploymentBranchesProtected},
				"github-pages": {Name: "github-pages", CanAdminsBypass: true, BranchPolicy: manifest.DeploymentBranchesAll},
				// A reviewers rule GitHub left with no reviewers.
				"emptied": {Name: "emptied", CanAdminsBypass: true, HasReviewersRule: true, BranchPolicy: manifest.DeploymentBranchesAll},
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current := roundTripState()
			tt.mutate(current)
			assertRoundTrip(t, current)
		})
	}
}

func assertRoundTrip(t *testing.T, current *CurrentState) {
	t.Helper()
	ctx := context.Background()
	resolver := manifest.NewResolver(roundTripRunner)
	opts := DiffOptions{Resolver: resolver.For(current.Owner)}

	m := ToManifest(ctx, current, resolver)
	if changes := Diff(ctx, m, current, opts); len(changes) > 0 {
		t.Errorf("exported manifest plans changes: %v", changes)
	}
	if err := ValidateDependencies(m, current); err != nil {
		t.Errorf("exported manifest fails the dependency checks: %v", err)
	}

	authoritative := *m
	authoritative.Reconcile = &manifest.RepositoryReconcile{
		Labels:           manifest.Ptr(manifest.CollectionReconcileAuthoritative),
		Rulesets:         manifest.Ptr(manifest.CollectionReconcileAuthoritative),
		BranchProtection: manifest.Ptr(manifest.CollectionReconcileAuthoritative),
	}
	if changes := Diff(ctx, &authoritative, current, opts); len(changes) > 0 {
		t.Errorf("with authoritative reconcile, plans changes: %v", changes)
	}

	// Through a file, as import writes it and plan reads it.
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "r.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.ParseAll(path)
	if err != nil {
		t.Fatalf("exported YAML does not parse: %v\n%s", err, data)
	}
	if changes := Diff(ctx, parsed.Repositories[0], current, opts); len(changes) > 0 {
		t.Errorf("exported YAML plans changes: %v\n%s", changes, data)
	}
}
