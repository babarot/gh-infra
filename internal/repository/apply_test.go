package repository

import (
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
			"repo view myorg/myrepo --json repositoryTopics --jq .repositoryTopics[].name": []byte("old-topic\nkeep-topic\n"),
		},
	}
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeNoOp,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for noop, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Fatalf("expected 0 calls for noop, got %d", len(mock.Called))
	}
}

func TestApplyBranchProtection(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock, nil)

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

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	exec := NewExecutor(mock, nil)

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
		{"true returns true", new(true), true},
		{"false returns false", new(false), false},
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


func TestApplyRuleset_Create(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			Conditions: &manifest.RulesetConditions{
				RefName: &manifest.RulesetRefCondition{
					Include: []string{"refs/heads/main"},
				},
			},
			Rules: manifest.RulesetRules{
				NonFastForward: manifest.Ptr(true),
				Deletion:       manifest.Ptr(true),
			},
		},
	}

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: "Ruleset[protect-main]",
			Name:     "myorg/myrepo",
			Field:    "ruleset",
			NewValue: "protect-main",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	found := false
	for _, call := range mock.Called {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "repos/myorg/myrepo/rulesets") &&
			strings.Contains(joined, "POST") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected POST to rulesets endpoint, got calls: %v", mock.Called)
	}
}

func TestApplyRuleset_Update(t *testing.T) {
	// Mock returns list of rulesets for ID resolution
	listResp := `[{"id":42,"name":"protect-main","target":"branch"}]`
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/rulesets": []byte(listResp),
		},
	}
	exec := NewExecutor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("evaluate"),
			Target:      manifest.Ptr("branch"),
			Rules:       manifest.RulesetRules{},
		},
	}

	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Ruleset[protect-main]",
			Name:     "myorg/myrepo",
			Field:    "enforcement",
			OldValue: "active",
			NewValue: "evaluate",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	found := false
	for _, call := range mock.Called {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "repos/myorg/myrepo/rulesets/42") &&
			strings.Contains(joined, "PUT") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected PUT to rulesets/42 endpoint, got calls: %v", mock.Called)
	}
}

func TestBuildRulesetPayload(t *testing.T) {
	rs := &manifest.Ruleset{
		Name:        "protect-main",
		Target:      manifest.Ptr("branch"),
		Enforcement: manifest.Ptr("active"),
		BypassActors: []manifest.RulesetBypassActor{
			{Role: "admin", BypassMode: "always"},
		},
		Conditions: &manifest.RulesetConditions{
			RefName: &manifest.RulesetRefCondition{
				Include: []string{"refs/heads/main"},
			},
		},
		Rules: manifest.RulesetRules{
			PullRequest: &manifest.RulesetPullRequest{
				RequiredApprovingReviewCount: manifest.Ptr(1),
				DismissStaleReviewsOnPush:    manifest.Ptr(true),
			},
			RequiredStatusChecks: &manifest.RulesetStatusChecks{
				StrictRequiredStatusChecksPolicy: manifest.Ptr(true),
				Contexts: []manifest.RulesetStatusCheck{
					{Context: "ci/test"},
				},
			},
			NonFastForward:     manifest.Ptr(true),
			Deletion:           manifest.Ptr(true),
			Creation:           manifest.Ptr(false),
			RequiredSignatures: manifest.Ptr(true),
		},
	}

	// Use a mock resolver that resolves "admin" → 5
	mockRunner := &gh.MockRunner{}
	resolver := manifest.NewResolver(mockRunner, "test-owner")
	payload, err := buildRulesetPayload(rs, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload["name"] != "protect-main" {
		t.Errorf("name: got %v, want %q", payload["name"], "protect-main")
	}
	if payload["target"] != "branch" {
		t.Errorf("target: got %v, want %q", payload["target"], "branch")
	}
	if payload["enforcement"] != "active" {
		t.Errorf("enforcement: got %v, want %q", payload["enforcement"], "active")
	}

	rules, ok := payload["rules"].([]map[string]any)
	if !ok {
		t.Fatalf("rules is not []map[string]any, got %T", payload["rules"])
	}

	// Should have: pull_request, required_status_checks, non_fast_forward, deletion, required_signatures
	// NOT creation (false)
	ruleTypes := make(map[string]bool)
	for _, r := range rules {
		ruleTypes[r["type"].(string)] = true
	}
	for _, expected := range []string{"pull_request", "required_status_checks", "non_fast_forward", "deletion", "required_signatures"} {
		if !ruleTypes[expected] {
			t.Errorf("expected rule type %q not found in payload", expected)
		}
	}
	if ruleTypes["creation"] {
		t.Error("creation rule should not be in payload when set to false")
	}
}

func TestResolveRulesetID_Ambiguous(t *testing.T) {
	listResp := `[{"id":1,"name":"dup","target":"branch"},{"id":2,"name":"dup","target":"branch"}]`
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/o/r/rulesets": []byte(listResp),
		},
	}
	exec := NewExecutor(mock, nil)

	_, err := exec.resolveRulesetID("o", "r", "dup", "branch")
	if err == nil {
		t.Fatal("expected error for ambiguous rulesets")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("expected 'multiple' in error, got: %v", err)
	}
}

func TestResolveRulesetID_NotFound(t *testing.T) {
	listResp := `[{"id":1,"name":"other","target":"branch"}]`
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/o/r/rulesets": []byte(listResp),
		},
	}
	exec := NewExecutor(mock, nil)

	_, err := exec.resolveRulesetID("o", "r", "missing", "branch")
	if err == nil {
		t.Fatal("expected error for missing ruleset")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestBuildRulesetPayload_WithResolver(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	resolver := manifest.NewResolver(mock, "test-owner")

	rs := &manifest.Ruleset{
		Name:        "protect-main",
		Target:      manifest.Ptr("branch"),
		Enforcement: manifest.Ptr("active"),
		BypassActors: []manifest.RulesetBypassActor{
			{Role: "admin", BypassMode: "always"},
		},
		Rules: manifest.RulesetRules{
			RequiredStatusChecks: &manifest.RulesetStatusChecks{
				StrictRequiredStatusChecksPolicy: manifest.Ptr(true),
				Contexts: []manifest.RulesetStatusCheck{
					{Context: "ci/test", App: "github-actions"},
				},
			},
		},
	}

	payload, err := buildRulesetPayload(rs, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify bypass_actors resolved correctly
	actors, ok := payload["bypass_actors"].([]map[string]any)
	if !ok {
		t.Fatalf("bypass_actors is %T, want []map[string]any", payload["bypass_actors"])
	}
	if len(actors) != 1 {
		t.Fatalf("expected 1 bypass actor, got %d", len(actors))
	}
	if actors[0]["actor_id"] != 5 {
		t.Errorf("actor_id = %v, want 5", actors[0]["actor_id"])
	}
	if actors[0]["actor_type"] != "RepositoryRole" {
		t.Errorf("actor_type = %v, want RepositoryRole", actors[0]["actor_type"])
	}
	if actors[0]["bypass_mode"] != "always" {
		t.Errorf("bypass_mode = %v, want always", actors[0]["bypass_mode"])
	}

	// Verify status checks resolved correctly
	rules, ok := payload["rules"].([]map[string]any)
	if !ok {
		t.Fatalf("rules is %T, want []map[string]any", payload["rules"])
	}

	var scRule map[string]any
	for _, r := range rules {
		if r["type"] == "required_status_checks" {
			scRule = r
			break
		}
	}
	if scRule == nil {
		t.Fatal("required_status_checks rule not found in payload")
	}

	params, ok := scRule["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters is %T, want map[string]any", scRule["parameters"])
	}

	checks, ok := params["required_status_checks"].([]map[string]any)
	if !ok {
		t.Fatalf("required_status_checks is %T, want []map[string]any", params["required_status_checks"])
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 status check, got %d", len(checks))
	}
	if checks[0]["context"] != "ci/test" {
		t.Errorf("context = %v, want ci/test", checks[0]["context"])
	}
	if checks[0]["integration_id"] != 15368 {
		t.Errorf("integration_id = %v, want 15368", checks[0]["integration_id"])
	}
}
