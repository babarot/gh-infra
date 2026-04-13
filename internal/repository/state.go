package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// Processor handles repository plan and apply operations.
type Processor struct {
	runner   gh.Runner
	resolver *manifest.Resolver
	diagnose DiagnosticReporter
}

func NewProcessor(runner gh.Runner, resolver *manifest.Resolver, diagnose DiagnosticReporter) *Processor {
	if diagnose == nil {
		diagnose = noopDiagnosticReporter{}
	}
	return &Processor{runner: runner, resolver: resolver, diagnose: diagnose}
}

// FetchRepository fetches the current state of a single repository.
// If the repository does not exist (404), it returns an empty CurrentState with IsNew=true.
// Sub-fetches (branch protection, secrets, variables) run in parallel.
// The optional onStatus callback is invoked with a human-readable label before each sub-fetch.
func (p *Processor) FetchRepository(ctx context.Context, owner, name string, onStatus func(string)) (*CurrentState, error) {
	status := func(s string) {
		if onStatus != nil {
			onStatus(s)
		}
	}

	status("fetching settings...")
	repo, err := p.fetchRepoSettings(ctx, owner, name)
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
		labels   map[string]*CurrentLabel
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		status("fetching branch protection...")
		var err error
		bp, err = p.fetchBranchProtection(ctx, owner, name)
		return err
	})

	g.Go(func() error {
		status("fetching rulesets...")
		var err error
		rulesets, err = p.fetchRulesets(ctx, owner, name)
		return err
	})

	g.Go(func() error {
		status("fetching secrets...")
		var err error
		secrets, err = p.fetchSecrets(ctx, owner, name)
		return err
	})

	g.Go(func() error {
		status("fetching variables...")
		var err error
		vars, err = p.fetchVariables(ctx, owner, name)
		return err
	})

	g.Go(func() error {
		status("fetching labels...")
		var err error
		labels, err = p.fetchLabels(ctx, owner, name)
		return err
	})

	var milestones map[string]*CurrentMilestone
	g.Go(func() error {
		status("fetching milestones...")
		var err error
		milestones, err = p.fetchMilestones(ctx, owner, name)
		return err
	})

	var actions CurrentActions
	g.Go(func() error {
		status("fetching actions...")
		var err error
		actions, err = p.fetchActionsSettings(ctx, owner, name)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	repo.BranchProtection = bp
	repo.Rulesets = rulesets
	repo.Secrets = secrets
	repo.Variables = vars
	repo.Labels = labels
	repo.Milestones = milestones
	repo.Actions = actions

	return repo, nil
}

func (p *Processor) fetchRepoSettings(ctx context.Context, owner, name string) (*CurrentState, error) {
	out, err := p.runner.Run(ctx,
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
	// 404/403 are ignored gracefully (e.g. GHES without support); other errors propagate.
	commitMsgSettings, err := p.fetchCommitMessageSettings(ctx, owner, name)
	if err != nil && !errors.Is(err, gh.ErrNotFound) && !errors.Is(err, gh.ErrForbidden) {
		return nil, fmt.Errorf("fetch commit message settings for %s/%s: %w", owner, name, err)
	}

	// Fetch release immutability setting via dedicated REST API endpoint
	// 404/403 are ignored gracefully (e.g. GHES without support); other errors propagate.
	releaseImmutability, err := p.fetchReleaseImmutability(ctx, owner, name)
	if err != nil && !errors.Is(err, gh.ErrNotFound) && !errors.Is(err, gh.ErrForbidden) {
		return nil, fmt.Errorf("fetch release immutability for %s/%s: %w", owner, name, err)
	}

	// Fetch vulnerability alerts (Dependabot) setting via dedicated REST API endpoint.
	// 404 is the documented "disabled" response and is handled inside the fetcher;
	// 403 is ignored gracefully (e.g. GHES without support); other errors propagate.
	vulnerabilityAlerts, err := p.fetchVulnerabilityAlerts(ctx, owner, name)
	if err != nil && !errors.Is(err, gh.ErrForbidden) {
		return nil, fmt.Errorf("fetch vulnerability alerts for %s/%s: %w", owner, name, err)
	}

	// Fetch Dependabot security updates (automated security fixes) via dedicated endpoint.
	// 404 may also indicate the feature is unavailable on the repo; treat as disabled.
	automatedSecurityFixes, err := p.fetchAutomatedSecurityFixes(ctx, owner, name)
	if err != nil && !errors.Is(err, gh.ErrForbidden) {
		return nil, fmt.Errorf("fetch automated security fixes for %s/%s: %w", owner, name, err)
	}

	// Fetch private vulnerability reporting setting via dedicated endpoint.
	privateVulnerabilityReporting, err := p.fetchPrivateVulnerabilityReporting(ctx, owner, name)
	if err != nil && !errors.Is(err, gh.ErrForbidden) {
		return nil, fmt.Errorf("fetch private vulnerability reporting for %s/%s: %w", owner, name, err)
	}

	return &CurrentState{
		Owner:               owner,
		Name:                name,
		Description:         raw.Description,
		Archived:            raw.IsArchived,
		Homepage:            raw.HomepageURL,
		Visibility:          strings.ToLower(raw.Visibility),
		Topics:                        topics,
		ReleaseImmutability:           releaseImmutability,
		VulnerabilityAlerts:           vulnerabilityAlerts,
		AutomatedSecurityFixes:        automatedSecurityFixes,
		PrivateVulnerabilityReporting: privateVulnerabilityReporting,
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

func (p *Processor) fetchCommitMessageSettings(ctx context.Context, owner, name string) (commitMessageSettings, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s", owner, name),
		"--jq", "{squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}",
	)
	if err != nil {
		return commitMessageSettings{}, err
	}

	var raw struct {
		SquashMergeCommitTitle   *string `json:"squash_merge_commit_title"`
		SquashMergeCommitMessage *string `json:"squash_merge_commit_message"`
		MergeCommitTitle         *string `json:"merge_commit_title"`
		MergeCommitMessage       *string `json:"merge_commit_message"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return commitMessageSettings{}, err
	}

	deref := func(s *string, def string) string {
		if s != nil && *s != "" {
			return *s
		}
		return def
	}

	return commitMessageSettings{
		MergeCommitTitle:         deref(raw.MergeCommitTitle, "MERGE_MESSAGE"),
		MergeCommitMessage:       deref(raw.MergeCommitMessage, "PR_TITLE"),
		SquashMergeCommitTitle:   deref(raw.SquashMergeCommitTitle, "COMMIT_OR_PR_TITLE"),
		SquashMergeCommitMessage: deref(raw.SquashMergeCommitMessage, "COMMIT_MESSAGES"),
	}, nil
}

func (p *Processor) fetchReleaseImmutability(ctx context.Context, owner, name string) (bool, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/immutable-releases", owner, name),
	)
	if err != nil {
		return false, err
	}

	var raw struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return false, err
	}
	return raw.Enabled, nil
}

// fetchVulnerabilityAlerts returns whether Dependabot vulnerability alerts are
// enabled on the repository. The GitHub API signals state via HTTP status:
// 204 No Content when enabled, 404 Not Found when disabled.
func (p *Processor) fetchVulnerabilityAlerts(ctx context.Context, owner, name string) (bool, error) {
	_, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name),
	)
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// fetchAutomatedSecurityFixes returns whether Dependabot security updates
// (automated security fixes) are enabled on the repository.
// The endpoint may return 404 when the feature is unavailable on the repo
// (e.g. vulnerability alerts disabled); treat that as disabled.
func (p *Processor) fetchAutomatedSecurityFixes(ctx context.Context, owner, name string) (bool, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name),
	)
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var raw struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return false, err
	}
	return raw.Enabled, nil
}

// fetchPrivateVulnerabilityReporting returns whether private vulnerability
// reporting is enabled on the repository.
func (p *Processor) fetchPrivateVulnerabilityReporting(ctx context.Context, owner, name string) (bool, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name),
	)
	if err != nil {
		if errors.Is(err, gh.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var raw struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return false, err
	}
	return raw.Enabled, nil
}

func (p *Processor) fetchBranchProtection(ctx context.Context, owner, name string) (map[string]*CurrentBranchProtection, error) {
	// First get the default branch to check protection
	out, err := p.runner.Run(ctx,
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
		bp, err := p.fetchBranchProtectionRule(ctx, owner, name, branch)
		if err != nil {
			continue // skip branches we can't read
		}
		if bp != nil {
			result[branch] = bp
		}
	}
	return result, nil
}

func (p *Processor) fetchBranchProtectionRule(ctx context.Context, owner, name, branch string) (*CurrentBranchProtection, error) {
	out, err := p.runner.Run(ctx,
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

func (p *Processor) fetchRulesets(ctx context.Context, owner, name string) (map[string]*CurrentRuleset, error) {
	out, err := p.runner.Run(ctx,
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
		rs, err := p.fetchRuleset(ctx, owner, name, item.ID)
		if err != nil {
			continue // skip inaccessible individual rulesets
		}
		result[rs.Name] = rs
	}
	return result, nil
}

func (p *Processor) fetchRuleset(ctx context.Context, owner, name string, id int) (*CurrentRuleset, error) {
	out, err := p.runner.Run(ctx,
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

func (p *Processor) fetchSecrets(ctx context.Context, owner, name string) ([]string, error) {
	out, err := p.runner.Run(ctx,
		"secret", "list",
		"--repo", owner+"/"+name,
		"--json", "name",
		"--jq", ".[].name",
	)
	if err != nil {
		if isIgnorableSubresourceError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch secrets for %s/%s: %w", owner, name, err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	names := strings.Split(raw, "\n")
	sort.Strings(names)
	return names, nil
}

func (p *Processor) fetchVariables(ctx context.Context, owner, name string) (map[string]string, error) {
	out, err := p.runner.Run(ctx,
		"variable", "list",
		"--repo", owner+"/"+name,
		"--json", "name,value",
	)
	if err != nil {
		if isIgnorableSubresourceError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch variables for %s/%s: %w", owner, name, err)
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

func (p *Processor) fetchLabels(ctx context.Context, owner, name string) (map[string]*CurrentLabel, error) {
	out, err := p.runner.Run(ctx,
		"label", "list",
		"--repo", owner+"/"+name,
		"--json", "name,color,description",
		"--limit", "1000",
	)
	if err != nil {
		if isIgnorableSubresourceError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch labels for %s/%s: %w", owner, name, err)
	}

	var labels []struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(out, &labels); err != nil {
		return nil, nil
	}

	result := make(map[string]*CurrentLabel)
	for _, l := range labels {
		result[l.Name] = &CurrentLabel{
			Name:        l.Name,
			Description: l.Description,
			Color:       l.Color,
		}
	}
	return result, nil
}

func (p *Processor) fetchMilestones(ctx context.Context, owner, name string) (map[string]*CurrentMilestone, error) {
	out, err := p.runner.Run(ctx,
		"api",
		fmt.Sprintf("repos/%s/%s/milestones?state=all&per_page=100", owner, name),
		"--paginate",
	)
	if err != nil {
		if isIgnorableSubresourceError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch milestones for %s/%s: %w", owner, name, err)
	}

	var milestones []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		DueOn       string `json:"due_on"`
	}
	if err := json.Unmarshal(out, &milestones); err != nil {
		return nil, nil
	}

	result := make(map[string]*CurrentMilestone)
	for _, m := range milestones {
		result[m.Title] = &CurrentMilestone{
			Number:      m.Number,
			Title:       m.Title,
			Description: m.Description,
			State:       m.State,
			DueOn:       normalizeDueOn(m.DueOn),
		}
	}
	return result, nil
}

// normalizeDueOn converts an ISO 8601 timestamp (e.g. "2026-06-01T00:00:00Z")
// to a YYYY-MM-DD date string for stable comparison with manifest values.
func normalizeDueOn(raw string) string {
	if raw == "" || raw == "null" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		// Already in YYYY-MM-DD or unparseable — return as-is
		return raw
	}
	return t.Format("2006-01-02")
}

// enrichLabelDeleteInfo fetches usage stats for label delete changes and
// embeds them in the OldValue for plan display.
func (p *Processor) enrichLabelDeleteInfo(ctx context.Context, changes []Change) {
	for i := range changes {
		c := &changes[i]
		if c.Resource != manifest.ResourceLabel || c.Type != ChangeDelete {
			continue
		}
		// c.Name is "owner/repo"
		parts := strings.SplitN(c.Name, "/", 2)
		if len(parts) != 2 {
			continue
		}
		usage, err := p.fetchLabelUsage(ctx, parts[0], parts[1], c.Field)
		if err != nil {
			continue // non-fatal: show without usage stats
		}
		old := fmt.Sprintf("%v", c.OldValue)
		if usage.Count == 0 {
			old += " (0 issues/PRs)"
		} else {
			age := formatTimeAgo(usage.LastUsed)
			old += fmt.Sprintf(" (%d issues/PRs, last used %s)", usage.Count, age)
		}
		c.OldValue = old
	}
}

// fetchLabelUsage returns the number of issues/PRs with a label and
// when it was last used, via the GitHub Search API.
func (p *Processor) fetchLabelUsage(ctx context.Context, owner, repo, labelName string) (LabelUsage, error) {
	q := fmt.Sprintf("label:\"%s\" repo:%s/%s sort:updated-desc", labelName, owner, repo)
	out, err := p.runner.Run(ctx,
		"api", "/search/issues",
		"-X", "GET",
		"-f", "q="+q,
		"-f", "per_page=1",
		"--jq", `"\(.total_count) \(.items[0].updated_at // "")"`,
	)
	if err != nil {
		return LabelUsage{}, err
	}

	raw := strings.TrimSpace(string(out))
	var count int
	var dateStr string
	if _, err := fmt.Sscanf(raw, "%d %s", &count, &dateStr); err != nil {
		return LabelUsage{}, err
	}

	var lastUsed time.Time
	if dateStr != "" {
		lastUsed, _ = time.Parse(time.RFC3339, dateStr)
	}
	return LabelUsage{Count: count, LastUsed: lastUsed}, nil
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (p *Processor) fetchActionsSettings(ctx context.Context, owner, name string) (CurrentActions, error) {
	var result CurrentActions
	fullName := fmt.Sprintf("repos/%s/%s", owner, name)

	// 1. Actions permissions (enabled + allowed_actions)
	out, err := p.runner.Run(ctx, "api", fullName+"/actions/permissions")
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
	out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/workflow")
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
		out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/selected-actions")
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
	out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/fork-pr-contributor-approval")
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

// isIgnorableSubresourceError reports whether a sub-resource fetch failure can
// be ignored without aborting repository state collection. This is limited to
// optional sub-resources (secrets, variables, labels, milestones) that may be
// inaccessible due to permissions, plan restrictions, or non-existence. All
// other errors (network, timeout, 5xx) are propagated.
func isIgnorableSubresourceError(err error) bool {
	return errors.Is(err, gh.ErrNotFound) ||
		errors.Is(err, gh.ErrForbidden) ||
		errors.Is(err, gh.ErrValidation)
}
