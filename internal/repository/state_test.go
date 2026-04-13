package repository

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
)

func TestNewProcessor(t *testing.T) {
	mock := &gh.MockRunner{}
	p := NewProcessor(mock, nil, nil)
	if p == nil {
		t.Fatal("expected non-nil Processor")
		return
	}
	if p.runner != mock {
		t.Error("expected runner to be the mock")
	}
}

func TestFetchRepository(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"repo view myorg/myrepo --json description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": []byte(`{
				"description": "A test repo",
				"homepageUrl": "https://example.com",
				"visibility": "PUBLIC",
				"isArchived": false,
				"repositoryTopics": [{"name": "go"}, {"name": "cli"}],
				"hasIssuesEnabled": true,
				"hasProjectsEnabled": false,
				"hasWikiEnabled": true,
				"hasDiscussionsEnabled": false,
				"mergeCommitAllowed": true,
				"squashMergeAllowed": true,
				"rebaseMergeAllowed": false,
				"deleteBranchOnMerge": true,
				"defaultBranchRef": {"name": "main"}
			}`),
			"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}": []byte(`{
				"squash_merge_commit_title": "PR_TITLE",
				"squash_merge_commit_message": "COMMIT_MESSAGES",
				"merge_commit_title": "MERGE_MESSAGE",
				"merge_commit_message": "PR_BODY"
			}`),
			"api repos/myorg/myrepo/immutable-releases":                                       []byte(`{"enabled": false}`),
			"api repos/myorg/myrepo/vulnerability-alerts":                                     []byte(``),
			"api repos/myorg/myrepo/automated-security-fixes":                                 []byte(`{"enabled": false, "paused": false}`),
			"api repos/myorg/myrepo/private-vulnerability-reporting":                          []byte(`{"enabled": false}`),
			"api repos/myorg/myrepo/branches --jq [.[] | select(.protected == true) | .name]": []byte(`[]`),
			"secret list --repo myorg/myrepo --json name --jq .[].name":                       []byte("SECRET1\nSECRET2"),
			"variable list --repo myorg/myrepo --json name,value":                             []byte(`[{"name":"VAR1","value":"val1"},{"name":"VAR2","value":"val2"}]`),
		},
	}

	p := NewProcessor(mock, nil, nil)
	state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Basic fields
	if state.Owner != "myorg" {
		t.Errorf("Owner = %q, want myorg", state.Owner)
	}
	if state.Name != "myrepo" {
		t.Errorf("Name = %q, want myrepo", state.Name)
	}
	if state.Description != "A test repo" {
		t.Errorf("Description = %q, want 'A test repo'", state.Description)
	}
	if state.Homepage != "https://example.com" {
		t.Errorf("Homepage = %q, want 'https://example.com'", state.Homepage)
	}
	if state.Visibility != "public" {
		t.Errorf("Visibility = %q, want 'public'", state.Visibility)
	}

	// Topics
	if len(state.Topics) != 2 || state.Topics[0] != "go" || state.Topics[1] != "cli" {
		t.Errorf("Topics = %v, want [go cli]", state.Topics)
	}

	// Features
	if !state.Features.Issues {
		t.Error("expected Issues = true")
	}
	if state.Features.Projects {
		t.Error("expected Projects = false")
	}
	if !state.Features.Wiki {
		t.Error("expected Wiki = true")
	}
	if state.Features.Discussions {
		t.Error("expected Discussions = false")
	}

	// Merge strategy
	if !state.MergeStrategy.AllowMergeCommit {
		t.Error("expected AllowMergeCommit = true")
	}
	if !state.MergeStrategy.AllowSquashMerge {
		t.Error("expected AllowSquashMerge = true")
	}
	if state.MergeStrategy.AllowRebaseMerge {
		t.Error("expected AllowRebaseMerge = false")
	}
	if !state.MergeStrategy.AutoDeleteHeadBranches {
		t.Error("expected AutoDeleteHeadBranches = true")
	}

	// Commit message settings
	if state.MergeStrategy.SquashMergeCommitTitle != "PR_TITLE" {
		t.Errorf("SquashMergeCommitTitle = %q, want PR_TITLE", state.MergeStrategy.SquashMergeCommitTitle)
	}
	if state.MergeStrategy.SquashMergeCommitMessage != "COMMIT_MESSAGES" {
		t.Errorf("SquashMergeCommitMessage = %q, want COMMIT_MESSAGES", state.MergeStrategy.SquashMergeCommitMessage)
	}
	if state.MergeStrategy.MergeCommitTitle != "MERGE_MESSAGE" {
		t.Errorf("MergeCommitTitle = %q, want MERGE_MESSAGE", state.MergeStrategy.MergeCommitTitle)
	}
	if state.MergeStrategy.MergeCommitMessage != "PR_BODY" {
		t.Errorf("MergeCommitMessage = %q, want PR_BODY", state.MergeStrategy.MergeCommitMessage)
	}

	// Secrets
	if len(state.Secrets) != 2 || state.Secrets[0] != "SECRET1" || state.Secrets[1] != "SECRET2" {
		t.Errorf("Secrets = %v, want [SECRET1 SECRET2]", state.Secrets)
	}

	// Variables
	if len(state.Variables) != 2 {
		t.Fatalf("Variables count = %d, want 2", len(state.Variables))
	}
	if state.Variables["VAR1"] != "val1" {
		t.Errorf("Variables[VAR1] = %q, want val1", state.Variables["VAR1"])
	}
	if state.Variables["VAR2"] != "val2" {
		t.Errorf("Variables[VAR2] = %q, want val2", state.Variables["VAR2"])
	}

	// Branch protection should be empty (no protected branches)
	if len(state.BranchProtection) != 0 {
		t.Errorf("BranchProtection count = %d, want 0", len(state.BranchProtection))
	}
}

func TestFetchRepository_RepoSettingsError(t *testing.T) {
	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo view myorg/myrepo --json description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": fmt.Errorf("not found"),
		},
	}

	p := NewProcessor(mock, nil, nil)
	_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
	if err == nil {
		t.Fatal("expected error from fetchRepoSettings")
	}
	if got := err.Error(); got != "fetch repo myorg/myrepo: not found" {
		t.Errorf("unexpected error: %q", got)
	}
}

func TestFetchSecrets(t *testing.T) {
	t.Run("multiple secrets", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"secret list --repo myorg/myrepo --json name --jq .[].name": []byte("SECRET1\nSECRET2\nSECRET3"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		secrets, err := p.fetchSecrets(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 3 {
			t.Fatalf("expected 3 secrets, got %d", len(secrets))
		}
		if secrets[0] != "SECRET1" || secrets[1] != "SECRET2" || secrets[2] != "SECRET3" {
			t.Errorf("secrets = %v, want [SECRET1 SECRET2 SECRET3]", secrets)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"secret list --repo myorg/myrepo --json name --jq .[].name": []byte(""),
			},
		}
		p := NewProcessor(mock, nil, nil)
		secrets, err := p.fetchSecrets(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if secrets != nil {
			t.Errorf("expected nil secrets for empty response, got %v", secrets)
		}
	})

	t.Run("permission error returns nil", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"secret list --repo myorg/myrepo --json name --jq .[].name": gh.ErrForbidden,
			},
		}
		p := NewProcessor(mock, nil, nil)
		secrets, err := p.fetchSecrets(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if secrets != nil {
			t.Errorf("expected nil secrets on error, got %v", secrets)
		}
	})

	t.Run("unexpected error propagates", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"secret list --repo myorg/myrepo --json name --jq .[].name": fmt.Errorf("network timeout"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.fetchSecrets(context.Background(), "myorg", "myrepo")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestFetchVariables(t *testing.T) {
	t.Run("multiple variables", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"variable list --repo myorg/myrepo --json name,value": []byte(`[{"name":"VAR1","value":"val1"},{"name":"VAR2","value":"val2"}]`),
			},
		}
		p := NewProcessor(mock, nil, nil)
		vars, err := p.fetchVariables(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vars) != 2 {
			t.Fatalf("expected 2 variables, got %d", len(vars))
		}
		if vars["VAR1"] != "val1" {
			t.Errorf("VAR1 = %q, want val1", vars["VAR1"])
		}
		if vars["VAR2"] != "val2" {
			t.Errorf("VAR2 = %q, want val2", vars["VAR2"])
		}
	})

	t.Run("empty array", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"variable list --repo myorg/myrepo --json name,value": []byte(`[]`),
			},
		}
		p := NewProcessor(mock, nil, nil)
		vars, err := p.fetchVariables(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vars) != 0 {
			t.Errorf("expected 0 variables, got %d", len(vars))
		}
	})

	t.Run("permission error returns nil", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"variable list --repo myorg/myrepo --json name,value": gh.ErrForbidden,
			},
		}
		p := NewProcessor(mock, nil, nil)
		vars, err := p.fetchVariables(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if vars != nil {
			t.Errorf("expected nil vars on error, got %v", vars)
		}
	})

	t.Run("unexpected error propagates", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"variable list --repo myorg/myrepo --json name,value": fmt.Errorf("network timeout"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.fetchVariables(context.Background(), "myorg", "myrepo")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestFetchCommitMessageSettings_NullValues(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}": []byte(`{
				"squash_merge_commit_title": null,
				"squash_merge_commit_message": null,
				"merge_commit_title": null,
				"merge_commit_message": null
			}`),
		},
	}

	p := NewProcessor(mock, nil, nil)
	settings, err := p.fetchCommitMessageSettings(context.Background(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if settings.MergeCommitTitle != "MERGE_MESSAGE" {
		t.Errorf("MergeCommitTitle = %q, want MERGE_MESSAGE", settings.MergeCommitTitle)
	}
	if settings.MergeCommitMessage != "PR_TITLE" {
		t.Errorf("MergeCommitMessage = %q, want PR_TITLE", settings.MergeCommitMessage)
	}
	if settings.SquashMergeCommitTitle != "COMMIT_OR_PR_TITLE" {
		t.Errorf("SquashMergeCommitTitle = %q, want COMMIT_OR_PR_TITLE", settings.SquashMergeCommitTitle)
	}
	if settings.SquashMergeCommitMessage != "COMMIT_MESSAGES" {
		t.Errorf("SquashMergeCommitMessage = %q, want COMMIT_MESSAGES", settings.SquashMergeCommitMessage)
	}
}

func TestCurrentState_FullName(t *testing.T) {
	s := &CurrentState{Owner: "myorg", Name: "myrepo"}
	if got := s.FullName(); got != "myorg/myrepo" {
		t.Errorf("FullName() = %q, want myorg/myrepo", got)
	}
}

func TestFetchRepoSettings_FetchErrorHandling(t *testing.T) {
	// Base responses required for fetchRepoSettings to succeed
	baseResponses := map[string][]byte{
		"repo view myorg/myrepo --json description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": []byte(`{
			"description": "A test repo",
			"homepageUrl": "",
			"visibility": "PUBLIC",
			"isArchived": false,
			"repositoryTopics": [],
			"hasIssuesEnabled": true,
			"hasProjectsEnabled": false,
			"hasWikiEnabled": true,
			"hasDiscussionsEnabled": false,
			"mergeCommitAllowed": true,
			"squashMergeAllowed": true,
			"rebaseMergeAllowed": false,
			"deleteBranchOnMerge": true,
			"defaultBranchRef": {"name": "main"}
		}`),
		"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}": []byte(`{
			"squash_merge_commit_title": "PR_TITLE",
			"squash_merge_commit_message": "COMMIT_MESSAGES",
			"merge_commit_title": "MERGE_MESSAGE",
			"merge_commit_message": "PR_BODY"
		}`),
		"api repos/myorg/myrepo/immutable-releases":                                       []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/automated-security-fixes":                                 []byte(`{"enabled": false, "paused": false}`),
		"api repos/myorg/myrepo/private-vulnerability-reporting":                          []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/branches --jq [.[] | select(.protected == true) | .name]": []byte(`[]`),
	}

	t.Run("commit message settings 404 is ignored", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}": fmt.Errorf("%w: api error", gh.ErrNotFound),
			},
		}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("expected no error for 404, got: %v", err)
		}
		if state.MergeStrategy.MergeCommitTitle != "" {
			t.Errorf("expected empty MergeCommitTitle, got %q", state.MergeStrategy.MergeCommitTitle)
		}
	})

	t.Run("release immutability 403 is ignored", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/immutable-releases")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/immutable-releases": fmt.Errorf("%w: api error", gh.ErrForbidden),
			},
		}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("expected no error for 403, got: %v", err)
		}
		if state.ReleaseImmutability {
			t.Error("expected ReleaseImmutability = false for 403")
		}
	})

	t.Run("commit message settings 500 propagates error", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}": fmt.Errorf("internal server error"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}
		if !strings.Contains(err.Error(), "fetch commit message settings") {
			t.Errorf("expected 'fetch commit message settings' in error, got: %v", err)
		}
	})

	t.Run("release immutability 500 propagates error", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/immutable-releases")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/immutable-releases": fmt.Errorf("internal server error"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}
		if !strings.Contains(err.Error(), "fetch release immutability") {
			t.Errorf("expected 'fetch release immutability' in error, got: %v", err)
		}
	})

	t.Run("vulnerability alerts 204 means enabled", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: baseResponses}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !state.Security.VulnerabilityAlerts {
			t.Error("expected VulnerabilityAlerts = true for 204 response")
		}
	})

	t.Run("vulnerability alerts 404 means disabled", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/vulnerability-alerts")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/vulnerability-alerts": fmt.Errorf("%w: api error", gh.ErrNotFound),
			},
		}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("expected no error for 404, got: %v", err)
		}
		if state.Security.VulnerabilityAlerts {
			t.Error("expected VulnerabilityAlerts = false for 404")
		}
	})

	t.Run("vulnerability alerts 403 is ignored", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/vulnerability-alerts")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/vulnerability-alerts": fmt.Errorf("%w: api error", gh.ErrForbidden),
			},
		}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("expected no error for 403, got: %v", err)
		}
		if state.Security.VulnerabilityAlerts {
			t.Error("expected VulnerabilityAlerts = false for 403")
		}
	})

	t.Run("vulnerability alerts 500 propagates error", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/vulnerability-alerts")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/vulnerability-alerts": fmt.Errorf("internal server error"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}
		if !strings.Contains(err.Error(), "fetch vulnerability alerts") {
			t.Errorf("expected 'fetch vulnerability alerts' in error, got: %v", err)
		}
	})

	t.Run("automated security fixes 404 means disabled", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/automated-security-fixes")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/automated-security-fixes": fmt.Errorf("%w: api error", gh.ErrNotFound),
			},
		}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("expected no error for 404, got: %v", err)
		}
		if state.Security.AutomatedSecurityFixes {
			t.Error("expected AutomatedSecurityFixes = false for 404")
		}
	})

	t.Run("automated security fixes 500 propagates error", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/automated-security-fixes")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/automated-security-fixes": fmt.Errorf("internal server error"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}
		if !strings.Contains(err.Error(), "fetch automated security fixes") {
			t.Errorf("expected 'fetch automated security fixes' in error, got: %v", err)
		}
	})

	t.Run("private vulnerability reporting enabled", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		responses["api repos/myorg/myrepo/private-vulnerability-reporting"] = []byte(`{"enabled": true}`)

		mock := &gh.MockRunner{Responses: responses}
		p := NewProcessor(mock, nil, nil)
		state, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !state.Security.PrivateVulnerabilityReporting {
			t.Error("expected PrivateVulnerabilityReporting = true")
		}
	})

	t.Run("private vulnerability reporting 500 propagates error", func(t *testing.T) {
		responses := make(map[string][]byte)
		maps.Copy(responses, baseResponses)
		delete(responses, "api repos/myorg/myrepo/private-vulnerability-reporting")

		mock := &gh.MockRunner{
			Responses: responses,
			Errors: map[string]error{
				"api repos/myorg/myrepo/private-vulnerability-reporting": fmt.Errorf("internal server error"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.FetchRepository(context.Background(), "myorg", "myrepo", nil)
		if err == nil {
			t.Fatal("expected error for 500, got nil")
		}
		if !strings.Contains(err.Error(), "fetch private vulnerability reporting") {
			t.Errorf("expected 'fetch private vulnerability reporting' in error, got: %v", err)
		}
	})
}

func TestFetchActionsSettings(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/actions/permissions": []byte(
				`{"enabled":true,"allowed_actions":"all","sha_pinning_required":true}`,
			),
			"api repos/myorg/myrepo/actions/permissions/workflow":                     []byte(`{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`),
			"api repos/myorg/myrepo/actions/permissions/fork-pr-contributor-approval": []byte(`{"approval_policy":"first_time_contributors"}`),
		},
	}

	p := NewProcessor(mock, nil, nil)
	actions, err := p.fetchActionsSettings(context.Background(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actions.Enabled {
		t.Error("expected Enabled = true")
	}
	if actions.AllowedActions != "all" {
		t.Errorf("AllowedActions = %q, want all", actions.AllowedActions)
	}
	if !actions.SHAPinningRequired {
		t.Error("expected SHAPinningRequired = true")
	}
	if actions.WorkflowPermissions != "read" {
		t.Errorf("WorkflowPermissions = %q, want read", actions.WorkflowPermissions)
	}
	if actions.CanApprovePullRequests {
		t.Error("expected CanApprovePullRequests = false")
	}
	if actions.ForkPRApproval != "first_time_contributors" {
		t.Errorf("ForkPRApproval = %q, want first_time_contributors", actions.ForkPRApproval)
	}
}

func TestNormalizeDueOn(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"RFC3339 timestamp", "2026-06-01T00:00:00Z", "2026-06-01"},
		{"RFC3339 with timezone", "2026-12-25T08:00:00+09:00", "2026-12-25"},
		{"already YYYY-MM-DD", "2026-06-01", "2026-06-01"},
		{"empty string", "", ""},
		{"null string", "null", ""},
		{"unparseable", "not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDueOn(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDueOn(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFetchMilestones(t *testing.T) {
	t.Run("parses milestones", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[
					{"number":1,"title":"v1.0","description":"First release","state":"open","due_on":"2026-06-01T00:00:00Z"},
					{"number":2,"title":"v2.0","description":"","state":"closed","due_on":null}
				]`),
			},
		}
		p := NewProcessor(mock, nil, nil)
		milestones, err := p.fetchMilestones(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(milestones) != 2 {
			t.Fatalf("expected 2 milestones, got %d", len(milestones))
		}
		m1 := milestones["v1.0"]
		if m1.Number != 1 || m1.State != "open" || m1.DueOn != "2026-06-01" {
			t.Errorf("v1.0 = %+v", m1)
		}
		m2 := milestones["v2.0"]
		if m2.Number != 2 || m2.State != "closed" || m2.DueOn != "" {
			t.Errorf("v2.0 = %+v", m2)
		}
	})

	t.Run("permission error returns nil", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": gh.ErrForbidden,
			},
		}
		p := NewProcessor(mock, nil, nil)
		milestones, err := p.fetchMilestones(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if milestones != nil {
			t.Errorf("expected nil milestones on error, got %v", milestones)
		}
	})

	t.Run("unexpected error propagates", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": fmt.Errorf("network timeout"),
			},
		}
		p := NewProcessor(mock, nil, nil)
		_, err := p.fetchMilestones(context.Background(), "myorg", "myrepo")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty array", func(t *testing.T) {
		mock := &gh.MockRunner{
			Responses: map[string][]byte{
				"api repos/myorg/myrepo/milestones?state=all&per_page=100 --paginate": []byte(`[]`),
			},
		}
		p := NewProcessor(mock, nil, nil)
		milestones, err := p.fetchMilestones(context.Background(), "myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(milestones) != 0 {
			t.Errorf("expected 0 milestones, got %d", len(milestones))
		}
	})
}

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "unknown"},
		{"minutes ago", time.Now().Add(-30 * time.Minute), "30m ago"},
		{"hours ago", time.Now().Add(-5 * time.Hour), "5h ago"},
		{"days ago", time.Now().Add(-3 * 24 * time.Hour), "3d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimeAgo(tt.t)
			if got != tt.want {
				t.Errorf("formatTimeAgo() = %q, want %q", got, tt.want)
			}
		})
	}
}
