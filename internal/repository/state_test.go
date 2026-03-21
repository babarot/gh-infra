package repository

import (
	"fmt"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
)

func TestNewFetcher(t *testing.T) {
	mock := &gh.MockRunner{}
	f := NewFetcher(mock)
	if f == nil {
		t.Fatal("expected non-nil Fetcher")
	}
	if f.runner != mock {
		t.Error("expected runner to be the mock")
	}
}

func TestFetchRepository(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"repo view myorg/myrepo --json description,homepageUrl,visibility,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": []byte(`{
				"description": "A test repo",
				"homepageUrl": "https://example.com",
				"visibility": "PUBLIC",
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
			"api repos/myorg/myrepo/branches --jq [.[] | select(.protected == true) | .name]": []byte(`[]`),
			"secret list --repo myorg/myrepo --json name --jq .[].name":                        []byte("SECRET1\nSECRET2"),
			"variable list --repo myorg/myrepo --json name,value":                               []byte(`[{"name":"VAR1","value":"val1"},{"name":"VAR2","value":"val2"}]`),
		},
	}

	f := NewFetcher(mock)
	state, err := f.FetchRepository("myorg", "myrepo")
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
	if !state.Features.MergeCommit {
		t.Error("expected MergeCommit = true")
	}
	if !state.Features.SquashMerge {
		t.Error("expected SquashMerge = true")
	}
	if state.Features.RebaseMerge {
		t.Error("expected RebaseMerge = false")
	}
	if !state.Features.AutoDeleteHeadBranches {
		t.Error("expected AutoDeleteHeadBranches = true")
	}

	// Commit message settings
	if state.Features.SquashMergeCommitTitle != "PR_TITLE" {
		t.Errorf("SquashMergeCommitTitle = %q, want PR_TITLE", state.Features.SquashMergeCommitTitle)
	}
	if state.Features.SquashMergeCommitMessage != "COMMIT_MESSAGES" {
		t.Errorf("SquashMergeCommitMessage = %q, want COMMIT_MESSAGES", state.Features.SquashMergeCommitMessage)
	}
	if state.Features.MergeCommitTitle != "MERGE_MESSAGE" {
		t.Errorf("MergeCommitTitle = %q, want MERGE_MESSAGE", state.Features.MergeCommitTitle)
	}
	if state.Features.MergeCommitMessage != "PR_BODY" {
		t.Errorf("MergeCommitMessage = %q, want PR_BODY", state.Features.MergeCommitMessage)
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
			"repo view myorg/myrepo --json description,homepageUrl,visibility,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef": fmt.Errorf("not found"),
		},
	}

	f := NewFetcher(mock)
	_, err := f.FetchRepository("myorg", "myrepo")
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
		f := NewFetcher(mock)
		secrets, err := f.fetchSecrets("myorg", "myrepo")
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
		f := NewFetcher(mock)
		secrets, err := f.fetchSecrets("myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if secrets != nil {
			t.Errorf("expected nil secrets for empty response, got %v", secrets)
		}
	})

	t.Run("error returns nil", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"secret list --repo myorg/myrepo --json name --jq .[].name": fmt.Errorf("forbidden"),
			},
		}
		f := NewFetcher(mock)
		secrets, err := f.fetchSecrets("myorg", "myrepo")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if secrets != nil {
			t.Errorf("expected nil secrets on error, got %v", secrets)
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
		f := NewFetcher(mock)
		vars, err := f.fetchVariables("myorg", "myrepo")
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
		f := NewFetcher(mock)
		vars, err := f.fetchVariables("myorg", "myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vars) != 0 {
			t.Errorf("expected 0 variables, got %d", len(vars))
		}
	})

	t.Run("error returns nil", func(t *testing.T) {
		mock := &gh.MockRunner{
			Errors: map[string]error{
				"variable list --repo myorg/myrepo --json name,value": fmt.Errorf("forbidden"),
			},
		}
		f := NewFetcher(mock)
		vars, err := f.fetchVariables("myorg", "myrepo")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if vars != nil {
			t.Errorf("expected nil vars on error, got %v", vars)
		}
	})
}

func TestCurrentState_FullName(t *testing.T) {
	s := &CurrentState{Owner: "myorg", Name: "myrepo"}
	if got := s.FullName(); got != "myorg/myrepo" {
		t.Errorf("FullName() = %q, want myorg/myrepo", got)
	}
}
