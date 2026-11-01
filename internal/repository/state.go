package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
)

// Processor handles repository plan and apply operations.
type Processor struct {
	runner   gh.Runner
	resolver *manifest.Resolver
	plans    *planCache

	// Replaceable in tests.
	sleep func(ctx context.Context, d time.Duration) error
}

func NewProcessor(runner gh.Runner, resolver *manifest.Resolver) *Processor {
	return &Processor{runner: runner, resolver: resolver, sleep: sleepContext, plans: &planCache{plans: map[string]string{}}}
}

// FetchRepository fetches the current state of a single repository.
// If the repository does not exist (404), it returns an empty CurrentState with IsNew=true.
// Sub-fetches (branch protection, secrets, variables) run in parallel.
// The optional onStatus callback is invoked with a human-readable label before each sub-fetch.
//
// With a desired repository (plan and apply), only the settings it manages
// are fetched, so the token needs only their permissions, and one of them
// that cannot be read is an error rather than "none" or a default. A
// collection declared empty that is not authoritative is not fetched: it
// plans nothing, so what GitHub has does not matter. With a nil desired
// repository (import), everything is fetched and what cannot be read is left
// empty or at its default.
func (p *Processor) FetchRepository(ctx context.Context, owner, name string, desired *manifest.Repository, onStatus func(string)) (*CurrentState, error) {
	status := func(s string) {
		if onStatus != nil {
			onStatus(s)
		}
	}
	var spec *manifest.RepositorySpec
	var reconcile *manifest.RepositoryReconcile
	if desired != nil {
		spec, reconcile = &desired.Spec, desired.Reconcile
	}
	strict := spec != nil
	// record keeps what import could not read in Unread and leaves the
	// setting unset; any other error is returned.
	var (
		unreadMu sync.Mutex
		unread   map[string]error
	)
	record := func(key string, err error) error {
		var u unreadError
		if !errors.As(err, &u) {
			return err
		}
		unreadMu.Lock()
		defer unreadMu.Unlock()
		if unread == nil {
			unread = make(map[string]error)
		}
		unread[key] = u.err
		return nil
	}
	want := func(manages func(manifest.RepositorySpec) bool) bool {
		return spec == nil || manages(*spec)
	}
	// wantCollection is want, with a spec, for a collection that deletes
	// only when authoritative: declared empty and additive, it plans nothing.
	wantCollection := func(manages func(manifest.RepositorySpec) bool, declared int, mode string) bool {
		return manages(*spec) && (declared > 0 || mode == manifest.CollectionReconcileAuthoritative)
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

	if spec == nil || wantCollection(manifest.RepositorySpec.ManagesBranchProtection, len(spec.BranchProtection), manifest.BranchProtectionReconcileMode(reconcile)) {
		g.Go(func() error {
			status("fetching branch protection...")
			var err error
			bp, err = p.fetchBranchProtection(ctx, owner, name, strict)
			return record("branch_protection", err)
		})
	}

	if spec == nil || wantCollection(manifest.RepositorySpec.ManagesRulesets, len(spec.Rulesets), manifest.RulesetsReconcileMode(reconcile)) {
		g.Go(func() error {
			status("fetching rulesets...")
			var err error
			rulesets, err = p.fetchRulesets(ctx, owner, name, strict)
			return record("rulesets", err)
		})
	}

	if want(manifest.RepositorySpec.ManagesSecrets) {
		g.Go(func() error {
			status("fetching secrets...")
			var err error
			secrets, err = p.fetchSecrets(ctx, owner, name, strict)
			return err
		})
	}

	if want(manifest.RepositorySpec.ManagesVariables) {
		g.Go(func() error {
			status("fetching variables...")
			var err error
			vars, err = p.fetchVariables(ctx, owner, name, strict)
			return record("variables", err)
		})
	}

	if spec == nil || wantCollection(manifest.RepositorySpec.ManagesLabels, len(spec.Labels), manifest.LabelsReconcileMode(reconcile, spec.LabelSync)) {
		g.Go(func() error {
			status("fetching labels...")
			var err error
			labels, err = p.fetchLabels(ctx, owner, name, strict)
			return record("labels", err)
		})
	}

	var milestones map[string]*CurrentMilestone
	if want(manifest.RepositorySpec.ManagesMilestones) {
		g.Go(func() error {
			status("fetching milestones...")
			var err error
			milestones, err = p.fetchMilestones(ctx, owner, name, strict)
			return record("milestones", err)
		})
	}

	var environments map[string]*CurrentEnvironment
	if want(manifest.RepositorySpec.ManagesEnvironments) {
		g.Go(func() error {
			status("fetching environments...")
			var err error
			environments, err = p.fetchEnvironments(ctx, owner, name, spec, strict, record)
			return record("environments", err)
		})
	}

	var actions CurrentActions
	if want(manifest.RepositorySpec.ManagesActions) {
		g.Go(func() error {
			status("fetching actions...")
			var err error
			actions, err = p.fetchActionsSettings(ctx, owner, name, spec)
			if err != nil && strict && errors.Is(err, gh.ErrForbidden) {
				return fetchError(strict, "actions settings", owner, name, err)
			}
			return record("actions", err)
		})
	}

	var (
		// Initialize with safe defaults so that 403/404 fallback does not
		// produce a destructive zero-value (e.g. has_pull_requests=false).
		commitMsgSettings = commitMessageSettings{
			HasPullRequests:     true,
			PullRequestCreation: manifest.PullRequestCreationAll,
		}
		releaseImmutability           bool
		vulnerabilityAlerts           bool
		automatedSecurityFixes        bool
		privateVulnerabilityReporting bool
	)

	// fetchSetting runs a single-value fetch of what, which the manifest
	// keys name. With a spec, a setting that cannot be read is an error (a
	// 404 too, e.g. GHES without the feature: apply could not set it).
	// Import leaves one that cannot be read (403, 404, 422, or a response
	// that does not parse) unread rather than at its default. Other errors
	// (5xx) propagate.
	fetchSetting := func(what string, keys []string, fetch func() error) func() error {
		return func() error {
			status("fetching " + what + "...")
			err := fetch()
			if err == nil {
				return nil
			}
			if !strict && (isIgnorableSubresourceError(err) || isParseError(err)) {
				for _, key := range keys {
					_ = record(key, unreadSetting(err))
				}
				return nil
			}
			return fetchError(strict, what, owner, name, err)
		}
	}

	if want(manifest.RepositorySpec.ManagesCommitMessageSettings) {
		// Not available in gh repo view --json.
		g.Go(fetchSetting("commit message settings", []string{
			"features.pull_requests",
			"merge_strategy.allow_auto_merge",
			"merge_strategy.merge_commit_message",
			"merge_strategy.merge_commit_title",
			"merge_strategy.squash_merge_commit_message",
			"merge_strategy.squash_merge_commit_title",
		}, func() error {
			s, err := p.fetchCommitMessageSettings(ctx, owner, name)
			if err == nil {
				commitMsgSettings = s
			}
			return err
		}))
	}

	if want(manifest.RepositorySpec.ManagesReleaseImmutability) {
		g.Go(fetchSetting("release immutability", []string{"release_immutability"}, func() error {
			v, err := p.fetchReleaseImmutability(ctx, owner, name)
			releaseImmutability = v
			return err
		}))
	}

	// For the security settings, 404 is handled inside the fetchers (it is
	// the documented "disabled" response).
	if want(manifest.RepositorySpec.ManagesVulnerabilityAlerts) {
		g.Go(fetchSetting("vulnerability alerts", []string{"security.vulnerability_alerts"}, func() error {
			v, err := p.fetchVulnerabilityAlerts(ctx, owner, name)
			vulnerabilityAlerts = v
			return err
		}))
	}

	if want(manifest.RepositorySpec.ManagesAutomatedSecurityFixes) {
		g.Go(fetchSetting("automated security fixes", []string{"security.automated_security_fixes"}, func() error {
			v, err := p.fetchAutomatedSecurityFixes(ctx, owner, name)
			automatedSecurityFixes = v
			return err
		}))
	}

	if want(manifest.RepositorySpec.ManagesPrivateVulnerabilityReporting) {
		g.Go(fetchSetting("private vulnerability reporting", []string{"security.private_vulnerability_reporting"}, func() error {
			v, err := p.fetchPrivateVulnerabilityReporting(ctx, owner, name)
			privateVulnerabilityReporting = v
			return err
		}))
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	repo.BranchProtection = bp
	repo.Rulesets = rulesets
	repo.Secrets = secrets
	repo.Variables = vars
	repo.Labels = labels
	repo.Milestones = milestones
	repo.Environments = environments
	repo.Actions = actions
	repo.Unread = unread
	repo.ReleaseImmutability = releaseImmutability
	repo.MergeStrategy.MergeCommitTitle = commitMsgSettings.MergeCommitTitle
	repo.MergeStrategy.MergeCommitMessage = commitMsgSettings.MergeCommitMessage
	repo.MergeStrategy.SquashMergeCommitTitle = commitMsgSettings.SquashMergeCommitTitle
	repo.MergeStrategy.SquashMergeCommitMessage = commitMsgSettings.SquashMergeCommitMessage
	repo.MergeStrategy.AllowAutoMerge = commitMsgSettings.AllowAutoMerge
	repo.Features.PullRequests = commitMsgSettings.HasPullRequests
	repo.Features.PullRequestCreation = commitMsgSettings.PullRequestCreation
	repo.Security = CurrentSecurity{
		VulnerabilityAlerts:           vulnerabilityAlerts,
		AutomatedSecurityFixes:        automatedSecurityFixes,
		PrivateVulnerabilityReporting: privateVulnerabilityReporting,
	}

	return repo, nil
}

func (p *Processor) fetchRepoSettings(ctx context.Context, owner, name string) (*CurrentState, error) {
	out, err := p.runner.Run(ctx,
		"repo", "view", owner+"/"+name,
		"--json", "description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef",
	)
	if err != nil {
		// gh repo view returns GraphQL error for non-existent repos, not REST 404
		if gh.IsRepoNotFound(err) {
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
			AllowMergeCommit:       raw.MergeCommitAllowed,
			AllowSquashMerge:       raw.SquashMergeAllowed,
			AllowRebaseMerge:       raw.RebaseMergeAllowed,
			AutoDeleteHeadBranches: raw.DeleteBranchOnMerge,
			// Commit message string fields are populated later via
			// fetchCommitMessageSettings in the FetchRepository errgroup.
		},
	}, nil
}

type commitMessageSettings struct {
	MergeCommitTitle         string
	MergeCommitMessage       string
	SquashMergeCommitTitle   string
	SquashMergeCommitMessage string
	AllowAutoMerge           bool
	HasPullRequests          bool
	PullRequestCreation      string
}

func (p *Processor) fetchCommitMessageSettings(ctx context.Context, owner, name string) (commitMessageSettings, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s", owner, name),
		"--jq", "{squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}",
	)
	if err != nil {
		return commitMessageSettings{}, err
	}

	var raw struct {
		SquashMergeCommitTitle    *string `json:"squash_merge_commit_title"`
		SquashMergeCommitMessage  *string `json:"squash_merge_commit_message"`
		MergeCommitTitle          *string `json:"merge_commit_title"`
		MergeCommitMessage        *string `json:"merge_commit_message"`
		AllowAutoMerge            bool    `json:"allow_auto_merge"`
		HasPullRequests           *bool   `json:"has_pull_requests"`
		PullRequestCreationPolicy *string `json:"pull_request_creation_policy"`
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

	hasPR := true
	if raw.HasPullRequests != nil {
		hasPR = *raw.HasPullRequests
	}
	prCreation := manifest.PullRequestCreationAll
	if raw.PullRequestCreationPolicy != nil && *raw.PullRequestCreationPolicy != "" {
		prCreation = *raw.PullRequestCreationPolicy
	}

	return commitMessageSettings{
		MergeCommitTitle:         deref(raw.MergeCommitTitle, "MERGE_MESSAGE"),
		MergeCommitMessage:       deref(raw.MergeCommitMessage, "PR_TITLE"),
		SquashMergeCommitTitle:   deref(raw.SquashMergeCommitTitle, "COMMIT_OR_PR_TITLE"),
		SquashMergeCommitMessage: deref(raw.SquashMergeCommitMessage, "COMMIT_MESSAGES"),
		AllowAutoMerge:           raw.AllowAutoMerge,
		HasPullRequests:          hasPR,
		PullRequestCreation:      prCreation,
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

func (p *Processor) fetchBranchProtection(ctx context.Context, owner, name string, strict bool) (map[string]*CurrentBranchProtection, error) {
	// First get the default branch to check protection
	// Paginate: a protected branch missed here would look unprotected and be
	// planned as a create.
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/branches?protected=true&per_page=100", owner, name),
		"--paginate", "--jq", ".[].name",
	)
	if err != nil {
		if !strict && isIgnorableSubresourceError(err) {
			return nil, unreadSetting(err)
		}
		return nil, fmt.Errorf("fetch branches for %s/%s: %w", owner, name, err)
	}

	var protectedBranches []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			protectedBranches = append(protectedBranches, line)
		}
	}

	// Fetch per-branch protection rules in parallel. Only a rule that is
	// gone (404) is skipped: skipping one that could not be read would plan
	// a create over it, or have import --into remove it from the manifest,
	// so for import the whole branch protection is left unread instead.
	type fetchedRule struct {
		bp  *CurrentBranchProtection
		err error
	}
	fetched := parallel.Map(ctx, protectedBranches, parallel.DefaultConcurrency, func(ctx context.Context, _ int, branch string) fetchedRule {
		bp, err := p.fetchBranchProtectionRule(ctx, owner, name, branch)
		if err != nil {
			switch {
			case errors.Is(err, gh.ErrNotFound):
				return fetchedRule{}
			case strict:
				return fetchedRule{err: fetchError(strict, "branch protection", owner, name, err)}
			default:
				return fetchedRule{err: unreadSetting(err)}
			}
		}
		return fetchedRule{bp: bp}
	})

	result := make(map[string]*CurrentBranchProtection)
	for i, f := range fetched {
		if f.err != nil {
			return nil, f.err
		}
		if f.bp != nil {
			result[protectedBranches[i]] = f.bp
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
		Restrictions json.RawMessage `json:"restrictions"`
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
	bp.RestrictPushes = len(raw.Restrictions) > 0 && string(raw.Restrictions) != "null"

	return bp, nil
}

func (p *Processor) fetchRulesets(ctx context.Context, owner, name string, strict bool) (map[string]*CurrentRuleset, error) {
	out, err := p.runner.Run(ctx,
		"api", fmt.Sprintf("repos/%s/%s/rulesets", owner, name),
		"--paginate",
	)
	if err != nil {
		// 403 or 404 can mean rulesets are not available: GitHub answers 403
		// on a plan without them (a GitHub Free private repository), and GHES
		// without them 404. Import leaves them unread; when the manifest
		// manages rulesets, they are reported instead: they could not be
		// applied either, and read as none, existing ones would be planned as
		// new. All other errors (429, 5xx) propagate to prevent false diffs.
		if !strict && (errors.Is(err, gh.ErrNotFound) || errors.Is(err, gh.ErrForbidden)) {
			return nil, unreadSetting(err)
		}
		return nil, fetchError(strict, "rulesets", owner, name, err)
	}

	var rawList []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
	}
	if err := json.Unmarshal(out, &rawList); err != nil {
		if strict {
			return nil, fetchError(strict, "rulesets", owner, name, fmt.Errorf("parse response: %w", err))
		}
		return nil, unreadSetting(fmt.Errorf("parse response: %w", err))
	}

	// Filter out org/enterprise-level rulesets (not manageable at repo level).
	ids := make([]int, 0, len(rawList))
	for _, item := range rawList {
		if item.SourceType == "Organization" || item.SourceType == "Enterprise" {
			continue
		}
		ids = append(ids, item.ID)
	}

	// Fetch per-ruleset details in parallel. Only a ruleset that is gone
	// (404) is skipped; for import, one that cannot be read leaves the whole
	// rulesets unread, as for branch protection.
	type fetchedRuleset struct {
		rs  *CurrentRuleset
		err error
	}
	fetched := parallel.Map(ctx, ids, parallel.DefaultConcurrency, func(ctx context.Context, _ int, id int) fetchedRuleset {
		rs, err := p.fetchRuleset(ctx, owner, name, id)
		if err != nil {
			switch {
			case errors.Is(err, gh.ErrNotFound):
				return fetchedRuleset{}
			case strict:
				return fetchedRuleset{err: fetchError(strict, "rulesets", owner, name, err)}
			default:
				return fetchedRuleset{err: unreadSetting(err)}
			}
		}
		return fetchedRuleset{rs: rs}
	})

	result := make(map[string]*CurrentRuleset)
	for _, f := range fetched {
		if f.err != nil {
			return nil, f.err
		}
		if f.rs != nil {
			result[f.rs.Name] = f.rs
		}
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
		case "update":
			var params struct {
				AllowsFetchAndMerge *bool `json:"update_allows_fetch_and_merge"`
			}
			if len(env.Parameters) > 0 {
				if err := json.Unmarshal(env.Parameters, &params); err != nil {
					logger.Debug("cannot parse update rule parameters", "ruleset", rs.Name, "err", err)
				}
			}
			rs.Rules.Update = &CurrentRulesetUpdate{
				AllowsFetchAndMerge: params.AllowsFetchAndMerge,
			}
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

func (p *Processor) fetchSecrets(ctx context.Context, owner, name string, strict bool) ([]string, error) {
	return p.fetchSecretsOf(ctx, owner, name, "", strict)
}

// fetchSecretsOf reads the secret names of the repository, or of its
// environment env when env is not "".
func (p *Processor) fetchSecretsOf(ctx context.Context, owner, name, env string, strict bool) ([]string, error) {
	args := []string{"secret", "list", "--repo", owner + "/" + name}
	what := "secrets"
	if env != "" {
		args = append(args, "--env", env)
		what = fmt.Sprintf("secrets of environment %q", env)
	}
	out, err := p.runner.Run(ctx, append(args, "--json", "name", "--jq", ".[].name")...)
	if err != nil {
		// Secrets the manifest manages that cannot be read are an error:
		// read as none, an existing secret would be planned as new and set
		// again, which nothing would show since values cannot be compared.
		if !strict && isIgnorableSubresourceError(err) {
			return nil, nil
		}
		return nil, fetchError(strict, what, owner, name, err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}
	names := strings.Split(raw, "\n")
	sort.Strings(names)
	return names, nil
}

func (p *Processor) fetchVariables(ctx context.Context, owner, name string, strict bool) (map[string]string, error) {
	return p.fetchVariablesOf(ctx, owner, name, "", strict)
}

// fetchVariablesOf reads the variables of the repository, or of its
// environment env when env is not "".
func (p *Processor) fetchVariablesOf(ctx context.Context, owner, name, env string, strict bool) (map[string]string, error) {
	args := []string{"variable", "list", "--repo", owner + "/" + name}
	what := "variables"
	if env != "" {
		args = append(args, "--env", env)
		what = fmt.Sprintf("variables of environment %q", env)
	}
	out, err := p.runner.Run(ctx, append(args, "--json", "name,value")...)
	if err != nil {
		// As for secrets; import leaves them unread.
		if !strict && isIgnorableSubresourceError(err) {
			return nil, unreadSetting(err)
		}
		return nil, fetchError(strict, what, owner, name, err)
	}

	var vars []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &vars); err != nil {
		if strict {
			return nil, fetchError(strict, what, owner, name, fmt.Errorf("parse response: %w", err))
		}
		return nil, unreadSetting(fmt.Errorf("parse response: %w", err))
	}

	result := make(map[string]string)
	for _, v := range vars {
		result[v.Name] = v.Value
	}
	return result, nil
}

// apiEnvironment is an environment in the shape of the environments API.
type apiEnvironment struct {
	Name            string `json:"name"`
	CanAdminsBypass *bool  `json:"can_admins_bypass"`
	ProtectionRules []struct {
		Type              string `json:"type"`
		WaitTimer         int    `json:"wait_timer"`
		PreventSelfReview bool   `json:"prevent_self_review"`
		Reviewers         []struct {
			Type     string `json:"type"`
			Reviewer struct {
				ID    int    `json:"id"`
				Login string `json:"login"`
				Slug  string `json:"slug"`
			} `json:"reviewer"`
		} `json:"reviewers"`
	} `json:"protection_rules"`
	DeploymentBranchPolicy *struct {
		ProtectedBranches    bool `json:"protected_branches"`
		CustomBranchPolicies bool `json:"custom_branch_policies"`
	} `json:"deployment_branch_policy"`
}

func (e apiEnvironment) current() *CurrentEnvironment {
	env := &CurrentEnvironment{
		Name:            e.Name,
		CanAdminsBypass: e.CanAdminsBypass == nil || *e.CanAdminsBypass, // GitHub's default
		BranchPolicy:    manifest.DeploymentBranchesAll,
	}
	for _, r := range e.ProtectionRules {
		switch r.Type {
		case "wait_timer":
			env.WaitTimer = r.WaitTimer
		case "required_reviewers":
			// The rule can be left with no reviewers when GitHub drops the
			// ones it does not allow.
			env.HasReviewersRule = true
			env.PreventSelfReview = r.PreventSelfReview
			for _, rv := range r.Reviewers {
				name := rv.Reviewer.Login
				if rv.Type == manifest.EnvironmentReviewerTeam {
					name = rv.Reviewer.Slug
				}
				env.Reviewers = append(env.Reviewers, CurrentEnvironmentReviewer{Type: rv.Type, ID: rv.Reviewer.ID, Name: name})
			}
		}
	}
	if p := e.DeploymentBranchPolicy; p != nil {
		switch {
		case p.CustomBranchPolicies:
			env.BranchPolicy = manifest.DeploymentBranchesCustom
		case p.ProtectedBranches:
			env.BranchPolicy = manifest.DeploymentBranchesProtected
		}
	}
	return env
}

// environmentPath is the API path of an environment; its name is a path
// segment, so a "/" in it is escaped.
func environmentPath(owner, name, env string) string {
	return fmt.Sprintf("repos/%s/%s/environments/%s", owner, name, url.PathEscape(env))
}

// fetchEnvironments reads every environment, and the name patterns of the
// custom policies the spec's environments need: those whose
// deployment_branch_policy it sets, since a change from custom removes them.
// With a nil spec (import) it reads the patterns of every custom policy.
// record keeps a part of an environment that import could not read (see
// CurrentState.Unread).
func (p *Processor) fetchEnvironments(ctx context.Context, owner, name string, spec *manifest.RepositorySpec, strict bool, record func(key string, err error) error) (map[string]*CurrentEnvironment, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/%s/environments", owner, name), "--paginate", "--slurp")
	if err != nil {
		if !strict && isIgnorableSubresourceError(err) {
			return nil, unreadSetting(err)
		}
		return nil, fetchError(strict, "environments", owner, name, err)
	}
	var pages []struct {
		Environments []apiEnvironment `json:"environments"`
	}
	if err := json.Unmarshal(out, &pages); err != nil {
		// Import leaves what it cannot read unread, as for the other
		// settings; plan reports it.
		if !strict {
			return nil, unreadSetting(fmt.Errorf("parse response: %w", err))
		}
		return nil, fetchError(strict, "environments", owner, name, fmt.Errorf("parse response: %w", err))
	}

	declared := func(env string) *manifest.Environment {
		for i := range spec.Environments {
			if strings.EqualFold(spec.Environments[i].Name, env) {
				return &spec.Environments[i]
			}
		}
		return nil
	}
	needs := func(env string, manages func(manifest.Environment) bool) bool {
		if spec == nil {
			return true
		}
		e := declared(env)
		return e != nil && manages(*e)
	}
	// Not nil even with no environment: nil means they could not be read.
	envs := make(map[string]*CurrentEnvironment)
	for _, page := range pages {
		for _, e := range page.Environments {
			env := e.current()
			if env.BranchPolicy == manifest.DeploymentBranchesCustom && needs(env.Name, func(e manifest.Environment) bool { return e.DeploymentBranchPolicy != nil }) {
				rules, err := p.fetchBranchRules(ctx, owner, name, env.Name)
				if err != nil {
					if strict {
						return nil, fetchError(strict, fmt.Sprintf("deployment branch policies of environment %q", env.Name), owner, name, err)
					}
					// Read as no patterns, import would write the
					// policy as custom with none.
					if err := record(environmentKey(env.Name, "deployment_branch_policy"), unreadSetting(err)); err != nil {
						return nil, err
					}
				}
				env.BranchRules = rules
			}
			// Import does not write secrets, so it does not read them.
			if spec != nil && needs(env.Name, func(e manifest.Environment) bool { return len(e.Secrets) > 0 }) {
				secrets, err := p.fetchSecretsOf(ctx, owner, name, env.Name, strict)
				if err != nil {
					return nil, err
				}
				env.Secrets = secrets
			}
			if needs(env.Name, func(e manifest.Environment) bool { return len(e.Variables) > 0 }) {
				vars, err := p.fetchVariablesOf(ctx, owner, name, env.Name, strict)
				if err := record(environmentKey(env.Name, "variables"), err); err != nil {
					return nil, err
				}
				env.Variables = vars
			}
			envs[strings.ToLower(env.Name)] = env
		}
	}
	return envs, nil
}

func (p *Processor) fetchBranchRules(ctx context.Context, owner, name, env string) ([]CurrentBranchRule, error) {
	out, err := p.runner.Run(ctx, "api", environmentPath(owner, name, env)+"/deployment-branch-policies", "--paginate", "--slurp")
	if err != nil {
		return nil, err
	}
	var pages []struct {
		BranchPolicies []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"branch_policies"`
	}
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	var rules []CurrentBranchRule
	for _, page := range pages {
		for _, r := range page.BranchPolicies {
			rules = append(rules, CurrentBranchRule{ID: r.ID, Name: r.Name, Type: r.Type})
		}
	}
	return rules, nil
}

func (p *Processor) fetchLabels(ctx context.Context, owner, name string, strict bool) (map[string]*CurrentLabel, error) {
	out, err := p.runner.Run(ctx,
		"label", "list",
		"--repo", owner+"/"+name,
		"--json", "name,color,description",
		"--limit", "1000",
	)
	if err != nil {
		// Managed labels that cannot be read are an error: read as none,
		// existing labels would be planned as new. Import leaves them unread.
		if !strict && isIgnorableSubresourceError(err) {
			return nil, unreadSetting(err)
		}
		return nil, fetchError(strict, "labels", owner, name, err)
	}

	var labels []struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(out, &labels); err != nil {
		if strict {
			return nil, fetchError(strict, "labels", owner, name, fmt.Errorf("parse response: %w", err))
		}
		return nil, unreadSetting(fmt.Errorf("parse response: %w", err))
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

func (p *Processor) fetchMilestones(ctx context.Context, owner, name string, strict bool) (map[string]*CurrentMilestone, error) {
	out, err := p.runner.Run(ctx,
		"api",
		fmt.Sprintf("repos/%s/%s/milestones?state=all&per_page=100", owner, name),
		"--paginate",
	)
	if err != nil {
		// As for labels.
		if !strict && isIgnorableSubresourceError(err) {
			return nil, unreadSetting(err)
		}
		return nil, fetchError(strict, "milestones", owner, name, err)
	}

	var milestones []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		DueOn       string `json:"due_on"`
	}
	if err := json.Unmarshal(out, &milestones); err != nil {
		if strict {
			return nil, fetchError(strict, "milestones", owner, name, fmt.Errorf("parse response: %w", err))
		}
		return nil, unreadSetting(fmt.Errorf("parse response: %w", err))
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

// fetchActionsSettings reads the Actions settings from four endpoints. With a
// spec, an endpoint that cannot be read (404, or a response that does not
// parse) is an error when the spec sets a field it reads: read as its zero
// value, the field would be planned against a value GitHub does not have;
// one whose fields the spec does not set is left at its zero value. Import (a
// nil spec) leaves the whole Actions settings unread when an endpoint cannot
// be read (403, 404, 422, or a response that does not parse): written with
// zero values, they would replace the manifest's.
func (p *Processor) fetchActionsSettings(ctx context.Context, owner, name string, spec *manifest.RepositorySpec) (CurrentActions, error) {
	var managed *manifest.Actions
	if spec != nil {
		managed = spec.Actions
	}
	var result CurrentActions
	fullName := fmt.Sprintf("repos/%s/%s", owner, name)
	// unreadable reports err for what, an endpoint's part of the settings,
	// when the manifest sets a field of it, and leaves it unread otherwise.
	unreadable := func(what string, sets bool, err error) error {
		err = fmt.Errorf("fetch actions %s for %s/%s: %w", what, owner, name, err)
		switch {
		case spec == nil:
			return unreadSetting(err)
		case managed != nil && sets:
			return err
		}
		return nil
	}
	// missing reports whether err only says the endpoint cannot be read,
	// rather than a failure (5xx) that is returned as it is.
	missing := func(err error) bool {
		return errors.Is(err, gh.ErrNotFound) || (spec == nil && isIgnorableSubresourceError(err))
	}

	// 1. Actions permissions (enabled + allowed_actions). The manifest
	// always compares enabled when it manages Actions.
	out, err := p.runner.Run(ctx, "api", fullName+"/actions/permissions")
	if err != nil {
		if missing(err) {
			return result, unreadable("permissions", true, err) // Actions API not available for this repo
		}
		return result, fmt.Errorf("fetch actions permissions for %s/%s: %w", owner, name, err)
	}
	var perms struct {
		Enabled            bool   `json:"enabled"`
		AllowedActions     string `json:"allowed_actions"`
		SHAPinningRequired bool   `json:"sha_pinning_required"`
	}
	if err := json.Unmarshal(out, &perms); err != nil {
		return result, unreadable("permissions", true, fmt.Errorf("parse response: %w", err))
	}
	result.Enabled = perms.Enabled
	result.AllowedActions = perms.AllowedActions
	result.SHAPinningRequired = perms.SHAPinningRequired

	// 2. Workflow permissions (GITHUB_TOKEN defaults)
	setsWorkflow := managed != nil && (managed.WorkflowPermissions != nil || managed.CanApprovePullRequests != nil)
	out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/workflow")
	if err != nil && !missing(err) {
		return result, fmt.Errorf("fetch actions workflow permissions for %s/%s: %w", owner, name, err)
	}
	if err != nil {
		if err := unreadable("workflow permissions", setsWorkflow, err); err != nil {
			return result, err
		}
	} else {
		var wf struct {
			DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
			CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
		}
		if err := json.Unmarshal(out, &wf); err != nil {
			if err := unreadable("workflow permissions", setsWorkflow, fmt.Errorf("parse response: %w", err)); err != nil {
				return result, err
			}
		} else {
			result.WorkflowPermissions = wf.DefaultWorkflowPermissions
			result.CanApprovePullRequests = wf.CanApprovePullRequestReviews
		}
	}

	// 3. Selected actions (only when allowed_actions == "selected")
	if result.AllowedActions == "selected" {
		setsSelected := managed != nil && managed.SelectedActions != nil
		out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/selected-actions")
		if err != nil && !missing(err) {
			return result, fmt.Errorf("fetch actions selected-actions for %s/%s: %w", owner, name, err)
		}
		if err != nil {
			if err := unreadable("selected-actions", setsSelected, err); err != nil {
				return result, err
			}
		} else {
			var sa struct {
				GithubOwnedAllowed bool     `json:"github_owned_allowed"`
				VerifiedAllowed    bool     `json:"verified_allowed"`
				PatternsAllowed    []string `json:"patterns_allowed"`
			}
			if err := json.Unmarshal(out, &sa); err != nil {
				if err := unreadable("selected-actions", setsSelected, fmt.Errorf("parse response: %w", err)); err != nil {
					return result, err
				}
			} else {
				result.SelectedActions = &CurrentSelectedActions{
					GithubOwnedAllowed: sa.GithubOwnedAllowed,
					VerifiedAllowed:    sa.VerifiedAllowed,
					PatternsAllowed:    sa.PatternsAllowed,
				}
			}
		}
	}

	// 4. Fork PR approval. GitHub answers 422 on private repositories, which
	// do not have it; a manifest that sets it there could not apply it, so
	// that is an error too, unless the manifest makes the repository public
	// or internal: apply changes the visibility first, so what the private
	// repository has does not matter.
	setsForkPR := managed != nil && managed.ForkPRApproval != nil
	madeNotPrivate := spec != nil && spec.Visibility != nil && *spec.Visibility != manifest.VisibilityPrivate
	out, err = p.runner.Run(ctx, "api", fullName+"/actions/permissions/fork-pr-contributor-approval")
	if err != nil && !missing(err) && !errors.Is(err, gh.ErrValidation) {
		return result, fmt.Errorf("fetch actions fork-pr-approval for %s/%s: %w", owner, name, err)
	}
	// For import, a 404 or 422 says the repository does not have the
	// setting (a private one does not), so it is left empty, not unread.
	notAvailable := spec == nil && (errors.Is(err, gh.ErrNotFound) || errors.Is(err, gh.ErrValidation))
	if err != nil {
		privateNow := errors.Is(err, gh.ErrValidation)
		if !notAvailable {
			if err := unreadable("fork-pr-approval", setsForkPR && (!privateNow || !madeNotPrivate), err); err != nil {
				return result, err
			}
		}
	} else {
		var fpr struct {
			ApprovalPolicy string `json:"approval_policy"`
		}
		if err := json.Unmarshal(out, &fpr); err != nil {
			if err := unreadable("fork-pr-approval", setsForkPR, fmt.Errorf("parse response: %w", err)); err != nil {
				return result, err
			}
		} else {
			result.ForkPRApproval = fpr.ApprovalPolicy
		}
	}

	return result, nil
}

// unreadError marks a setting import could not read: FetchRepository records
// it in CurrentState.Unread and leaves the setting unset.
type unreadError struct{ err error }

func (e unreadError) Error() string { return e.err.Error() }
func (e unreadError) Unwrap() error { return e.err }

// isParseError reports whether err is a JSON response that does not parse.
func isParseError(err error) bool {
	var syntax *json.SyntaxError
	var typ *json.UnmarshalTypeError
	return errors.As(err, &syntax) || errors.As(err, &typ)
}

// unreadSetting marks err as a setting import could not read.
func unreadSetting(err error) error { return unreadError{err: err} }

// environmentKey is the Unread key of part, a field of the environment env.
func environmentKey(env, part string) string {
	return "environments." + env + "." + part
}

// forbiddenWhenManaged reports whether err is a 403 for a resource the
// manifest manages (strict). Such a 403 is an error: treating the resource as
// absent or as its default would plan changes that apply cannot make.
func forbiddenWhenManaged(strict bool, err error) bool {
	return strict && errors.Is(err, gh.ErrForbidden)
}

// permissionFor names the fine-grained token permission that reads what.
var permissionFor = map[string]string{
	"branch protection":        "Administration",
	"secrets":                  "Secrets",
	"variables":                "Variables",
	"labels":                   "Issues",
	"milestones":               "Issues",
	"commit message settings":  "Contents",
	"release immutability":     "Administration",
	"vulnerability alerts":     "Administration",
	"automated security fixes": "Administration",
	"actions settings":         "Administration",
}

// fetchError wraps a failed fetch of what. A 403 on a managed resource says
// that it could not be read, and which permission reads it.
func fetchError(strict bool, what, owner, name string, err error) error {
	if forbiddenWhenManaged(strict, err) {
		if perm := permissionFor[what]; perm != "" {
			return fmt.Errorf("cannot read %s of %s/%s (the token needs the %s permission): %w", what, owner, name, perm, err)
		}
		return fmt.Errorf("cannot read %s of %s/%s: %w", what, owner, name, err)
	}
	return fmt.Errorf("fetch %s for %s/%s: %w", what, owner, name, err)
}

// isIgnorableSubresourceError reports whether import (no spec) can leave a
// sub-resource (secrets, variables, labels, milestones, environments) that
// failed to fetch empty instead of aborting: one that may be inaccessible due
// to permissions, plan restrictions, or non-existence. Plan reports these for
// the settings it manages. All other errors (network, timeout, 5xx) are
// propagated.
func isIgnorableSubresourceError(err error) bool {
	return errors.Is(err, gh.ErrNotFound) ||
		errors.Is(err, gh.ErrForbidden) ||
		errors.Is(err, gh.ErrValidation)
}
