package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

// Processor handles repository plan and apply operations.
type Processor struct {
	runner   gh.Runner
	resolver *manifest.Resolver
	printer  ui.Printer
}

func NewProcessor(runner gh.Runner, resolver *manifest.Resolver, printer ui.Printer) *Processor {
	return &Processor{runner: runner, resolver: resolver, printer: printer}
}

// FetchRepository fetches the current state of a single repository.
// If the repository does not exist (404), it returns an empty CurrentState with IsNew=true.
// Sub-fetches (branch protection, secrets, variables) run in parallel.
func (f *Processor) FetchRepository(owner, name string) (*CurrentState, error) {
	repo, err := f.fetchRepoSettings(owner, name)
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return &CurrentState{Owner: owner, Name: name, IsNew: true}, nil
		}
		return nil, err
	}

	var (
		bp       map[string]*CurrentBranchProtection
		rulesets map[string]*CurrentRuleset
		secrets  []string
		vars     map[string]string
	)

	g := new(errgroup.Group)

	g.Go(func() error {
		var err error
		bp, err = f.fetchBranchProtection(owner, name)
		return err
	})

	g.Go(func() error {
		var err error
		rulesets, err = f.fetchRulesets(owner, name)
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

	var actions CurrentActions
	g.Go(func() error {
		var err error
		actions, err = f.fetchActionsSettings(owner, name)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	repo.BranchProtection = bp
	repo.Rulesets = rulesets
	repo.Secrets = secrets
	repo.Variables = vars
	repo.Actions = actions

	return repo, nil
}

func (f *Processor) fetchRepoSettings(owner, name string) (*CurrentState, error) {
	out, err := f.runner.Run(
		"repo", "view", owner+"/"+name,
		"--json", "description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef",
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
		IsArchived            bool `json:"isArchived"`
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
		Archived:    raw.IsArchived,
		Homepage:    raw.HomepageURL,
		Visibility:  strings.ToLower(raw.Visibility),
		Topics:      topics,
		Features: CurrentFeatures{
			Issues:      raw.HasIssuesEnabled,
			Projects:    raw.HasProjectsEnabled,
			Wiki:        raw.HasWikiEnabled,
			Discussions: raw.HasDiscussionsEnabled,
		},
		MergeStrategy: CurrentMergeStrategy{
			AllowMergeCommit:         raw.MergeCommitAllowed,
			AllowSquashMerge:         raw.SquashMergeAllowed,
			AllowRebaseMerge:         raw.RebaseMergeAllowed,
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

func (f *Processor) fetchCommitMessageSettings(owner, name string) (commitMessageSettings, error) {
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

func (f *Processor) fetchBranchProtection(owner, name string) (map[string]*CurrentBranchProtection, error) {
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

func (f *Processor) fetchBranchProtectionRule(owner, name, branch string) (*CurrentBranchProtection, error) {
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

func (f *Processor) fetchRulesets(owner, name string) (map[string]*CurrentRuleset, error) {
	out, err := f.runner.Run(
		"api", fmt.Sprintf("repos/%s/%s/rulesets", owner, name),
		"--paginate",
	)
	if err != nil {
		// 404/403 means rulesets not available (e.g., free plan private repo, GHES without rulesets)
		if errors.Is(err, gh.ErrNotFound) || errors.Is(err, gh.ErrForbidden) {
			return make(map[string]*CurrentRuleset), nil
		}
		// All other errors (429, 5xx) propagate to prevent false diffs
		return nil, fmt.Errorf("fetch rulesets for %s/%s: %w", owner, name, err)
	}

	var rawList []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
	}
	if err := json.Unmarshal(out, &rawList); err != nil {
		return make(map[string]*CurrentRuleset), nil
	}

	result := make(map[string]*CurrentRuleset)
	for _, item := range rawList {
		// Skip org-level rulesets (not manageable at repo level)
		if item.SourceType == "Organization" || item.SourceType == "Enterprise" {
			continue
		}
		rs, err := f.fetchRuleset(owner, name, item.ID)
		if err != nil {
			continue // skip inaccessible individual rulesets
		}
		result[rs.Name] = rs
	}
	return result, nil
}

func (f *Processor) fetchRuleset(owner, name string, id int) (*CurrentRuleset, error) {
	out, err := f.runner.Run(
		"api", fmt.Sprintf("repos/%s/%s/rulesets/%d", owner, name, id),
	)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Target       string `json:"target"`
		Enforcement  string `json:"enforcement"`
		BypassActors []struct {
			ActorID    int    `json:"actor_id"`
			ActorType  string `json:"actor_type"`
			BypassMode string `json:"bypass_mode"`
		} `json:"bypass_actors"`
		Conditions struct {
			RefName *struct {
				Include []string `json:"include"`
				Exclude []string `json:"exclude"`
			} `json:"ref_name"`
		} `json:"conditions"`
		Rules []json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ruleset %d for %s/%s: %w", id, owner, name, err)
	}

	rs := &CurrentRuleset{
		ID:          raw.ID,
		Name:        raw.Name,
		Target:      raw.Target,
		Enforcement: raw.Enforcement,
	}

	for _, ba := range raw.BypassActors {
		rs.BypassActors = append(rs.BypassActors, CurrentRulesetBypassActor{
			ActorID:    ba.ActorID,
			ActorType:  ba.ActorType,
			BypassMode: ba.BypassMode,
		})
	}

	if raw.Conditions.RefName != nil {
		rs.Conditions = &CurrentRulesetConditions{
			RefName: &CurrentRulesetRefCondition{
				Include: raw.Conditions.RefName.Include,
				Exclude: raw.Conditions.RefName.Exclude,
			},
		}
	}

	// Parse rules array
	type ruleEnvelope struct {
		Type       string          `json:"type"`
		Parameters json.RawMessage `json:"parameters"`
	}
	for _, rawRule := range raw.Rules {
		var env ruleEnvelope
		if err := json.Unmarshal(rawRule, &env); err != nil {
			continue
		}
		switch env.Type {
		case "pull_request":
			var params struct {
				RequiredApprovingReviewCount   int  `json:"required_approving_review_count"`
				DismissStaleReviewsOnPush      bool `json:"dismiss_stale_reviews_on_push"`
				RequireCodeOwnerReview         bool `json:"require_code_owner_review"`
				RequireLastPushApproval        bool `json:"require_last_push_approval"`
				RequiredReviewThreadResolution bool `json:"required_review_thread_resolution"`
			}
			if err := json.Unmarshal(env.Parameters, &params); err == nil {
				rs.Rules.PullRequest = &CurrentRulesetPullRequest{
					RequiredApprovingReviewCount:   params.RequiredApprovingReviewCount,
					DismissStaleReviewsOnPush:      params.DismissStaleReviewsOnPush,
					RequireCodeOwnerReview:         params.RequireCodeOwnerReview,
					RequireLastPushApproval:        params.RequireLastPushApproval,
					RequiredReviewThreadResolution: params.RequiredReviewThreadResolution,
				}
			}
		case "required_status_checks":
			var params struct {
				StrictRequiredStatusChecksPolicy bool `json:"strict_required_status_checks_policy"`
				RequiredStatusChecks             []struct {
					Context       string `json:"context"`
					IntegrationID int    `json:"integration_id"`
				} `json:"required_status_checks"`
			}
			if err := json.Unmarshal(env.Parameters, &params); err == nil {
				sc := &CurrentRulesetStatusChecks{
					StrictRequiredStatusChecksPolicy: params.StrictRequiredStatusChecksPolicy,
				}
				for _, c := range params.RequiredStatusChecks {
					sc.Contexts = append(sc.Contexts, CurrentRulesetStatusCheck{
						Context:       c.Context,
						IntegrationID: c.IntegrationID,
					})
				}
				rs.Rules.RequiredStatusChecks = sc
			}
		case "non_fast_forward":
			rs.Rules.NonFastForward = true
		case "deletion":
			rs.Rules.Deletion = true
		case "creation":
			rs.Rules.Creation = true
		case "required_linear_history":
			rs.Rules.RequiredLinearHistory = true
		case "required_signatures":
			rs.Rules.RequiredSignatures = true
		}
	}

	return rs, nil
}

func (f *Processor) fetchSecrets(owner, name string) ([]string, error) {
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

func (f *Processor) fetchVariables(owner, name string) (map[string]string, error) {
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

func (f *Processor) fetchActionsSettings(owner, name string) (CurrentActions, error) {
	var result CurrentActions
	fullName := fmt.Sprintf("repos/%s/%s", owner, name)

	// 1. Actions permissions (enabled + allowed_actions)
	out, err := f.runner.Run("api", fullName+"/actions/permissions")
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return result, nil // Actions API not available for this repo
		}
		return result, fmt.Errorf("fetch actions permissions for %s/%s: %w", owner, name, err)
	}
	var perms struct {
		Enabled            bool   `json:"enabled"`
		AllowedActions     string `json:"allowed_actions"`
		SHAPinningRequired bool   `json:"sha_pinning_required"`
	}
	if err := json.Unmarshal(out, &perms); err != nil {
		return result, nil
	}
	result.Enabled = perms.Enabled
	result.AllowedActions = perms.AllowedActions
	result.SHAPinningRequired = perms.SHAPinningRequired

	// 2. Workflow permissions (GITHUB_TOKEN defaults)
	out, err = f.runner.Run("api", fullName+"/actions/permissions/workflow")
	if err != nil && !errors.Is(err, gh.ErrNotFound) {
		return result, fmt.Errorf("fetch actions workflow permissions for %s/%s: %w", owner, name, err)
	}
	if err == nil {
		var wf struct {
			DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
			CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
		}
		if json.Unmarshal(out, &wf) == nil {
			result.WorkflowPermissions = wf.DefaultWorkflowPermissions
			result.CanApprovePullRequests = wf.CanApprovePullRequestReviews
		}
	}

	// 3. Selected actions (only when allowed_actions == "selected")
	if result.AllowedActions == "selected" {
		out, err = f.runner.Run("api", fullName+"/actions/permissions/selected-actions")
		if err != nil && !errors.Is(err, gh.ErrNotFound) {
			return result, fmt.Errorf("fetch actions selected-actions for %s/%s: %w", owner, name, err)
		}
		if err == nil {
			var sa struct {
				GithubOwnedAllowed bool     `json:"github_owned_allowed"`
				VerifiedAllowed    bool     `json:"verified_allowed"`
				PatternsAllowed    []string `json:"patterns_allowed"`
			}
			if json.Unmarshal(out, &sa) == nil {
				result.SelectedActions = &CurrentSelectedActions{
					GithubOwnedAllowed: sa.GithubOwnedAllowed,
					VerifiedAllowed:    sa.VerifiedAllowed,
					PatternsAllowed:    sa.PatternsAllowed,
				}
			}
		}
	}

	// 4. Fork PR approval (may 404 on user-owned repos or 422 on private repos — ignore gracefully)
	out, err = f.runner.Run("api", fullName+"/actions/permissions/fork-pr-contributor-approval")
	if err != nil && !errors.Is(err, gh.ErrNotFound) && !errors.Is(err, gh.ErrValidation) {
		return result, fmt.Errorf("fetch actions fork-pr-approval for %s/%s: %w", owner, name, err)
	}
	if err == nil {
		var fpr struct {
			ApprovalPolicy string `json:"approval_policy"`
		}
		if json.Unmarshal(out, &fpr) == nil {
			result.ForkPRApproval = fpr.ApprovalPolicy
		}
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
