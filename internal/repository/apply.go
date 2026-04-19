package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
)

// Apply executes all changes in the plan result.
// Changes are grouped by repo and applied in parallel across repos.
// Within a single repo, changes are applied sequentially to maintain ordering.
func (p *Processor) Apply(ctx context.Context, changes []Change, repos []*manifest.Repository, reporter ProgressReporter) []ApplyResult {
	if reporter == nil {
		reporter = noopProgressReporter{}
	}
	repoMap := make(map[string]*manifest.Repository)
	for _, r := range repos {
		repoMap[r.Metadata.FullName()] = r
	}

	groups := groupByName(changes)
	if len(groups) == 0 {
		return nil
	}

	// Apply repo groups in parallel
	allResults := parallel.Map(ctx, groups, parallel.DefaultConcurrency, func(ctx context.Context, _ int, g changeGroup) []ApplyResult {
		fields := make([]string, 0, len(g.changes))
		for _, c := range g.changes {
			fields = append(fields, c.Field)
		}
		reporter.Start(g.name, fields)

		start := time.Now()
		var results []ApplyResult
		for _, c := range g.changes {
			reporter.UpdateStatus(g.name, "applying "+applyStatusTarget(c)+"...")
			result := p.applyChange(ctx, c, repoMap[c.Name])
			results = append(results, result)
		}
		elapsed := time.Since(start)

		var firstErr error
		for _, r := range results {
			if r.Err != nil {
				firstErr = r.Err
				break
			}
		}

		if firstErr != nil {
			reporter.Error(g.name, elapsed, firstErr)
		} else {
			reporter.Done(g.name, elapsed, len(results))
		}
		return results
	})
	reporter.Wait()

	// Flatten in order
	var results []ApplyResult
	for _, r := range allResults {
		results = append(results, r...)
	}
	return results
}

func applyStatusTarget(c Change) string {
	if resource, key, ok := splitApplyResource(c.Resource); ok {
		switch resource {
		case manifest.ResourceRuleset:
			return fmt.Sprintf("ruleset %q", key)
		case manifest.ResourceBranchProtection:
			return fmt.Sprintf("branch protection %q", key)
		}
	}
	switch c.Resource {
	case manifest.ResourceLabel:
		return fmt.Sprintf("label %q", c.Field)
	case manifest.ResourceMilestone:
		return fmt.Sprintf("milestone %q", c.Field)
	case manifest.ResourceSecret:
		return fmt.Sprintf("secret %q", c.Field)
	case manifest.ResourceVariable:
		return fmt.Sprintf("variable %q", c.Field)
	case manifest.ResourceRuleset:
		return fmt.Sprintf("ruleset %q", c.Field)
	case manifest.ResourceBranchProtection:
		return fmt.Sprintf("branch protection %q", c.Field)
	case manifest.ResourceActions:
		return "actions settings"
	case manifest.ResourceRepository:
		if c.Field == "" {
			return "repository settings"
		}
		return "repository " + c.Field
	default:
		if c.Field == "" {
			return strings.ToLower(c.Resource)
		}
		return c.Field
	}
}

func splitApplyResource(resource string) (name, key string, ok bool) {
	prefix, rest, found := strings.Cut(resource, "[")
	if !found || !strings.HasSuffix(rest, "]") {
		return "", "", false
	}
	key = strings.TrimSuffix(rest, "]")
	if prefix == "" || key == "" {
		return "", "", false
	}
	return prefix, key, true
}

type ApplyResult struct {
	Change Change
	Err    error
}

type rulesetLookup struct {
	Repo        string
	RulesetName string
	Target      string
}

func (p *Processor) applyChange(ctx context.Context, c Change, repo *manifest.Repository) ApplyResult {
	// Merge strategy children are batched into a single PATCH to avoid
	// ordering issues (e.g. squash_merge_commit_title requires allow_squash_merge).
	if len(c.Children) > 0 && c.Field == "merge_strategy" {
		return p.applyMergeStrategyBatch(ctx, c)
	}

	// Generic: if this change has children, expand and apply each child.
	// Label and Milestone updates carry children for display (e.g. color, description)
	// but should be applied as a single API call using the parent's Field (the name/title).
	// Delete changes also carry children for display only — the parent handles the API call.
	if len(c.Children) > 0 && c.Type != ChangeDelete && c.Resource != manifest.ResourceLabel && c.Resource != manifest.ResourceMilestone {
		for _, child := range c.Children {
			child.Resource = c.Resource
			child.Name = c.Name
			if result := p.applyChange(ctx, child, repo); result.Err != nil {
				return ApplyResult{Change: c, Err: result.Err}
			}
		}
		return ApplyResult{Change: c}
	}

	var err error

	switch {
	case c.Resource == manifest.ResourceRepository && c.Type == ChangeCreate && c.Field == "repository":
		err = p.createRepo(ctx, repo)
	case c.Resource == manifest.ResourceRepository:
		err = p.applyRepoSetting(ctx, c, repo)
	case strings.HasPrefix(c.Resource, manifest.ResourceBranchProtection):
		err = p.applyBranchProtection(ctx, c, repo)
	case strings.HasPrefix(c.Resource, manifest.ResourceRuleset):
		err = p.applyRuleset(ctx, c, repo)
	case c.Resource == manifest.ResourceSecret:
		err = p.applySecret(ctx, c, repo)
	case c.Resource == manifest.ResourceVariable:
		err = p.applyVariable(ctx, c, repo)
	case c.Resource == manifest.ResourceLabel:
		err = p.applyLabel(ctx, c, repo)
	case c.Resource == manifest.ResourceMilestone:
		err = p.applyMilestone(ctx, c, repo)
	case c.Resource == manifest.ResourceActions:
		err = p.applyActions(ctx, c, repo)
	default:
		err = fmt.Errorf("unknown resource type: %s", c.Resource)
	}

	return ApplyResult{Change: c, Err: err}
}

func (p *Processor) createRepo(ctx context.Context, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	args := []string{"repo", "create", fullName}

	// Visibility
	visibility := "private" // default
	if repo.Spec.Visibility != nil {
		visibility = *repo.Spec.Visibility
	}
	args = append(args, "--"+visibility)

	// Description
	if repo.Spec.Description != nil {
		args = append(args, "--description", *repo.Spec.Description)
	}

	// Disable features that are false
	if f := repo.Spec.Features; f != nil {
		if f.Wiki != nil && !*f.Wiki {
			args = append(args, "--disable-wiki")
		}
		if f.Issues != nil && !*f.Issues {
			args = append(args, "--disable-issues")
		}
	}

	_, err := p.runner.Run(ctx, args...)
	if err != nil {
		return wrapError(err, fullName, "create")
	}

	// Apply remaining settings via gh repo edit
	return p.applyAllSettings(ctx, repo)
}

func (p *Processor) applyAllSettings(ctx context.Context, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	// Batch features, merge strategy, and homepage into a single PATCH.
	// This avoids ordering issues (e.g. squash_merge_commit_title requires
	// allow_squash_merge) and reduces API calls.
	if err := p.applyRepoPatch(ctx, fullName, repo); err != nil {
		return err
	}

	// Topics
	for _, t := range repo.Spec.Topics {
		if _, err := p.runner.Run(ctx, "repo", "edit", fullName, "--add-topic", t); err != nil {
			return wrapError(err, fullName, "add-topic:"+t)
		}
	}

	// Release immutability
	if repo.Spec.ReleaseImmutability != nil {
		if err := p.applyReleaseImmutability(ctx, owner, name, *repo.Spec.ReleaseImmutability); err != nil {
			return err
		}
	}

	// Security (Advanced Security)
	if s := repo.Spec.Security; s != nil {
		if s.VulnerabilityAlerts != nil {
			if err := p.applyVulnerabilityAlerts(ctx, owner, name, *s.VulnerabilityAlerts); err != nil {
				return err
			}
		}
		if s.AutomatedSecurityFixes != nil {
			if err := p.applyAutomatedSecurityFixes(ctx, owner, name, *s.AutomatedSecurityFixes); err != nil {
				return err
			}
		}
		if s.PrivateVulnerabilityReporting != nil {
			if err := p.applyPrivateVulnerabilityReporting(ctx, owner, name, *s.PrivateVulnerabilityReporting); err != nil {
				return err
			}
		}
	}

	// Actions (permissions, workflow defaults, selected actions, fork PR)
	if a := repo.Spec.Actions; a != nil && a.Enabled != nil {
		visibility := repo.Spec.Visibility
		if visibility == nil {
			visibility = manifest.Ptr(manifest.VisibilityPrivate)
		}
		if err := p.applyActionsPermissions(ctx, owner, name, a); err != nil {
			return err
		}
		if err := p.applyActionsWorkflow(ctx, owner, name, a); err != nil {
			return err
		}
		if a.SelectedActions != nil {
			if err := p.applyActionsSelectedActions(ctx, owner, name, a); err != nil {
				return err
			}
		}
		if a.ForkPRApproval != nil {
			if err := p.applyActionsForkPR(ctx, owner, name, a, visibility); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyRepoPatch batches features, merge strategy, and homepage into a single
// repos PATCH call. Used during repo creation to avoid ordering issues and
// reduce API calls.
func (p *Processor) applyRepoPatch(ctx context.Context, fullName string, repo *manifest.Repository) error {
	payload := map[string]any{}

	// Features
	if f := repo.Spec.Features; f != nil {
		if f.Projects != nil {
			payload["has_projects"] = *f.Projects
		}
		if f.Discussions != nil {
			payload["has_discussions"] = *f.Discussions
		}
		if f.PullRequests != nil {
			if f.PullRequests.Enabled != nil {
				payload["has_pull_requests"] = *f.PullRequests.Enabled
			}
			if f.PullRequests.Creation != nil {
				payload["pull_request_creation_policy"] = *f.PullRequests.Creation
			}
		}
	}

	// Merge strategy
	if ms := repo.Spec.MergeStrategy; ms != nil {
		if ms.AllowMergeCommit != nil {
			payload["allow_merge_commit"] = *ms.AllowMergeCommit
		}
		if ms.AllowSquashMerge != nil {
			payload["allow_squash_merge"] = *ms.AllowSquashMerge
		}
		if ms.AllowRebaseMerge != nil {
			payload["allow_rebase_merge"] = *ms.AllowRebaseMerge
		}
		if ms.AllowAutoMerge != nil {
			payload["allow_auto_merge"] = *ms.AllowAutoMerge
		}
		if ms.AutoDeleteHeadBranches != nil {
			payload["delete_branch_on_merge"] = *ms.AutoDeleteHeadBranches
		}
		if ms.SquashMergeCommitTitle != nil {
			payload["squash_merge_commit_title"] = *ms.SquashMergeCommitTitle
		}
		if ms.SquashMergeCommitMessage != nil {
			payload["squash_merge_commit_message"] = *ms.SquashMergeCommitMessage
		}
		if ms.MergeCommitTitle != nil {
			payload["merge_commit_title"] = *ms.MergeCommitTitle
		}
		if ms.MergeCommitMessage != nil {
			payload["merge_commit_message"] = *ms.MergeCommitMessage
		}
	}

	// Homepage
	if repo.Spec.Homepage != nil {
		payload["homepage"] = *repo.Spec.Homepage
	}

	if len(payload) == 0 {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s", fullName),
		"--method", "PATCH",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, fullName, "settings")
}

// applyMergeStrategyBatch batches merge strategy children into a single repos
// PATCH call. Used during updates when multiple merge strategy fields change
// together.
func (p *Processor) applyMergeStrategyBatch(ctx context.Context, c Change) ApplyResult {
	fullName := c.Name
	payload := map[string]any{}
	for _, child := range c.Children {
		switch child.Field {
		case "auto_delete_head_branches":
			payload["delete_branch_on_merge"] = child.NewValue
		default:
			payload[child.Field] = child.NewValue
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ApplyResult{Change: c, Err: err}
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s", fullName),
		"--method", "PATCH",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return ApplyResult{Change: c, Err: wrapError(err, fullName, "merge_strategy")}
}

func (p *Processor) applyRepoSetting(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	switch c.Field {
	case "description":
		_, err := p.runner.Run(ctx, "repo", "edit", fullName, "--description", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "homepage":
		_, err := p.runner.Run(ctx, "repo", "edit", fullName, "--homepage", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "visibility":
		_, err := p.runner.Run(ctx, "repo", "edit", fullName, "--visibility", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "archived":
		archived, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for archived: %T", c.NewValue)
		}
		if archived {
			_, err := p.runner.Run(ctx, "repo", "archive", fullName, "--yes")
			return wrapError(err, fullName, c.Field)
		}
		_, err := p.runner.Run(ctx, "repo", "unarchive", fullName, "--yes")
		return wrapError(err, fullName, c.Field)

	case "topics":
		return p.applyTopics(ctx, fullName, repo)

	case "release_immutability":
		enabled, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.applyReleaseImmutability(ctx, owner, name, enabled)

	case "vulnerability_alerts":
		enabled, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.applyVulnerabilityAlerts(ctx, owner, name, enabled)

	case "automated_security_fixes":
		enabled, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.applyAutomatedSecurityFixes(ctx, owner, name, enabled)

	case "private_vulnerability_reporting":
		enabled, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.applyPrivateVulnerabilityReporting(ctx, owner, name, enabled)

	case "issues":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-issues", v)
	case "projects":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-projects", v)
	case "wiki":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-wiki", v)
	case "discussions":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-discussions", v)
	case "pull_requests":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.updateRepoField(ctx, owner+"/"+name, "has_pull_requests", fmt.Sprintf("%t", v))
	case "pull_requests.creation":
		v, ok := c.NewValue.(string)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.updateRepoField(ctx, owner+"/"+name, "pull_request_creation_policy", v)
	case "allow_merge_commit":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-merge-commit", v)
	case "allow_squash_merge":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-squash-merge", v)
	case "allow_rebase_merge":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "enable-rebase-merge", v)
	case "auto_delete_head_branches":
		v, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for %s: %T", c.Field, c.NewValue)
		}
		return p.toggleFeature(ctx, fullName, "delete-branch-on-merge", v)

	case "merge_commit_title", "merge_commit_message", "squash_merge_commit_title", "squash_merge_commit_message":
		return p.updateRepoField(ctx, owner+"/"+name, c.Field, fmt.Sprintf("%v", c.NewValue))

	default:
		return fmt.Errorf("unknown field: %s", c.Field)
	}
}

func (p *Processor) updateRepoField(ctx context.Context, fullName, field, value string) error {
	endpoint := fmt.Sprintf("repos/%s", fullName)
	_, err := p.runner.Run(ctx, "api", endpoint, "--method", "PATCH",
		"-f", fmt.Sprintf("%s=%s", field, value),
	)
	return wrapError(err, fullName, field)
}

func (p *Processor) toggleFeature(ctx context.Context, repo, flag string, enable bool) error {
	arg := fmt.Sprintf("--%s=%t", flag, enable)
	_, err := p.runner.Run(ctx, "repo", "edit", repo, arg)
	return wrapError(err, repo, flag)
}

func (p *Processor) applyTopics(ctx context.Context, fullName string, repo *manifest.Repository) error {
	// Get current topics
	out, err := p.runner.Run(ctx, "repo", "view", fullName, "--json", "repositoryTopics", "--jq", ".repositoryTopics // [] | .[].name")
	if err != nil {
		return wrapError(err, fullName, "topics")
	}

	currentTopics := make(map[string]bool)
	for t := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if t != "" {
			currentTopics[t] = true
		}
	}

	desiredTopics := make(map[string]bool)
	for _, t := range repo.Spec.Topics {
		desiredTopics[t] = true
	}

	// Remove topics not in desired
	for t := range currentTopics {
		if !desiredTopics[t] {
			if _, err := p.runner.Run(ctx, "repo", "edit", fullName, "--remove-topic", t); err != nil {
				return wrapError(err, fullName, "remove-topic:"+t)
			}
		}
	}

	// Add topics not in current
	for t := range desiredTopics {
		if !currentTopics[t] {
			if _, err := p.runner.Run(ctx, "repo", "edit", fullName, "--add-topic", t); err != nil {
				return wrapError(err, fullName, "add-topic:"+t)
			}
		}
	}

	return nil
}

func (p *Processor) applyReleaseImmutability(ctx context.Context, owner, name string, enable bool) error {
	endpoint := fmt.Sprintf("repos/%s/%s/immutable-releases", owner, name)
	fullName := owner + "/" + name
	if enable {
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "PUT")
		return wrapError(err, fullName, "release_immutability")
	}
	_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
	return wrapError(err, fullName, "release_immutability")
}

func (p *Processor) applyVulnerabilityAlerts(ctx context.Context, owner, name string, enable bool) error {
	endpoint := fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, name)
	fullName := owner + "/" + name
	if enable {
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "PUT")
		return wrapError(err, fullName, "vulnerability_alerts")
	}
	_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
	return wrapError(err, fullName, "vulnerability_alerts")
}

func (p *Processor) applyAutomatedSecurityFixes(ctx context.Context, owner, name string, enable bool) error {
	endpoint := fmt.Sprintf("repos/%s/%s/automated-security-fixes", owner, name)
	fullName := owner + "/" + name
	if enable {
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "PUT")
		return wrapError(err, fullName, "automated_security_fixes")
	}
	_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
	return wrapError(err, fullName, "automated_security_fixes")
}

func (p *Processor) applyPrivateVulnerabilityReporting(ctx context.Context, owner, name string, enable bool) error {
	endpoint := fmt.Sprintf("repos/%s/%s/private-vulnerability-reporting", owner, name)
	fullName := owner + "/" + name
	if enable {
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "PUT")
		return wrapError(err, fullName, "private_vulnerability_reporting")
	}
	_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
	return wrapError(err, fullName, "private_vulnerability_reporting")
}

func (p *Processor) applyBranchProtection(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	if c.Type == ChangeDelete {
		var pattern string
		if after, ok := strings.CutPrefix(c.Resource, manifest.ResourceBranchProtection+"["); ok {
			pattern = strings.TrimSuffix(after, "]")
		}
		endpoint := fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, name, pattern)
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
		return wrapError(err, owner+"/"+name, "branch_protection:"+pattern)
	}

	// Find the matching branch protection rule from desired state
	var pattern string
	// Extract pattern from resource name like "BranchProtection[main]"
	if after, ok := strings.CutPrefix(c.Resource, manifest.ResourceBranchProtection+"["); ok {
		pattern = strings.TrimSuffix(after, "]")
	}

	var bp *manifest.BranchProtection
	branchProtection := repo.Spec.BranchProtection.Get()
	for i := range branchProtection {
		if branchProtection[i].Pattern == pattern {
			bp = &branchProtection[i]
			break
		}
	}
	if bp == nil {
		return fmt.Errorf("branch protection rule %q not found in desired state", pattern)
	}

	// Use the field-based API approach (Runner doesn't pipe stdin).
	return p.applyBranchProtectionViaAPI(ctx, owner, name, bp)
}

func (p *Processor) applyBranchProtectionViaAPI(ctx context.Context, owner, name string, bp *manifest.BranchProtection) error {
	payload := buildBranchProtectionPayload(bp)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal branch protection: %w", err)
	}

	endpoint := fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, name, bp.Pattern)

	_, err = p.runner.RunWithStdin(ctx, payloadJSON,
		"api", endpoint,
		"--method", "PUT",
		"--header", "Accept: application/vnd.github+json",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, owner+"/"+name, "branch_protection:"+bp.Pattern)
}

func buildBranchProtectionPayload(bp *manifest.BranchProtection) map[string]any {
	payload := map[string]any{
		"enforce_admins":     derefBool(bp.EnforceAdmins),
		"restrictions":       nil,
		"allow_force_pushes": derefBool(bp.AllowForcePushes),
		"allow_deletions":    derefBool(bp.AllowDeletions),
	}

	if bp.RequiredReviews != nil || bp.DismissStaleReviews != nil || bp.RequireCodeOwnerReviews != nil {
		reviews := map[string]any{}
		if bp.RequiredReviews != nil {
			reviews["required_approving_review_count"] = *bp.RequiredReviews
		}
		if bp.DismissStaleReviews != nil {
			reviews["dismiss_stale_reviews"] = *bp.DismissStaleReviews
		}
		if bp.RequireCodeOwnerReviews != nil {
			reviews["require_code_owner_reviews"] = *bp.RequireCodeOwnerReviews
		}
		payload["required_pull_request_reviews"] = reviews
	} else {
		payload["required_pull_request_reviews"] = nil
	}

	if bp.RequireStatusChecks != nil {
		payload["required_status_checks"] = map[string]any{
			"strict":   bp.RequireStatusChecks.Strict,
			"contexts": bp.RequireStatusChecks.Contexts,
		}
	} else {
		payload["required_status_checks"] = nil
	}

	return payload
}

func (p *Processor) applyRuleset(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	if c.Type == ChangeDelete {
		info, ok := c.OldValue.(rulesetDeleteInfo)
		if !ok {
			return fmt.Errorf("unexpected OldValue type for ruleset delete: %T", c.OldValue)
		}
		endpoint := fmt.Sprintf("repos/%s/%s/rulesets/%d", owner, name, info.ID)
		_, err := p.runner.Run(ctx, "api", endpoint, "--method", "DELETE")
		return wrapError(err, owner+"/"+name, "ruleset:"+info.Name)
	}

	// Extract ruleset name from resource like "Ruleset[protect-main]"
	rulesetName := strings.TrimSuffix(strings.TrimPrefix(c.Resource, manifest.ResourceRuleset+"["), "]")

	var rs *manifest.Ruleset
	rulesets := repo.Spec.Rulesets.Get()
	for i := range rulesets {
		if rulesets[i].Name == rulesetName {
			rs = &rulesets[i]
			break
		}
	}
	if rs == nil {
		return fmt.Errorf("ruleset %q not found in desired state", rulesetName)
	}

	payload, err := buildRulesetPayload(ctx, rs, p.resolver)
	if err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ruleset payload: %w", err)
	}

	switch c.Type {
	case ChangeCreate:
		_, err = p.runner.RunWithStdin(ctx, payloadJSON,
			"api",
			fmt.Sprintf("repos/%s/%s/rulesets", owner, name),
			"--method", "POST",
			"--header", "Accept: application/vnd.github+json",
			"--header", "Content-Type: application/json",
			"--input", "-",
		)
		return wrapError(err, owner+"/"+name, "ruleset:"+rulesetName)

	case ChangeUpdate:
		target := "branch"
		if rs.Target != nil {
			target = *rs.Target
		}
		rulesetID, err := p.resolveRulesetID(ctx, rulesetLookup{
			Repo:        owner + "/" + name,
			RulesetName: rulesetName,
			Target:      target,
		})
		if err != nil {
			return err
		}
		_, err = p.runner.RunWithStdin(ctx, payloadJSON,
			"api",
			fmt.Sprintf("repos/%s/%s/rulesets/%d", owner, name, rulesetID),
			"--method", "PUT",
			"--header", "Accept: application/vnd.github+json",
			"--header", "Content-Type: application/json",
			"--input", "-",
		)
		return wrapError(err, owner+"/"+name, "ruleset:"+rulesetName)
	}

	return nil
}

func (p *Processor) resolveRulesetID(ctx context.Context, lookup rulesetLookup) (int, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/rulesets", lookup.Repo))
	if err != nil {
		return 0, fmt.Errorf("list rulesets for %s: %w", lookup.Repo, err)
	}

	var rulesets []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(out, &rulesets); err != nil {
		return 0, fmt.Errorf("parse rulesets list for %s: %w", lookup.Repo, err)
	}

	var matches []int
	for _, rs := range rulesets {
		if rs.Name == lookup.RulesetName && rs.Target == lookup.Target {
			matches = append(matches, rs.ID)
		}
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("ruleset %q (target=%s) not found in %s", lookup.RulesetName, lookup.Target, lookup.Repo)
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("multiple rulesets named %q (target=%s) found in %s; cannot determine which to update", lookup.RulesetName, lookup.Target, lookup.Repo)
	}
}

func buildRulesetPayload(ctx context.Context, rs *manifest.Ruleset, resolver *manifest.Resolver) (map[string]any, error) {
	target := "branch"
	if rs.Target != nil {
		target = *rs.Target
	}
	enforcement := "active"
	if rs.Enforcement != nil {
		enforcement = *rs.Enforcement
	}

	payload := map[string]any{
		"name":        rs.Name,
		"target":      target,
		"enforcement": enforcement,
	}

	// bypass_actors (resolve names → IDs)
	if len(rs.BypassActors) > 0 && resolver != nil {
		resolved, err := resolver.ResolveBypassActors(ctx, rs.BypassActors)
		if err != nil {
			return nil, fmt.Errorf("resolve bypass actors: %w", err)
		}
		actors := make([]map[string]any, len(resolved))
		for i, a := range resolved {
			actors[i] = map[string]any{
				"actor_id":    a.ActorID,
				"actor_type":  a.ActorType,
				"bypass_mode": a.BypassMode,
			}
		}
		payload["bypass_actors"] = actors
	} else {
		payload["bypass_actors"] = []map[string]any{}
	}

	// conditions
	if rs.Conditions != nil && rs.Conditions.RefName != nil {
		exclude := rs.Conditions.RefName.Exclude
		if exclude == nil {
			exclude = []string{}
		}
		payload["conditions"] = map[string]any{
			"ref_name": map[string]any{
				"include": rs.Conditions.RefName.Include,
				"exclude": exclude,
			},
		}
	}

	// rules
	var rules []map[string]any

	if rs.Rules.PullRequest != nil {
		pr := rs.Rules.PullRequest
		params := map[string]any{}
		if pr.RequiredApprovingReviewCount != nil {
			params["required_approving_review_count"] = *pr.RequiredApprovingReviewCount
		}
		if pr.DismissStaleReviewsOnPush != nil {
			params["dismiss_stale_reviews_on_push"] = *pr.DismissStaleReviewsOnPush
		}
		if pr.RequireCodeOwnerReview != nil {
			params["require_code_owner_review"] = *pr.RequireCodeOwnerReview
		}
		if pr.RequireLastPushApproval != nil {
			params["require_last_push_approval"] = *pr.RequireLastPushApproval
		}
		if pr.RequiredReviewThreadResolution != nil {
			params["required_review_thread_resolution"] = *pr.RequiredReviewThreadResolution
		}
		rules = append(rules, map[string]any{"type": "pull_request", "parameters": params})
	}

	if rs.Rules.RequiredStatusChecks != nil && resolver != nil {
		sc := rs.Rules.RequiredStatusChecks
		resolvedChecks, err := resolver.ResolveStatusChecks(ctx, sc.Contexts)
		if err != nil {
			return nil, fmt.Errorf("resolve status checks: %w", err)
		}
		checks := make([]map[string]any, len(resolvedChecks))
		for i, rc := range resolvedChecks {
			check := map[string]any{"context": rc.Context}
			if rc.IntegrationID != 0 {
				check["integration_id"] = rc.IntegrationID
			}
			checks[i] = check
		}
		params := map[string]any{"required_status_checks": checks}
		if sc.StrictRequiredStatusChecksPolicy != nil {
			params["strict_required_status_checks_policy"] = *sc.StrictRequiredStatusChecksPolicy
		}
		rules = append(rules, map[string]any{"type": "required_status_checks", "parameters": params})
	}

	// Toggle rules
	toggles := map[string]*bool{
		"non_fast_forward":        rs.Rules.NonFastForward,
		"deletion":                rs.Rules.Deletion,
		"creation":                rs.Rules.Creation,
		"required_linear_history": rs.Rules.RequiredLinearHistory,
		"required_signatures":     rs.Rules.RequiredSignatures,
	}
	for ruleType, enabled := range toggles {
		if enabled != nil && *enabled {
			rules = append(rules, map[string]any{"type": ruleType})
		}
	}

	payload["rules"] = rules
	return payload, nil
}

func (p *Processor) applySecret(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	// Find secret value from desired state
	var value string
	for _, s := range repo.Spec.Secrets {
		if s.Name == c.Field {
			value = s.Value
			break
		}
	}

	_, err := p.runner.Run(ctx, "secret", "set", c.Field,
		"--repo", fullName,
		"--body", value,
	)
	return wrapError(err, fullName, "secret:"+c.Field)
}

func (p *Processor) applyVariable(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	// Find variable value from desired state
	var value string
	for _, v := range repo.Spec.Variables {
		if v.Name == c.Field {
			value = v.Value
			break
		}
	}

	_, err := p.runner.Run(ctx, "variable", "set", c.Field,
		"--repo", fullName,
		"--body", value,
	)
	return wrapError(err, fullName, "variable:"+c.Field)
}

func (p *Processor) applyLabel(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	if c.Type == ChangeDelete {
		_, err := p.runner.Run(ctx, "label", "delete", c.Field, "--repo", fullName, "--yes")
		return wrapError(err, fullName, "label:"+c.Field)
	}

	// Find label from desired state (required for create/update)
	var label *manifest.Label
	for i := range repo.Spec.Labels {
		if repo.Spec.Labels[i].Name == c.Field {
			label = &repo.Spec.Labels[i]
			break
		}
	}
	if label == nil {
		return fmt.Errorf("label %q not found in desired state", c.Field)
	}

	switch c.Type {
	case ChangeCreate:
		args := []string{"label", "create", c.Field, "--repo", fullName, "--color", label.Color}
		if label.Description != "" {
			args = append(args, "--description", label.Description)
		}
		_, err := p.runner.Run(ctx, args...)
		return wrapError(err, fullName, "label:"+c.Field)
	case ChangeUpdate:
		args := []string{"label", "edit", c.Field, "--repo", fullName}
		args = append(args, "--color", label.Color)
		args = append(args, "--description", label.Description)
		_, err := p.runner.Run(ctx, args...)
		return wrapError(err, fullName, "label:"+c.Field)
	}

	return nil
}

func (p *Processor) applyMilestone(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	// Find milestone from desired state
	var ms *manifest.Milestone
	for i := range repo.Spec.Milestones {
		if repo.Spec.Milestones[i].Title == c.Field {
			ms = &repo.Spec.Milestones[i]
			break
		}
	}
	if ms == nil {
		return fmt.Errorf("milestone %q not found in desired state", c.Field)
	}

	payload := map[string]any{
		"title":       ms.Title,
		"description": ms.Description,
		"state":       manifest.MilestoneState(ms.State),
	}
	if ms.DueOn != nil && *ms.DueOn != "" {
		payload["due_on"] = *ms.DueOn + "T00:00:00Z"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	switch c.Type {
	case ChangeCreate:
		_, err = p.runner.RunWithStdin(ctx, body,
			"api",
			fmt.Sprintf("repos/%s/%s/milestones", owner, name),
			"--method", "POST",
			"--header", "Content-Type: application/json",
			"--input", "-",
		)
	case ChangeUpdate:
		// Need milestone number — fetch current milestones to find it
		number, numErr := p.findMilestoneNumber(ctx, owner, name, c.Field)
		if numErr != nil {
			return fmt.Errorf("resolve milestone number for %q: %w", c.Field, numErr)
		}
		_, err = p.runner.RunWithStdin(ctx, body,
			"api",
			fmt.Sprintf("repos/%s/%s/milestones/%d", owner, name, number),
			"--method", "PATCH",
			"--header", "Content-Type: application/json",
			"--input", "-",
		)
	}

	return wrapError(err, fullName, "milestone:"+c.Field)
}

func (p *Processor) findMilestoneNumber(ctx context.Context, owner, name, title string) (int, error) {
	out, err := p.runner.Run(ctx,
		"api",
		fmt.Sprintf("repos/%s/%s/milestones?state=all&per_page=100", owner, name),
		"--paginate",
	)
	if err != nil {
		return 0, err
	}

	var milestones []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(out, &milestones); err != nil {
		return 0, err
	}

	for _, m := range milestones {
		if m.Title == title {
			return m.Number, nil
		}
	}
	return 0, fmt.Errorf("milestone %q not found", title)
}

func (p *Processor) applyActions(ctx context.Context, c Change, repo *manifest.Repository) error {
	a := repo.Spec.Actions
	if a == nil {
		return nil
	}
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	switch {
	case c.Field == "enabled" || c.Field == "allowed_actions" || c.Field == "sha_pinning_required":
		return p.applyActionsPermissions(ctx, owner, name, a)
	case c.Field == "workflow_permissions" || c.Field == "can_approve_pull_requests":
		return p.applyActionsWorkflow(ctx, owner, name, a)
	case c.Field == "fork_pr_approval":
		return p.applyActionsForkPR(ctx, owner, name, a, repo.Spec.Visibility)
	case strings.HasPrefix(c.Field, "selected_actions."):
		return p.applyActionsSelectedActions(ctx, owner, name, a)
	}
	return nil
}

func (p *Processor) applyActionsPermissions(ctx context.Context, owner, name string, a *manifest.Actions) error {
	if a.Enabled == nil {
		return nil // nothing to apply (empty actions block)
	}
	// GitHub API requires "enabled" in every PUT to this endpoint.
	// Validation ensures enabled is always set when other actions fields are present.
	payload := map[string]any{
		"enabled": *a.Enabled,
	}
	if a.AllowedActions != nil {
		payload["allowed_actions"] = *a.AllowedActions
	}
	if a.SHAPinningRequired != nil {
		payload["sha_pinning_required"] = *a.SHAPinningRequired
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api",
		fmt.Sprintf("repos/%s/%s/actions/permissions", owner, name),
		"--method", "PUT",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, owner+"/"+name, "actions.permissions")
}

func (p *Processor) applyActionsWorkflow(ctx context.Context, owner, name string, a *manifest.Actions) error {
	payload := map[string]any{}
	if a.WorkflowPermissions != nil {
		payload["default_workflow_permissions"] = *a.WorkflowPermissions
	}
	if a.CanApprovePullRequests != nil {
		payload["can_approve_pull_request_reviews"] = *a.CanApprovePullRequests
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name),
		"--method", "PUT",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, owner+"/"+name, "actions.workflow")
}

func (p *Processor) applyActionsSelectedActions(ctx context.Context, owner, name string, a *manifest.Actions) error {
	if a.SelectedActions == nil {
		return nil
	}
	sa := a.SelectedActions
	payload := map[string]any{}
	if sa.GithubOwnedAllowed != nil {
		payload["github_owned_allowed"] = *sa.GithubOwnedAllowed
	}
	if sa.VerifiedAllowed != nil {
		payload["verified_allowed"] = *sa.VerifiedAllowed
	}
	if sa.PatternsAllowed != nil {
		payload["patterns_allowed"] = sa.PatternsAllowed
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/selected-actions", owner, name),
		"--method", "PUT",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, owner+"/"+name, "actions.selected_actions")
}

func (p *Processor) applyActionsForkPR(ctx context.Context, owner, name string, a *manifest.Actions, visibility *string) error {
	if a.ForkPRApproval == nil {
		return nil
	}
	if visibility != nil && *visibility == manifest.VisibilityPrivate {
		return fmt.Errorf("actions.fork_pr_approval is not supported for private repositories")
	}
	payload := map[string]any{
		"approval_policy": *a.ForkPRApproval,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = p.runner.RunWithStdin(ctx, body,
		"api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/fork-pr-contributor-approval", owner, name),
		"--method", "PUT",
		"--header", "Content-Type: application/json",
		"--input", "-",
	)
	return wrapError(err, owner+"/"+name, "actions.fork_pr_approval")
}

func wrapError(err error, repo, field string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gh.ErrNotFound) {
		return fmt.Errorf("%s not found", repo)
	}
	if errors.Is(err, gh.ErrForbidden) {
		return fmt.Errorf("no permission to edit %s: check token scopes", repo)
	}
	if errors.Is(err, gh.ErrValidation) {
		return fmt.Errorf("validation failed for %s %s: %w", repo, field, err)
	}
	return fmt.Errorf("update %s %s: %w", repo, field, err)
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
