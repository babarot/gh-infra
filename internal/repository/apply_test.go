package repository

import (
	"context"
	"encoding/json"
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

func TestApplyStatusTarget(t *testing.T) {
	tests := []struct {
		name string
		c    Change
		want string
	}{
		{
			name: "label",
			c:    Change{Resource: manifest.ResourceLabel, Field: "bug"},
			want: `label "bug"`,
		},
		{
			name: "milestone",
			c:    Change{Resource: manifest.ResourceMilestone, Field: "v1.0"},
			want: `milestone "v1.0"`,
		},
		{
			name: "repository field",
			c:    Change{Resource: manifest.ResourceRepository, Field: "description"},
			want: "repository description",
		},
		{
			name: "ruleset resource key",
			c:    Change{Resource: `Ruleset[main-protection]`, Field: "ruleset"},
			want: `ruleset "main-protection"`,
		},
		{
			name: "branch protection resource key",
			c:    Change{Resource: `BranchProtection[main]`, Field: "branch_protection"},
			want: `branch protection "main"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := applyStatusTarget(tt.c); got != tt.want {
				t.Fatalf("applyStatusTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyRepoDescription(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
			"repo view myorg/myrepo --json repositoryTopics --jq .repositoryTopics // [] | .[].name": []byte("old-topic\nkeep-topic\n"),
		},
	}
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--enable-wiki=false"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplySecurityFields(t *testing.T) {
	tests := []struct {
		name         string
		field        string
		wantEndpoint string
	}{
		{"vulnerability_alerts", "vulnerability_alerts", "repos/myorg/myrepo/vulnerability-alerts"},
		{"automated_security_fixes", "automated_security_fixes", "repos/myorg/myrepo/automated-security-fixes"},
		{"private_vulnerability_reporting", "private_vulnerability_reporting", "repos/myorg/myrepo/private-vulnerability-reporting"},
	}

	for _, tt := range tests {
		for _, sub := range []struct {
			name       string
			newValue   bool
			wantMethod string
		}{
			{"enable", true, "PUT"},
			{"disable", false, "DELETE"},
		} {
			t.Run(tt.name+"/"+sub.name, func(t *testing.T) {
				mock := &gh.MockRunner{}
				proc := NewProcessor(mock, nil)

				repo := newTestRepo("myorg", "myrepo")
				changes := []Change{
					{
						Type:     ChangeUpdate,
						Resource: "Repository",
						Name:     "myorg/myrepo",
						Field:    tt.field,
						NewValue: sub.newValue,
					},
				}

				results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
				if results[0].Err != nil {
					t.Fatalf("unexpected error: %v", results[0].Err)
				}
				args := mock.Called[0]
				expected := []string{"api", tt.wantEndpoint, "--method", sub.wantMethod}
				if strings.Join(args, " ") != strings.Join(expected, " ") {
					t.Errorf("args: got %v, want %v", args, expected)
				}
			})
		}
	}
}

func TestApplyRepoSetting_BoolTypeAssertionError(t *testing.T) {
	boolFields := []string{
		"release_immutability",
		"vulnerability_alerts",
		"automated_security_fixes",
		"private_vulnerability_reporting",
		"issues",
		"projects",
		"wiki",
		"discussions",
		"allow_merge_commit",
		"allow_squash_merge",
		"allow_rebase_merge",
		"auto_delete_head_branches",
	}

	for _, field := range boolFields {
		t.Run(field, func(t *testing.T) {
			mock := &gh.MockRunner{}
			proc := NewProcessor(mock, nil)

			repo := newTestRepo("myorg", "myrepo")
			changes := []Change{
				{
					Type:     ChangeUpdate,
					Resource: "Repository",
					Name:     "myorg/myrepo",
					Field:    field,
					NewValue: "not-a-bool", // wrong type
				},
			}

			results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].Err == nil {
				t.Fatal("expected error for non-bool NewValue, got nil")
			}
			if !strings.Contains(results[0].Err.Error(), "unexpected type") {
				t.Errorf("expected 'unexpected type' in error, got: %v", results[0].Err)
			}
		})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "no permission") {
		t.Errorf("expected user-friendly forbidden message, got %q", errMsg)
	}
}

func TestApplyWithErrValidation(t *testing.T) {
	validationErr := fmt.Errorf("%w: %w", gh.ErrValidation, &gh.ExitError{
		Cmd: "repo edit myorg/myrepo", ExitCode: 1,
		APIError: &gh.APIError{Status: 422, Message: "Validation Failed"},
	})

	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo edit myorg/myrepo --description new desc": validationErr,
		},
	}
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "validation failed") {
		t.Errorf("expected user-friendly validation message, got %q", errMsg)
	}
}

func TestApplyVariableSet(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeNoOp,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for noop, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Fatalf("expected 0 calls for noop, got %d", len(mock.Called))
	}
}

func TestApplyBranchProtection(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

	err := proc.updateRepoField(context.Background(), "myorg/myrepo", "merge_commit_title", "PR_TITLE")
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	proc := NewProcessor(mock, nil)

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

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
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
	payload, err := buildRulesetPayload(context.Background(), rs, resolver)
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
		if typ, ok := r["type"].(string); ok {
			ruleTypes[typ] = true
		}
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
	proc := NewProcessor(mock, nil)

	_, err := proc.resolveRulesetID(context.Background(), rulesetLookup{
		Repo:        "o/r",
		RulesetName: "dup",
		Target:      "branch",
	})
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
	proc := NewProcessor(mock, nil)

	_, err := proc.resolveRulesetID(context.Background(), rulesetLookup{
		Repo:        "o/r",
		RulesetName: "missing",
		Target:      "branch",
	})
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

	payload, err := buildRulesetPayload(context.Background(), rs, resolver)
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

func TestApplyRepoPatch_BatchesSettings(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Features = &manifest.Features{
		Projects:    manifest.Ptr(false),
		Discussions: manifest.Ptr(false),
	}
	repo.Spec.MergeStrategy = &manifest.MergeStrategy{
		AllowMergeCommit:         manifest.Ptr(true),
		AllowSquashMerge:         manifest.Ptr(true),
		AllowRebaseMerge:         manifest.Ptr(false),
		AllowAutoMerge:           manifest.Ptr(true),
		AutoDeleteHeadBranches:   manifest.Ptr(true),
		SquashMergeCommitTitle:   manifest.Ptr("COMMIT_OR_PR_TITLE"),
		SquashMergeCommitMessage: manifest.Ptr("COMMIT_MESSAGES"),
		MergeCommitTitle:         manifest.Ptr("MERGE_MESSAGE"),
		MergeCommitMessage:       manifest.Ptr("PR_TITLE"),
	}
	hp := "https://example.com"
	repo.Spec.Homepage = &hp

	err := proc.applyRepoPatch(context.Background(), "myorg/myrepo", repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be exactly 1 API call (batched PATCH)
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}

	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "repos/myorg/myrepo") {
		t.Errorf("expected repos endpoint, got: %s", call)
	}
	if !strings.Contains(call, "--method PATCH") {
		t.Errorf("expected PATCH method, got: %s", call)
	}

	// Verify JSON payload
	body := mock.CalledStdin[0]
	if body == nil {
		t.Fatal("expected stdin body, got nil")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to parse stdin payload: %v", err)
	}

	// Features
	if payload["has_projects"] != false {
		t.Errorf("has_projects = %v, want false", payload["has_projects"])
	}
	if payload["has_discussions"] != false {
		t.Errorf("has_discussions = %v, want false", payload["has_discussions"])
	}

	// Merge strategy
	if payload["allow_merge_commit"] != true {
		t.Errorf("allow_merge_commit = %v, want true", payload["allow_merge_commit"])
	}
	if payload["allow_squash_merge"] != true {
		t.Errorf("allow_squash_merge = %v, want true", payload["allow_squash_merge"])
	}
	if payload["allow_rebase_merge"] != false {
		t.Errorf("allow_rebase_merge = %v, want false", payload["allow_rebase_merge"])
	}
	if payload["allow_auto_merge"] != true {
		t.Errorf("allow_auto_merge = %v, want true", payload["allow_auto_merge"])
	}
	if payload["delete_branch_on_merge"] != true {
		t.Errorf("delete_branch_on_merge = %v, want true", payload["delete_branch_on_merge"])
	}
	if payload["squash_merge_commit_title"] != "COMMIT_OR_PR_TITLE" {
		t.Errorf("squash_merge_commit_title = %v, want COMMIT_OR_PR_TITLE", payload["squash_merge_commit_title"])
	}
	if payload["squash_merge_commit_message"] != "COMMIT_MESSAGES" {
		t.Errorf("squash_merge_commit_message = %v, want COMMIT_MESSAGES", payload["squash_merge_commit_message"])
	}
	if payload["merge_commit_title"] != "MERGE_MESSAGE" {
		t.Errorf("merge_commit_title = %v, want MERGE_MESSAGE", payload["merge_commit_title"])
	}
	if payload["merge_commit_message"] != "PR_TITLE" {
		t.Errorf("merge_commit_message = %v, want PR_TITLE", payload["merge_commit_message"])
	}

	// Homepage
	if payload["homepage"] != "https://example.com" {
		t.Errorf("homepage = %v, want https://example.com", payload["homepage"])
	}
}

func TestApplyRepoPatch_Empty(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	// No features, merge strategy, or homepage set

	err := proc.applyRepoPatch(context.Background(), "myorg/myrepo", repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected 0 gh calls for empty settings, got %d", len(mock.Called))
	}
}

func TestApplyMergeStrategyBatch(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "merge_strategy",
			Children: []Change{
				{Field: "allow_squash_merge", NewValue: true},
				{Field: "squash_merge_commit_title", NewValue: "PR_TITLE"},
				{Field: "auto_delete_head_branches", NewValue: true},
			},
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	// Should be exactly 1 API call (batched PATCH)
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}

	body := mock.CalledStdin[0]
	if body == nil {
		t.Fatal("expected stdin body, got nil")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to parse stdin payload: %v", err)
	}
	if payload["allow_squash_merge"] != true {
		t.Errorf("allow_squash_merge = %v, want true", payload["allow_squash_merge"])
	}
	if payload["squash_merge_commit_title"] != "PR_TITLE" {
		t.Errorf("squash_merge_commit_title = %v, want PR_TITLE", payload["squash_merge_commit_title"])
	}
	// auto_delete_head_branches should be mapped to delete_branch_on_merge
	if payload["delete_branch_on_merge"] != true {
		t.Errorf("delete_branch_on_merge = %v, want true", payload["delete_branch_on_merge"])
	}
	if _, ok := payload["auto_delete_head_branches"]; ok {
		t.Error("auto_delete_head_branches should be mapped to delete_branch_on_merge, not sent directly")
	}
}

func TestApplyAllSettings_EmptyActions(t *testing.T) {
	// actions: {} should not panic during new repo creation.
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Actions = &manifest.Actions{} // empty — Enabled is nil

	// applyAllSettings is called after createRepo for new repos.
	err := proc.applyAllSettings(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error for empty actions block: %v", err)
	}
}

func TestApplyActionsPermissions_WithSHAPinningRequired(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsPermissions(context.Background(), "myorg", "myrepo", &manifest.Actions{
		Enabled:            manifest.Ptr(true),
		AllowedActions:     manifest.Ptr("all"),
		SHAPinningRequired: manifest.Ptr(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}
	// Verify args use --input - instead of --body
	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "actions/permissions") {
		t.Errorf("expected actions/permissions endpoint, got: %s", call)
	}
	if !strings.Contains(call, "--input -") {
		t.Errorf("expected --input - flag, got: %s", call)
	}
	// Verify JSON payload sent via stdin
	body := mock.CalledStdin[0]
	if body == nil {
		t.Fatal("expected stdin body, got nil")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to parse stdin payload: %v", err)
	}
	if payload["enabled"] != true {
		t.Errorf("enabled = %v, want true", payload["enabled"])
	}
	if payload["allowed_actions"] != "all" {
		t.Errorf("allowed_actions = %v, want all", payload["allowed_actions"])
	}
	if payload["sha_pinning_required"] != true {
		t.Errorf("sha_pinning_required = %v, want true", payload["sha_pinning_required"])
	}
}

func TestApplyActionsWorkflow(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsWorkflow(context.Background(), "myorg", "myrepo", &manifest.Actions{
		WorkflowPermissions:    manifest.Ptr("read"),
		CanApprovePullRequests: manifest.Ptr(false),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}
	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "actions/permissions/workflow") {
		t.Errorf("expected workflow endpoint, got: %s", call)
	}
}

func TestApplyActionsSelectedActions(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsSelectedActions(context.Background(), "myorg", "myrepo", &manifest.Actions{
		SelectedActions: &manifest.SelectedActions{
			GithubOwnedAllowed: manifest.Ptr(true),
			VerifiedAllowed:    manifest.Ptr(false),
			PatternsAllowed:    []string{"actions/*"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}
	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "actions/permissions/selected-actions") {
		t.Errorf("expected selected-actions endpoint, got: %s", call)
	}
}

func TestApplyActionsSelectedActions_NilSelectedActions(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsSelectedActions(context.Background(), "myorg", "myrepo", &manifest.Actions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected no calls for nil SelectedActions, got %d", len(mock.Called))
	}
}

func TestApplyActionsForkPR(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsForkPR(context.Background(), "myorg", "myrepo", &manifest.Actions{
		ForkPRApproval: manifest.Ptr("first_time_contributors"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(mock.Called))
	}
	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "fork-pr-contributor-approval") {
		t.Errorf("expected fork-pr endpoint, got: %s", call)
	}
}

func TestApplyActionsForkPR_Nil(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	err := proc.applyActionsForkPR(context.Background(), "myorg", "myrepo", &manifest.Actions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected no calls for nil ForkPRApproval, got %d", len(mock.Called))
	}
}

func TestApplyActions_RoutesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		wantCall string
	}{
		{"enabled", "enabled", "actions/permissions"},
		{"allowed_actions", "allowed_actions", "actions/permissions"},
		{"workflow_permissions", "workflow_permissions", "actions/permissions/workflow"},
		{"can_approve_pull_requests", "can_approve_pull_requests", "actions/permissions/workflow"},
		{"fork_pr_approval", "fork_pr_approval", "fork-pr-contributor-approval"},
		{"selected_actions", "selected_actions.github_owned_allowed", "actions/permissions/selected-actions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{}
			proc := NewProcessor(mock, nil)

			repo := newTestRepo("myorg", "myrepo")
			repo.Spec.Actions = &manifest.Actions{
				Enabled:             manifest.Ptr(true),
				AllowedActions:      manifest.Ptr("selected"),
				WorkflowPermissions: manifest.Ptr("read"),
				ForkPRApproval:      manifest.Ptr("first_time_contributors"),
				SelectedActions: &manifest.SelectedActions{
					GithubOwnedAllowed: manifest.Ptr(true),
				},
			}

			c := Change{Type: ChangeUpdate, Field: tt.field, Resource: manifest.ResourceActions}
			_ = proc.applyActions(context.Background(), c, repo)

			if len(mock.Called) == 0 {
				t.Fatal("expected at least 1 gh call")
			}
			call := strings.Join(mock.Called[0], " ")
			if !strings.Contains(call, tt.wantCall) {
				t.Errorf("field %q: expected call containing %q, got: %s", tt.field, tt.wantCall, call)
			}
		})
	}
}

func TestApplyMilestone_Create(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Milestones = []manifest.Milestone{
		{Title: "v1.0", Description: "First release", State: manifest.Ptr("open"), DueOn: manifest.Ptr("2026-06-01")},
	}

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: manifest.ResourceMilestone,
			Name:     "myorg/myrepo",
			Field:    "v1.0",
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}

	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "repos/myorg/myrepo/milestones") {
		t.Errorf("expected milestones endpoint, got: %s", call)
	}
	if !strings.Contains(call, "--method POST") {
		t.Errorf("expected POST method, got: %s", call)
	}

	body := mock.CalledStdin[0]
	if body == nil {
		t.Fatal("expected stdin body, got nil")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}
	if payload["title"] != "v1.0" {
		t.Errorf("title = %v, want v1.0", payload["title"])
	}
	if payload["state"] != "open" {
		t.Errorf("state = %v, want open", payload["state"])
	}
	if payload["due_on"] != "2026-06-01T00:00:00Z" {
		t.Errorf("due_on = %v, want 2026-06-01T00:00:00Z", payload["due_on"])
	}
}

func TestApplyMilestone_Update(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[{"number":3,"title":"v1.0"}]`),
		},
	}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Milestones = []manifest.Milestone{
		{Title: "v1.0", State: manifest.Ptr("closed")},
	}

	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceMilestone,
			Name:     "myorg/myrepo",
			Field:    "v1.0",
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	// First call is the findMilestoneNumber lookup, second is the PATCH
	if len(mock.Called) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(mock.Called))
	}

	patchCall := strings.Join(mock.Called[1], " ")
	if !strings.Contains(patchCall, "repos/myorg/myrepo/milestones/3") {
		t.Errorf("expected PATCH to milestone 3, got: %s", patchCall)
	}
	if !strings.Contains(patchCall, "--method PATCH") {
		t.Errorf("expected PATCH method, got: %s", patchCall)
	}
}

func TestApplyMilestone_NotFoundInDesired(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	// No milestones in spec

	changes := []Change{
		{
			Type:     ChangeCreate,
			Resource: manifest.ResourceMilestone,
			Name:     "myorg/myrepo",
			Field:    "v1.0",
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err == nil {
		t.Fatal("expected error for missing milestone in desired state")
	}
	if !strings.Contains(results[0].Err.Error(), "not found in desired state") {
		t.Errorf("unexpected error message: %v", results[0].Err)
	}
}

func TestFindMilestoneNumber(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[{"number":1,"title":"v0.9"},{"number":5,"title":"v1.0"}]`),
			},
		}
		proc := NewProcessor(mock, nil)
		num, err := proc.findMilestoneNumber(context.Background(), "myorg", "myrepo", "v1.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if num != 5 {
			t.Errorf("expected 5, got %d", num)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[{"number":1,"title":"v0.9"}]`),
			},
		}
		proc := NewProcessor(mock, nil)
		_, err := proc.findMilestoneNumber(context.Background(), "myorg", "myrepo", "v1.0")
		if err == nil {
			t.Fatal("expected error for not found milestone")
		}
	})
}

func TestApplyLabel_UpdateWithChildren(t *testing.T) {
	mock := &gh.MockRunner{}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Labels = []manifest.Label{
		{Name: "enhancement", Color: "eeeeee", Description: "New feature"},
	}

	// Label update produces a Change with Children (one per changed field).
	// The apply logic must NOT expand children individually — it should apply
	// the parent change as a single "label edit" call.
	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceLabel,
			Name:     "myorg/myrepo",
			Field:    "enhancement",
			Children: []Change{
				{Type: ChangeUpdate, Field: "color", OldValue: "a2eeef", NewValue: "eeeeee"},
			},
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	call := strings.Join(mock.Called[0], " ")
	if !strings.Contains(call, "label edit enhancement") {
		t.Errorf("expected 'label edit enhancement', got: %s", call)
	}
}

func TestApplyMilestone_UpdateWithChildren(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[{"number":1,"title":"v1.0"}]`),
		},
	}
	proc := NewProcessor(mock, nil)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Milestones = []manifest.Milestone{
		{Title: "v1.0", State: manifest.Ptr("open"), DueOn: manifest.Ptr("2026-06-01")},
	}

	changes := []Change{
		{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceMilestone,
			Name:     "myorg/myrepo",
			Field:    "v1.0",
			Children: []Change{
				{Type: ChangeUpdate, Field: "due_on", OldValue: "2026-05-31", NewValue: "2026-06-01"},
			},
		},
	}

	results := proc.Apply(context.Background(), changes, []*manifest.Repository{repo}, ui.NoopReporter{})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	// First call: findMilestoneNumber, second call: PATCH
	if len(mock.Called) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(mock.Called))
	}
	patchCall := strings.Join(mock.Called[1], " ")
	if !strings.Contains(patchCall, "milestones/1") {
		t.Errorf("expected PATCH to milestone 1, got: %s", patchCall)
	}
}
