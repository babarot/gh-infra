package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

func TestPlanTargetRepoNames_All(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo2"}},
	}
	names := PlanTargetRepoNames(repos, "")
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
	if names[0] != "org/repo1" {
		t.Errorf("names[0] = %q", names[0])
	}
	if names[1] != "org/repo2" {
		t.Errorf("names[1] = %q", names[1])
	}
}

func TestPlanTargetRepoNames_Filtered(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo2"}},
	}
	names := PlanTargetRepoNames(repos, "org/repo2")
	if len(names) != 1 {
		t.Fatalf("expected 1, got %d", len(names))
	}
	if names[0] != "org/repo2" {
		t.Errorf("names[0] = %q", names[0])
	}
}

func TestPlanTargetRepoNames_NoMatch(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
	}
	names := PlanTargetRepoNames(repos, "org/other")
	if len(names) != 0 {
		t.Errorf("expected 0, got %d", len(names))
	}
}

func TestPlanTargetRepoNames_Empty(t *testing.T) {
	names := PlanTargetRepoNames(nil, "")
	if len(names) != 0 {
		t.Errorf("expected 0, got %d", len(names))
	}
}

// publicRepoRunner returns a mock whose responses make FetchRepository report
// myorg/myrepo as an existing public repository with description "old".
func publicRepoRunner() *gh.MockRunner {
	return &gh.MockRunner{Responses: map[string][]byte{
		"repo view myorg/myrepo --json description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": []byte(`{
			"description": "old",
			"visibility": "PUBLIC",
			"repositoryTopics": [],
			"defaultBranchRef": {"name": "main"}
		}`),
		"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}": []byte(`{}`),
		"api repos/myorg/myrepo/immutable-releases":                                       []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/automated-security-fixes":                                 []byte(`{"enabled": false, "paused": false}`),
		"api repos/myorg/myrepo/private-vulnerability-reporting":                          []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/branches --jq [.[] | select(.protected == true) | .name]": []byte(`[]`),
	}}
}

func TestPlan_ConditionMet_ReturnsResolvedRepo(t *testing.T) {
	repo := &manifest.Repository{
		Metadata:        manifest.RepositoryMetadata{Owner: "myorg", Name: "myrepo"},
		Condition:       &manifest.RepositoryCondition{Visibility: manifest.VisibilityPublic},
		ConditionalSpec: &manifest.RepositorySpec{Description: manifest.Ptr("public repo")},
	}

	changes, targets, err := NewProcessor(publicRepoRunner(), nil).Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(changes) != 1 || changes[0].Field != "description" || changes[0].NewValue != "public repo" {
		t.Errorf("expected description change from conditional_spec, got %v", changes)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target repo, got %d", len(targets))
	}
	if got := targets[0]; got.ConditionalSpec != nil || got.Spec.Description == nil || *got.Spec.Description != "public repo" {
		t.Errorf("expected target repo with conditional_spec merged into spec, got %+v", got)
	}
}

func TestPlan_ConditionalForkPRApproval_DeclaredPrivate_Error(t *testing.T) {
	repo := &manifest.Repository{
		Metadata:  manifest.RepositoryMetadata{Owner: "myorg", Name: "myrepo"},
		Spec:      manifest.RepositorySpec{Visibility: manifest.Ptr(manifest.VisibilityPrivate)},
		Condition: &manifest.RepositoryCondition{Visibility: manifest.VisibilityPublic},
		ConditionalSpec: &manifest.RepositorySpec{
			Actions: &manifest.Actions{Enabled: manifest.Ptr(true), ForkPRApproval: manifest.Ptr("all_external_contributors")},
		},
	}

	_, _, err := NewProcessor(publicRepoRunner(), nil).Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "fork_pr_approval is not supported for private repositories") {
		t.Errorf("Plan() error = %v, want fork_pr_approval private error", err)
	}
}
