package repository

import (
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

func newTestRepo(owner, name string) *manifest.Repository {
	return &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{
			Owner: owner,
			Name:  name,
		},
	}
}

func TestApplyRepoDescription(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			OldValue: "old desc",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--description", "new desc"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyHomepage(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "homepage",
			NewValue: "https://example.com",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--homepage", "https://example.com"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyVisibility(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "visibility",
			NewValue: "private",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--visibility", "private"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyTopics(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"repo view myorg/myrepo --json repositoryTopics --jq .[].name": []byte("old-topic\nkeep-topic\n"),
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Topics = []string{"keep-topic", "new-topic"}

	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "topics",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	// Expect: view call, remove old-topic, add new-topic
	// (keep-topic should not be touched)
	var removeCalls, addCalls []string
	for _, call := range mock.Called {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "--remove-topic") {
			removeCalls = append(removeCalls, joined)
		}
		if strings.Contains(joined, "--add-topic") {
			addCalls = append(addCalls, joined)
		}
	}

	if len(removeCalls) != 1 {
		t.Fatalf("expected 1 remove-topic call, got %d: %v", len(removeCalls), removeCalls)
	}
	if !strings.Contains(removeCalls[0], "old-topic") {
		t.Errorf("expected remove old-topic, got %s", removeCalls[0])
	}

	if len(addCalls) != 1 {
		t.Fatalf("expected 1 add-topic call, got %d: %v", len(addCalls), addCalls)
	}
	if !strings.Contains(addCalls[0], "new-topic") {
		t.Errorf("expected add new-topic, got %s", addCalls[0])
	}
}

func TestApplyFeatureToggle(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "wiki",
			NewValue: false,
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--enable-wiki=false"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyWithErrNotFound(t *testing.T) {
	notFoundErr := fmt.Errorf("%w: %w", gh.ErrNotFound, &gh.ExitError{
		Cmd: "repo edit myorg/myrepo", ExitCode: 1,
		APIError: &gh.APIError{Status: 404, Message: "Not Found"},
	})

	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo edit myorg/myrepo --description new desc": notFoundErr,
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("expected user-friendly not found message, got %q", errMsg)
	}
}

func TestApplyWithErrForbidden(t *testing.T) {
	forbiddenErr := fmt.Errorf("%w: %w", gh.ErrForbidden, &gh.ExitError{
		Cmd: "repo edit myorg/myrepo", ExitCode: 1,
		APIError: &gh.APIError{Status: 403, Message: "Forbidden"},
	})

	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo edit myorg/myrepo --description new desc": forbiddenErr,
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "no permission") {
		t.Errorf("expected user-friendly forbidden message, got %q", errMsg)
	}
}

func TestApplyVariableSet(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Variables = []manifest.Variable{
		{Name: "MY_VAR", Value: "my-value"},
	}

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: "Variable",
			Name:     "myorg/myrepo",
			Field:    "MY_VAR",
			NewValue: "my-value",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"variable", "set", "MY_VAR", "--repo", "myorg/myrepo", "--body", "my-value"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplySecretSet(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Secrets = []manifest.Secret{
		{Name: "MY_SECRET", Value: "secret-value"},
	}

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: "Secret",
			Name:     "myorg/myrepo",
			Field:    "MY_SECRET",
			NewValue: "secret-value",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"secret", "set", "MY_SECRET", "--repo", "myorg/myrepo", "--body", "secret-value"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplySkipsNoOp(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeNoOp,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for noop, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Fatalf("expected 0 calls for noop, got %d", len(mock.Called))
	}
}

func TestApplyBranchProtection(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	reviews := 2
	enforceAdmins := true
	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.BranchProtection = []manifest.BranchProtection{
		{
			Pattern:         "main",
			RequiredReviews: &reviews,
			EnforceAdmins:   &enforceAdmins,
			RequireStatusChecks: &manifest.StatusChecks{
				Strict:   true,
				Contexts: []string{"ci/test"},
			},
		},
	}

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: "BranchProtection[main]",
			Name:     "myorg/myrepo",
			Field:    "branch_protection",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// The applyBranchProtection method ultimately calls applyBranchProtectionViaAPI
	// which uses "api repos/{owner}/{repo}/branches/{pattern}/protection --method PUT ..."
	// Check that at least one call was made with the correct endpoint
	found := false
	for _, call := range mock.Called {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "repos/myorg/myrepo/branches/main/protection") &&
			strings.Contains(joined, "PUT") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected API call to branch protection endpoint with PUT, got calls: %v", mock.Called)
	}
}

func TestBuildBranchProtectionPayload(t *testing.T) {
	reviews := 2
	dismissStale := true
	codeOwners := true
	enforceAdmins := true
	allowForce := false
	allowDel := false

	bp := &manifest.BranchProtection{
		Pattern:                 "main",
		RequiredReviews:         &reviews,
		DismissStaleReviews:     &dismissStale,
		RequireCodeOwnerReviews: &codeOwners,
		EnforceAdmins:           &enforceAdmins,
		AllowForcePushes:        &allowForce,
		AllowDeletions:          &allowDel,
		RequireStatusChecks: &manifest.StatusChecks{
			Strict:   true,
			Contexts: []string{"ci/test", "ci/lint"},
		},
	}

	payload := buildBranchProtectionPayload(bp)

	// Check enforce_admins
	if payload["enforce_admins"] != true {
		t.Errorf("enforce_admins = %v, want true", payload["enforce_admins"])
	}
	if payload["allow_force_pushes"] != false {
		t.Errorf("allow_force_pushes = %v, want false", payload["allow_force_pushes"])
	}
	if payload["allow_deletions"] != false {
		t.Errorf("allow_deletions = %v, want false", payload["allow_deletions"])
	}

	// Check reviews
	reviewsPayload, ok := payload["required_pull_request_reviews"].(map[string]any)
	if !ok {
		t.Fatalf("required_pull_request_reviews is not a map, got %T", payload["required_pull_request_reviews"])
	}
	if reviewsPayload["required_approving_review_count"] != 2 {
		t.Errorf("required_approving_review_count = %v, want 2", reviewsPayload["required_approving_review_count"])
	}
	if reviewsPayload["dismiss_stale_reviews"] != true {
		t.Errorf("dismiss_stale_reviews = %v, want true", reviewsPayload["dismiss_stale_reviews"])
	}
	if reviewsPayload["require_code_owner_reviews"] != true {
		t.Errorf("require_code_owner_reviews = %v, want true", reviewsPayload["require_code_owner_reviews"])
	}

	// Check status checks
	scPayload, ok := payload["required_status_checks"].(map[string]any)
	if !ok {
		t.Fatalf("required_status_checks is not a map, got %T", payload["required_status_checks"])
	}
	if scPayload["strict"] != true {
		t.Errorf("strict = %v, want true", scPayload["strict"])
	}
	contexts, ok := scPayload["contexts"].([]string)
	if !ok {
		t.Fatalf("contexts is not []string, got %T", scPayload["contexts"])
	}
	if len(contexts) != 2 || contexts[0] != "ci/test" || contexts[1] != "ci/lint" {
		t.Errorf("contexts = %v, want [ci/test ci/lint]", contexts)
	}
}

func TestBuildBranchProtectionPayload_NilReviews(t *testing.T) {
	bp := &manifest.BranchProtection{
		Pattern: "main",
	}

	payload := buildBranchProtectionPayload(bp)

	if payload["required_pull_request_reviews"] != nil {
		t.Errorf("expected nil required_pull_request_reviews, got %v", payload["required_pull_request_reviews"])
	}
	if payload["required_status_checks"] != nil {
		t.Errorf("expected nil required_status_checks, got %v", payload["required_status_checks"])
	}
}

func TestUpdateRepoField(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	err := exec.updateRepoField("myorg/myrepo", "merge_commit_title", "PR_TITLE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	args := mock.Called[0]
	expected := []string{"api", "repos/myorg/myrepo", "--method", "PATCH", "-f", "merge_commit_title=PR_TITLE"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestDerefBool(t *testing.T) {
	tests := []struct {
		name string
		val  *bool
		want bool
	}{
		{"nil returns false", nil, false},
		{"true returns true", boolPtr(true), true},
		{"false returns false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derefBool(tt.val)
			if got != tt.want {
				t.Errorf("derefBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
