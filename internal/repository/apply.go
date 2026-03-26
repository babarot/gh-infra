package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

// Executor applies planned changes to GitHub.
type Executor struct {
	runner   gh.Runner
	resolver *manifest.Resolver
}

func NewExecutor(runner gh.Runner, resolver *manifest.Resolver) *Executor {
	return &Executor{runner: runner, resolver: resolver}
}

const defaultApplyParallel = 5

// Apply executes all changes in the plan result.
// Changes are grouped by repo and applied in parallel across repos.
// Within a single repo, changes are applied sequentially to maintain ordering.
func (e *Executor) Apply(changes []Change, repos []*manifest.Repository, reporter ui.ProgressReporter) []ApplyResult {
	repoMap := make(map[string]*manifest.Repository)
	for _, r := range repos {
		repoMap[r.Metadata.FullName()] = r
	}

	groups := groupByName(changes)
	if len(groups) == 0 {
		return nil
	}

	// Apply repo groups in parallel
	allResults := parallel.Map(groups, defaultApplyParallel, func(_ int, g changeGroup) []ApplyResult {
		fields := make([]string, 0, len(g.changes))
		for _, c := range g.changes {
			fields = append(fields, c.Field)
		}
		reporter.Start(g.name, fields)

		start := time.Now()
		var results []ApplyResult
		for _, c := range g.changes {
			result := e.applyChange(c, repoMap[c.Name])
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

type ApplyResult struct {
	Change Change
	Err    error
}

func (e *Executor) applyChange(c Change, repo *manifest.Repository) ApplyResult {
	// Generic: if this change has children, expand and apply each child.
	if len(c.Children) > 0 {
		for _, child := range c.Children {
			child.Resource = c.Resource
			child.Name = c.Name
			if result := e.applyChange(child, repo); result.Err != nil {
				return ApplyResult{Change: c, Err: result.Err}
			}
		}
		return ApplyResult{Change: c}
	}

	var err error

	switch {
	case c.Resource == manifest.ResourceRepository && c.Type == ChangeCreate && c.Field == "repository":
		err = e.createRepo(repo)
	case c.Resource == manifest.ResourceRepository:
		err = e.applyRepoSetting(c, repo)
	case strings.HasPrefix(c.Resource, manifest.ResourceBranchProtection):
		err = e.applyBranchProtection(c, repo)
	case strings.HasPrefix(c.Resource, manifest.ResourceRuleset):
		err = e.applyRuleset(c, repo)
	case c.Resource == manifest.ResourceSecret:
		err = e.applySecret(c, repo)
	case c.Resource == manifest.ResourceVariable:
		err = e.applyVariable(c, repo)
	case c.Resource == manifest.ResourceActions:
		err = e.applyActions(c, repo)
	default:
		err = fmt.Errorf("unknown resource type: %s", c.Resource)
	}

	return ApplyResult{Change: c, Err: err}
}

func (e *Executor) createRepo(repo *manifest.Repository) error {
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

	_, err := e.runner.Run(args...)
	if err != nil {
		return wrapError(err, fullName, "create")
	}

	// Apply remaining settings via gh repo edit
	return e.applyAllSettings(repo)
}

func (e *Executor) applyAllSettings(repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	// Features
	if f := repo.Spec.Features; f != nil {
		featureFlags := map[string]*bool{
			"enable-projects":    f.Projects,
			"enable-discussions": f.Discussions,
		}
		for flag, val := range featureFlags {
			if val != nil {
				if err := e.toggleFeature(fullName, flag, *val); err != nil {
					return err
				}
			}
		}
	}

	// Merge strategy
	if ms := repo.Spec.MergeStrategy; ms != nil {
		mergeFlags := map[string]*bool{
			"enable-merge-commit":    ms.AllowMergeCommit,
			"enable-squash-merge":    ms.AllowSquashMerge,
			"enable-rebase-merge":    ms.AllowRebaseMerge,
			"delete-branch-on-merge": ms.AutoDeleteHeadBranches,
		}
		for flag, val := range mergeFlags {
			if val != nil {
				if err := e.toggleFeature(fullName, flag, *val); err != nil {
					return err
				}
			}
		}

		// Commit message settings
		commitFields := map[string]*string{
			"squash_merge_commit_title":   ms.SquashMergeCommitTitle,
			"squash_merge_commit_message": ms.SquashMergeCommitMessage,
			"merge_commit_title":          ms.MergeCommitTitle,
			"merge_commit_message":        ms.MergeCommitMessage,
		}
		for field, val := range commitFields {
			if val != nil {
				if err := e.updateRepoField(fullName, field, *val); err != nil {
					return err
				}
			}
		}
	}

	// Homepage
	if repo.Spec.Homepage != nil {
		if _, err := e.runner.Run("repo", "edit", fullName, "--homepage", *repo.Spec.Homepage); err != nil {
			return wrapError(err, fullName, "homepage")
		}
	}

	// Topics
	for _, t := range repo.Spec.Topics {
		if _, err := e.runner.Run("repo", "edit", fullName, "--add-topic", t); err != nil {
			return wrapError(err, fullName, "add-topic:"+t)
		}
	}

	// Actions (permissions, workflow defaults, selected actions, fork PR)
	if a := repo.Spec.Actions; a != nil && a.Enabled != nil {
		if err := e.applyActionsPermissions(owner, name, a); err != nil {
			return err
		}
		if err := e.applyActionsWorkflow(owner, name, a); err != nil {
			return err
		}
		if a.SelectedActions != nil {
			if err := e.applyActionsSelectedActions(owner, name, a); err != nil {
				return err
			}
		}
		if a.ForkPRApproval != nil {
			if err := e.applyActionsForkPR(owner, name, a); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *Executor) applyRepoSetting(c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name
	fullName := owner + "/" + name

	switch c.Field {
	case "description":
		_, err := e.runner.Run("repo", "edit", fullName, "--description", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "homepage":
		_, err := e.runner.Run("repo", "edit", fullName, "--homepage", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "visibility":
		_, err := e.runner.Run("repo", "edit", fullName, "--visibility", fmt.Sprintf("%v", c.NewValue))
		return wrapError(err, fullName, c.Field)

	case "archived":
		archived, ok := c.NewValue.(bool)
		if !ok {
			return fmt.Errorf("unexpected type for archived: %T", c.NewValue)
		}
		if archived {
			_, err := e.runner.Run("repo", "archive", fullName, "--yes")
			return wrapError(err, fullName, c.Field)
		}
		_, err := e.runner.Run("repo", "unarchive", fullName, "--yes")
		return wrapError(err, fullName, c.Field)

	case "topics":
		return e.applyTopics(fullName, repo)

	case "issues":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-issues", v)
	case "projects":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-projects", v)
	case "wiki":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-wiki", v)
	case "discussions":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-discussions", v)
	case "allow_merge_commit":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-merge-commit", v)
	case "allow_squash_merge":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-squash-merge", v)
	case "allow_rebase_merge":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "enable-rebase-merge", v)
	case "auto_delete_head_branches":
		v, _ := c.NewValue.(bool)
		return e.toggleFeature(fullName, "delete-branch-on-merge", v)

	case "merge_commit_title", "merge_commit_message", "squash_merge_commit_title", "squash_merge_commit_message":
		return e.updateRepoField(owner+"/"+name, c.Field, fmt.Sprintf("%v", c.NewValue))
	}

	return nil
}

func (e *Executor) updateRepoField(fullName, field, value string) error {
	endpoint := fmt.Sprintf("repos/%s", fullName)
	_, err := e.runner.Run("api", endpoint, "--method", "PATCH",
		"-f", fmt.Sprintf("%s=%s", field, value),
	)
	return wrapError(err, fullName, field)
}

func (e *Executor) toggleFeature(repo, flag string, enable bool) error {
	arg := fmt.Sprintf("--%s=%t", flag, enable)
	_, err := e.runner.Run("repo", "edit", repo, arg)
	return wrapError(err, repo, flag)
}

func (e *Executor) applyTopics(fullName string, repo *manifest.Repository) error {
	// Get current topics
	out, err := e.runner.Run("repo", "view", fullName, "--json", "repositoryTopics", "--jq", ".repositoryTopics[].name")
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
			if _, err := e.runner.Run("repo", "edit", fullName, "--remove-topic", t); err != nil {
				return wrapError(err, fullName, "remove-topic:"+t)
			}
		}
	}

	// Add topics not in current
	for t := range desiredTopics {
		if !currentTopics[t] {
			if _, err := e.runner.Run("repo", "edit", fullName, "--add-topic", t); err != nil {
				return wrapError(err, fullName, "add-topic:"+t)
			}
		}
	}

	return nil
}

func (e *Executor) applyBranchProtection(c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	// Find the matching branch protection rule from desired state
	var pattern string
	// Extract pattern from resource name like "BranchProtection[main]"
	if after, ok := strings.CutPrefix(c.Resource, manifest.ResourceBranchProtection+"["); ok {
		pattern = strings.TrimSuffix(after, "]")
	}

	var bp *manifest.BranchProtection
	for i := range repo.Spec.BranchProtection {
		if repo.Spec.BranchProtection[i].Pattern == pattern {
			bp = &repo.Spec.BranchProtection[i]
			break
		}
	}
	if bp == nil {
		return fmt.Errorf("branch protection rule %q not found in desired state", pattern)
	}

	// Use the field-based API approach (Runner doesn't pipe stdin).
	return e.applyBranchProtectionViaAPI(owner, name, bp)
}

func (e *Executor) applyBranchProtectionViaAPI(owner, name string, bp *manifest.BranchProtection) error {
	payload := buildBranchProtectionPayload(bp)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal branch protection: %w", err)
	}

	endpoint := fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, name, bp.Pattern)

	// Write payload to temp approach - use gh api with body
	args := []string{
		"api", endpoint,
		"--method", "PUT",
		"--header", "Accept: application/vnd.github+json",
		"--body", string(payloadJSON),
	}

	_, err = e.runner.Run(args...)
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

func (e *Executor) applyRuleset(c Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	// Extract ruleset name from resource like "Ruleset[protect-main]"
	rulesetName := strings.TrimSuffix(strings.TrimPrefix(c.Resource, manifest.ResourceRuleset+"["), "]")

	var rs *manifest.Ruleset
	for i := range repo.Spec.Rulesets {
		if repo.Spec.Rulesets[i].Name == rulesetName {
			rs = &repo.Spec.Rulesets[i]
			break
		}
	}
	if rs == nil {
		return fmt.Errorf("ruleset %q not found in desired state", rulesetName)
	}

	payload, err := buildRulesetPayload(rs, e.resolver)
	if err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ruleset payload: %w", err)
	}

	switch c.Type {
	case ChangeCreate:
		_, err = e.runner.Run("api",
			fmt.Sprintf("repos/%s/%s/rulesets", owner, name),
			"--method", "POST",
			"--header", "Accept: application/vnd.github+json",
			"--body", string(payloadJSON),
		)
		return wrapError(err, owner+"/"+name, "ruleset:"+rulesetName)

	case ChangeUpdate:
		target := "branch"
		if rs.Target != nil {
			target = *rs.Target
		}
		rulesetID, err := e.resolveRulesetID(owner, name, rulesetName, target)
		if err != nil {
			return err
		}
		_, err = e.runner.Run("api",
			fmt.Sprintf("repos/%s/%s/rulesets/%d", owner, name, rulesetID),
			"--method", "PUT",
			"--header", "Accept: application/vnd.github+json",
			"--body", string(payloadJSON),
		)
		return wrapError(err, owner+"/"+name, "ruleset:"+rulesetName)
	}

	return nil
}

func (e *Executor) resolveRulesetID(owner, name, rulesetName, target string) (int, error) {
	out, err := e.runner.Run("api", fmt.Sprintf("repos/%s/%s/rulesets", owner, name))
	if err != nil {
		return 0, fmt.Errorf("list rulesets for %s/%s: %w", owner, name, err)
	}

	var rulesets []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal(out, &rulesets); err != nil {
		return 0, fmt.Errorf("parse rulesets list for %s/%s: %w", owner, name, err)
	}

	var matches []int
	for _, rs := range rulesets {
		if rs.Name == rulesetName && rs.Target == target {
			matches = append(matches, rs.ID)
		}
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("ruleset %q (target=%s) not found in %s/%s", rulesetName, target, owner, name)
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("multiple rulesets named %q (target=%s) found in %s/%s; cannot determine which to update", rulesetName, target, owner, name)
	}
}

func buildRulesetPayload(rs *manifest.Ruleset, resolver *manifest.Resolver) (map[string]any, error) {
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
		resolved, err := resolver.ResolveBypassActors(rs.BypassActors)
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
		resolvedChecks, err := resolver.ResolveStatusChecks(sc.Contexts)
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

func (e *Executor) applySecret(c Change, repo *manifest.Repository) error {
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

	_, err := e.runner.Run("secret", "set", c.Field,
		"--repo", fullName,
		"--body", value,
	)
	return wrapError(err, fullName, "secret:"+c.Field)
}

func (e *Executor) applyVariable(c Change, repo *manifest.Repository) error {
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

	_, err := e.runner.Run("variable", "set", c.Field,
		"--repo", fullName,
		"--body", value,
	)
	return wrapError(err, fullName, "variable:"+c.Field)
}

func (e *Executor) applyActions(c Change, repo *manifest.Repository) error {
	a := repo.Spec.Actions
	if a == nil {
		return nil
	}
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	switch {
	case c.Field == "enabled" || c.Field == "allowed_actions" || c.Field == "sha_pinning_required":
		return e.applyActionsPermissions(owner, name, a)
	case c.Field == "workflow_permissions" || c.Field == "can_approve_pull_requests":
		return e.applyActionsWorkflow(owner, name, a)
	case c.Field == "fork_pr_approval":
		return e.applyActionsForkPR(owner, name, a)
	case strings.HasPrefix(c.Field, "selected_actions."):
		return e.applyActionsSelectedActions(owner, name, a)
	}
	return nil
}

func (e *Executor) applyActionsPermissions(owner, name string, a *manifest.Actions) error {
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
	_, err = e.runner.Run("api",
		fmt.Sprintf("repos/%s/%s/actions/permissions", owner, name),
		"--method", "PUT",
		"--body", string(body),
	)
	return wrapError(err, owner+"/"+name, "actions.permissions")
}

func (e *Executor) applyActionsWorkflow(owner, name string, a *manifest.Actions) error {
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
	_, err = e.runner.Run("api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/workflow", owner, name),
		"--method", "PUT",
		"--body", string(body),
	)
	return wrapError(err, owner+"/"+name, "actions.workflow")
}

func (e *Executor) applyActionsSelectedActions(owner, name string, a *manifest.Actions) error {
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
	_, err = e.runner.Run("api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/selected-actions", owner, name),
		"--method", "PUT",
		"--body", string(body),
	)
	return wrapError(err, owner+"/"+name, "actions.selected_actions")
}

func (e *Executor) applyActionsForkPR(owner, name string, a *manifest.Actions) error {
	if a.ForkPRApproval == nil {
		return nil
	}
	payload := map[string]any{
		"approval_policy": *a.ForkPRApproval,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = e.runner.Run("api",
		fmt.Sprintf("repos/%s/%s/actions/permissions/fork-pr-contributor-approval", owner, name),
		"--method", "PUT",
		"--body", string(body),
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
