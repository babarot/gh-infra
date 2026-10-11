package repository

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// substringRunner answers a gh command with the first response whose key the
// command contains; anything else succeeds with no output.
type substringRunner struct{ rules [][2]string }

func (r substringRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	for _, rule := range r.rules {
		if strings.Contains(cmd, rule[0]) {
			return []byte(rule[1]), nil
		}
	}
	return nil, nil
}

func (r substringRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

// A repository whose every setting differs from the zero value and from the
// defaults, so a diff that read a group plan did not fetch would differ.
var richRepository = substringRunner{rules: [][2]string{
	{"repo view", `{"description":"d","homepageUrl":"https://h","visibility":"PUBLIC","isArchived":false,
		"repositoryTopics":[{"name":"go"}],"hasIssuesEnabled":true,"hasProjectsEnabled":true,"hasWikiEnabled":true,
		"hasDiscussionsEnabled":true,"mergeCommitAllowed":true,"squashMergeAllowed":true,"rebaseMergeAllowed":true,
		"deleteBranchOnMerge":true,"defaultBranchRef":{"name":"main"}}`},
	{"--jq {squash_merge_commit_title", `{"squash_merge_commit_title":"PR_TITLE","squash_merge_commit_message":"PR_BODY",
		"merge_commit_title":"PR_TITLE","merge_commit_message":"PR_BODY","allow_auto_merge":true,
		"has_pull_requests":true,"pull_request_creation_policy":"collaborators_only"}`},
	{"immutable-releases", `{"enabled":true}`},
	{"vulnerability-alerts", ``},
	{"automated-security-fixes", `{"enabled":true,"paused":false}`},
	{"private-vulnerability-reporting", `{"enabled":true}`},
	{"branches/main/protection", `{"required_pull_request_reviews":{"required_approving_review_count":2,"dismiss_stale_reviews":true}}`},
	{"branches?protected", "main\n"},
	{"rulesets/1", `{"id":1,"name":"main","target":"branch","enforcement":"active","source_type":"Repository",
		"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"deletion"}],"bypass_actors":[]}`},
	{"/rulesets", `[{"id":1,"name":"main","source_type":"Repository"}]`},
	{"secret list", "S1\n"},
	{"variable list", `[{"name":"V","value":"1"}]`},
	{"label list", `[{"name":"bug","color":"d73a4a","description":"x"}]`},
	{"/milestones", `[{"number":1,"title":"v1","state":"open","description":"m","due_on":null}]`},
	{"actions/permissions/workflow", `{"default_workflow_permissions":"write","can_approve_pull_request_reviews":true}`},
	{"actions/permissions/selected-actions", `{"github_owned_allowed":true,"verified_allowed":true,"patterns_allowed":["a/*"]}`},
	{"actions/permissions/fork-pr-contributor-approval", `{"approval_policy":"all_external_contributors"}`},
	{"actions/permissions", `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true}`},
}}

// Fetching only what a spec manages must give the same diff and the same
// dependency check result as fetching everything.
func TestFetchRepository_PrunedStateGivesSameDiff(t *testing.T) {
	no, yes := manifest.Ptr(false), manifest.Ptr(true)
	for _, tt := range []struct {
		name string
		spec manifest.RepositorySpec
	}{
		{"description", manifest.RepositorySpec{Description: manifest.Ptr("new")}},
		{"topics", manifest.RepositorySpec{Topics: []string{"cli"}, TopicsSet: true}},
		{"automated security fixes only", manifest.RepositorySpec{Security: &manifest.Security{AutomatedSecurityFixes: no}}},
		{"automated security fixes on", manifest.RepositorySpec{Security: &manifest.Security{AutomatedSecurityFixes: yes}}},
		{"vulnerability alerts", manifest.RepositorySpec{Security: &manifest.Security{VulnerabilityAlerts: no}}},
		{"private vulnerability reporting", manifest.RepositorySpec{Security: &manifest.Security{PrivateVulnerabilityReporting: no}}},
		{"merge commits only", manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{AllowMergeCommit: no}}},
		{"squash title", manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{SquashMergeCommitTitle: manifest.Ptr("COMMIT_OR_PR_TITLE"), SquashMergeCommitMessage: manifest.Ptr("COMMIT_MESSAGES")}}},
		{"auto merge", manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{AllowAutoMerge: no}}},
		{"wiki only", manifest.RepositorySpec{Features: &manifest.Features{Wiki: no}}},
		{"pull requests", manifest.RepositorySpec{Features: &manifest.Features{PullRequests: &manifest.PullRequests{Enabled: no}}}},
		{"release immutability", manifest.RepositorySpec{ReleaseImmutability: no}},
		{"label_sync only", manifest.RepositorySpec{LabelSync: manifest.Ptr("mirror")}},
		{"labels", manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "ffffff"}}}},
		{"secrets", manifest.RepositorySpec{Secrets: []manifest.Secret{{Name: "S2", Value: "v"}}}},
		{"variables", manifest.RepositorySpec{Variables: []manifest.Variable{{Name: "V", Value: "2"}}}},
		{"milestones", manifest.RepositorySpec{Milestones: []manifest.Milestone{{Title: "v1", State: manifest.Ptr("closed")}}}},
		{"actions", manifest.RepositorySpec{Actions: &manifest.Actions{Enabled: yes, AllowedActions: manifest.Ptr("all")}}},
		{"branch protection", manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{Pattern: "main", RequiredReviews: manifest.Ptr(1)}}}},
		{"branch protection []", manifest.RepositorySpec{BranchProtectionSet: true}},
		{"rulesets", manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{Name: "main", Enforcement: manifest.Ptr("evaluate")}}}},
		{"rulesets []", manifest.RepositorySpec{RulesetsSet: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			p := NewProcessor(richRepository, nil)
			full, err := p.FetchRepository(ctx, "o", "r", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			spec := tt.spec
			pruned, err := p.FetchRepository(ctx, "o", "r", &spec, nil)
			if err != nil {
				t.Fatal(err)
			}
			desired := &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"}, Spec: spec}
			desired.Reconcile = &manifest.RepositoryReconcile{
				Labels:           manifest.Ptr(manifest.CollectionReconcileAuthoritative),
				Rulesets:         manifest.Ptr(manifest.CollectionReconcileAuthoritative),
				BranchProtection: manifest.Ptr(manifest.CollectionReconcileAuthoritative),
			}

			fullDiff, prunedDiff := Diff(ctx, desired, full), Diff(ctx, desired, pruned)
			if !reflect.DeepEqual(fullDiff, prunedDiff) {
				t.Errorf("diff differs\nfull:   %+v\npruned: %+v", fullDiff, prunedDiff)
			}
			fullErr, prunedErr := ValidateDependencies(desired, full), ValidateDependencies(desired, pruned)
			if fmt.Sprint(fullErr) != fmt.Sprint(prunedErr) {
				t.Errorf("ValidateDependencies differs: full %v, pruned %v", fullErr, prunedErr)
			}
		})
	}
}
