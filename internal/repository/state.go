package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"golang.org/x/sync/errgroup"
)

// Fetcher retrieves current repository state from GitHub.
type Fetcher struct {
	runner gh.Runner
}

func NewFetcher(runner gh.Runner) *Fetcher {
	return &Fetcher{runner: runner}
}

// FetchRepository fetches the current state of a single repository.
// If the repository does not exist (404), it returns an empty CurrentState with IsNew=true.
// Sub-fetches (branch protection, secrets, variables) run in parallel.
func (f *Fetcher) FetchRepository(owner, name string) (*CurrentState, error) {
	repo, err := f.fetchRepoSettings(owner, name)
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return &CurrentState{Owner: owner, Name: name, IsNew: true}, nil
		}
		return nil, err
	}

	var (
		bp      map[string]*CurrentBranchProtection
		secrets []string
		vars    map[string]string
	)

	g := new(errgroup.Group)

	g.Go(func() error {
		var err error
		bp, err = f.fetchBranchProtection(owner, name)
		return err
	})

	g.Go(func() error {
		var err error
		secrets, err = f.fetchSecrets(owner, name)
		return err
	})

	g.Go(func() error {
		var err error
		vars, err = f.fetchVariables(owner, name)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	repo.BranchProtection = bp
	repo.Secrets = secrets
	repo.Variables = vars

	return repo, nil
}

func (f *Fetcher) fetchRepoSettings(owner, name string) (*CurrentState, error) {
	out, err := f.runner.Run(
		"repo", "view", owner+"/"+name,
		"--json", "description,homepageUrl,visibility,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef",
	)
	if err != nil {
		// gh repo view returns GraphQL error for non-existent repos, not REST 404
		if isRepoNotFound(err) {
			return nil, gh.ErrNotFound
		}
		return nil, fmt.Errorf("fetch repo %s/%s: %w", owner, name, err)
	}

	var raw struct {
		Description      string `json:"description"`
		HomepageURL      string `json:"homepageUrl"`
		Visibility       string `json:"visibility"`
		RepositoryTopics []struct {
			Name string `json:"name"`
		} `json:"repositoryTopics"`
		HasIssuesEnabled      bool `json:"hasIssuesEnabled"`
		HasProjectsEnabled    bool `json:"hasProjectsEnabled"`
		HasWikiEnabled        bool `json:"hasWikiEnabled"`
		HasDiscussionsEnabled bool `json:"hasDiscussionsEnabled"`
		MergeCommitAllowed    bool `json:"mergeCommitAllowed"`
		SquashMergeAllowed    bool `json:"squashMergeAllowed"`
		RebaseMergeAllowed    bool `json:"rebaseMergeAllowed"`
		DeleteBranchOnMerge   bool `json:"deleteBranchOnMerge"`
		DefaultBranchRef      struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse repo view for %s/%s: %w", owner, name, err)
	}

	var topics []string
	for _, t := range raw.RepositoryTopics {
		topics = append(topics, t.Name)
	}

	// Fetch commit message settings via REST API (not available in gh repo view --json)
	commitMsgSettings, _ := f.fetchCommitMessageSettings(owner, name)

	return &CurrentState{
		Owner:       owner,
		Name:        name,
		Description: raw.Description,
		Homepage:    raw.HomepageURL,
		Visibility:  strings.ToLower(raw.Visibility),
		Topics:      topics,
		Features: CurrentFeatures{
			Issues:                   raw.HasIssuesEnabled,
			Projects:                 raw.HasProjectsEnabled,
			Wiki:                     raw.HasWikiEnabled,
			Discussions:              raw.HasDiscussionsEnabled,
			MergeCommit:              raw.MergeCommitAllowed,
			SquashMerge:              raw.SquashMergeAllowed,
			RebaseMerge:              raw.RebaseMergeAllowed,
			AutoDeleteHeadBranches:   raw.DeleteBranchOnMerge,
			MergeCommitTitle:         commitMsgSettings.MergeCommitTitle,
			MergeCommitMessage:       commitMsgSettings.MergeCommitMessage,
			SquashMergeCommitTitle:   commitMsgSettings.SquashMergeCommitTitle,
			SquashMergeCommitMessage: commitMsgSettings.SquashMergeCommitMessage,
		},
	}, nil
}

type commitMessageSettings struct {
	MergeCommitTitle         string
	MergeCommitMessage       string
	SquashMergeCommitTitle   string
	SquashMergeCommitMessage string
}

func (f *Fetcher) fetchCommitMessageSettings(owner, name string) (commitMessageSettings, error) {
	out, err := f.runner.Run(
		"api", fmt.Sprintf("repos/%s/%s", owner, name),
		"--jq", "{squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}",
	)
	if err != nil {
		return commitMessageSettings{}, err
	}

	var raw struct {
		SquashMergeCommitTitle   string `json:"squash_merge_commit_title"`
		SquashMergeCommitMessage string `json:"squash_merge_commit_message"`
		MergeCommitTitle         string `json:"merge_commit_title"`
		MergeCommitMessage       string `json:"merge_commit_message"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return commitMessageSettings{}, err
	}

	return commitMessageSettings{
		MergeCommitTitle:         raw.MergeCommitTitle,
		MergeCommitMessage:       raw.MergeCommitMessage,
		SquashMergeCommitTitle:   raw.SquashMergeCommitTitle,
		SquashMergeCommitMessage: raw.SquashMergeCommitMessage,
	}, nil
}

func (f *Fetcher) fetchBranchProtection(owner, name string) (map[string]*CurrentBranchProtection, error) {
	// First get the default branch to check protection
	out, err := f.runner.Run(
		"api", fmt.Sprintf("repos/%s/%s/branches", owner, name),
		"--jq", `[.[] | select(.protected == true) | .name]`,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch branches for %s/%s: %w", owner, name, err)
	}

	var protectedBranches []string
	if err := json.Unmarshal(out, &protectedBranches); err != nil {
		return nil, nil // no protected branches or parse error
	}

	result := make(map[string]*CurrentBranchProtection)
	for _, branch := range protectedBranches {
		bp, err := f.fetchBranchProtectionRule(owner, name, branch)
		if err != nil {
			continue // skip branches we can't read
		}
		if bp != nil {
			result[branch] = bp
		}
	}
	return result, nil
}

func (f *Fetcher) fetchBranchProtectionRule(owner, name, branch string) (*CurrentBranchProtection, error) {
	out, err := f.runner.Run(
		"api", fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, name, branch),
	)
	if err != nil {
		return nil, err
	}

	var raw struct {
		RequiredPullRequestReviews *struct {
			RequiredApprovingReviewCount int  `json:"required_approving_review_count"`
			DismissStaleReviews          bool `json:"dismiss_stale_reviews"`
			RequireCodeOwnerReviews      bool `json:"require_code_owner_reviews"`
		} `json:"required_pull_request_reviews"`
		RequiredStatusChecks *struct {
			Strict   bool     `json:"strict"`
			Contexts []string `json:"contexts"`
		} `json:"required_status_checks"`
		EnforceAdmins *struct {
			Enabled bool `json:"enabled"`
		} `json:"enforce_admins"`
		AllowForcePushes *struct {
			Enabled bool `json:"enabled"`
		} `json:"allow_force_pushes"`
		AllowDeletions *struct {
			Enabled bool `json:"enabled"`
		} `json:"allow_deletions"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse branch protection for %s: %w", branch, err)
	}

	bp := &CurrentBranchProtection{
		Pattern: branch,
	}

	if raw.RequiredPullRequestReviews != nil {
		bp.RequiredReviews = raw.RequiredPullRequestReviews.RequiredApprovingReviewCount
		bp.DismissStaleReviews = raw.RequiredPullRequestReviews.DismissStaleReviews
		bp.RequireCodeOwnerReviews = raw.RequiredPullRequestReviews.RequireCodeOwnerReviews
	}
	if raw.RequiredStatusChecks != nil {
		bp.RequireStatusChecks = &CurrentStatusChecks{
			Strict:   raw.RequiredStatusChecks.Strict,
			Contexts: raw.RequiredStatusChecks.Contexts,
		}
	}
	if raw.EnforceAdmins != nil {
		bp.EnforceAdmins = raw.EnforceAdmins.Enabled
	}
	if raw.AllowForcePushes != nil {
		bp.AllowForcePushes = raw.AllowForcePushes.Enabled
	}
	if raw.AllowDeletions != nil {
		bp.AllowDeletions = raw.AllowDeletions.Enabled
	}

	return bp, nil
}

func (f *Fetcher) fetchSecrets(owner, name string) ([]string, error) {
	out, err := f.runner.Run(
		"secret", "list",
		"--repo", owner+"/"+name,
		"--json", "name",
		"--jq", ".[].name",
	)
	if err != nil {
		return nil, nil // secrets might not be accessible
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

func (f *Fetcher) fetchVariables(owner, name string) (map[string]string, error) {
	out, err := f.runner.Run(
		"variable", "list",
		"--repo", owner+"/"+name,
		"--json", "name,value",
	)
	if err != nil {
		return nil, nil // variables might not be accessible
	}

	var vars []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &vars); err != nil {
		return nil, nil
	}

	result := make(map[string]string)
	for _, v := range vars {
		result[v.Name] = v.Value
	}
	return result, nil
}

// isRepoNotFound checks if an error indicates the repository doesn't exist.
// gh repo view uses GraphQL which returns "Could not resolve to a Repository"
// instead of a REST 404.
func isRepoNotFound(err error) bool {
	if errors.Is(err, gh.ErrNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Could not resolve to a Repository")
}
