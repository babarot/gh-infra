package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// Executor applies planned changes to GitHub.
type Executor struct {
	runner gh.Runner
}

func NewExecutor(runner gh.Runner) *Executor {
	return &Executor{runner: runner}
}

// Apply executes all changes in the plan result.
func (e *Executor) Apply(changes []Change, repos []*manifest.Repository) []ApplyResult {
	// Group changes by repo name
	repoMap := make(map[string]*manifest.Repository)
	for _, r := range repos {
		repoMap[r.Metadata.FullName()] = r
	}

	var results []ApplyResult
	for _, c := range changes {
		if c.Type == ChangeNoOp {
			continue
		}
		switch c.Type {
		case ChangeCreate:
			fmt.Fprintf(os.Stderr, "  Creating %s %s...\n", c.Name, c.Field)
		case ChangeUpdate:
			fmt.Fprintf(os.Stderr, "  Updating %s %s...\n", c.Name, c.Field)
		case ChangeDelete:
			fmt.Fprintf(os.Stderr, "  Destroying %s %s...\n", c.Name, c.Field)
		}
		result := e.applyChange(c, repoMap[c.Name])
		results = append(results, result)
	}
	return results
}

type ApplyResult struct {
	Change Change
	Err    error
}

func (e *Executor) applyChange(c Change, repo *manifest.Repository) ApplyResult {
	var err error

	switch {
	case c.Resource == manifest.ResourceRepository && c.Type == ChangeCreate && c.Field == "repository":
		err = e.createRepo(repo)
	case c.Resource == manifest.ResourceRepository:
		err = e.applyRepoSetting(c, repo)
	case strings.HasPrefix(c.Resource, manifest.ResourceBranchProtection):
		err = e.applyBranchProtection(c, repo)
	case c.Resource == manifest.ResourceSecret:
		err = e.applySecret(c, repo)
	case c.Resource == manifest.ResourceVariable:
		err = e.applyVariable(c, repo)
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
			"enable-projects":       f.Projects,
			"enable-discussions":    f.Discussions,
			"enable-merge-commit":   f.MergeCommit,
			"enable-squash-merge":   f.SquashMerge,
			"enable-rebase-merge":   f.RebaseMerge,
			"delete-branch-on-merge": f.AutoDeleteHeadBranches,
		}
		for flag, val := range featureFlags {
			if val != nil {
				if err := e.toggleFeature(fullName, flag, *val); err != nil {
					return err
				}
			}
		}

		// Commit message settings
		commitFields := map[string]*string{
			"squash_merge_commit_title":   f.SquashMergeCommitTitle,
			"squash_merge_commit_message": f.SquashMergeCommitMessage,
			"merge_commit_title":          f.MergeCommitTitle,
			"merge_commit_message":        f.MergeCommitMessage,
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

	case "topics":
		return e.applyTopics(fullName, repo)

	case "issues":
		return e.toggleFeature(fullName, "enable-issues", c.NewValue.(bool))
	case "projects":
		return e.toggleFeature(fullName, "enable-projects", c.NewValue.(bool))
	case "wiki":
		return e.toggleFeature(fullName, "enable-wiki", c.NewValue.(bool))
	case "discussions":
		return e.toggleFeature(fullName, "enable-discussions", c.NewValue.(bool))
	case "merge_commit":
		return e.toggleFeature(fullName, "enable-merge-commit", c.NewValue.(bool))
	case "squash_merge":
		return e.toggleFeature(fullName, "enable-squash-merge", c.NewValue.(bool))
	case "rebase_merge":
		return e.toggleFeature(fullName, "enable-rebase-merge", c.NewValue.(bool))
	case "auto_delete_head_branches":
		return e.toggleFeature(fullName, "delete-branch-on-merge", c.NewValue.(bool))

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
	out, err := e.runner.Run("repo", "view", fullName, "--json", "repositoryTopics", "--jq", ".[].name")
	if err != nil {
		return wrapError(err, fullName, "topics")
	}

	currentTopics := make(map[string]bool)
	for _, t := range strings.Split(strings.TrimSpace(string(out)), "\n") {
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
	if strings.HasPrefix(c.Resource, manifest.ResourceBranchProtection+"[") {
		pattern = strings.TrimSuffix(strings.TrimPrefix(c.Resource, manifest.ResourceBranchProtection+"["), "]")
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

	// Build the protection payload
	payload := buildBranchProtectionPayload(bp)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal branch protection payload: %w", err)
	}

	endpoint := fmt.Sprintf("repos/%s/%s/branches/%s/protection", owner, name, pattern)
	_, err = e.runner.Run("api", endpoint, "--method", "PUT", "--input", "-",
		"--header", "Accept: application/vnd.github+json",
	)
	// The gh api command doesn't support --input - from pipe via Runner,
	// so we use a different approach with -f flags
	if err != nil {
		// Fallback: use raw body via environment
		_, err = e.runner.Run("api", endpoint,
			"--method", "PUT",
			"--header", "Accept: application/vnd.github+json",
			"--raw-field", fmt.Sprintf("payload=%s", string(payloadJSON)),
		)
	}

	// Actually, gh api supports --input, but our Runner doesn't pipe stdin.
	// Use the field-based approach instead.
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
	return fmt.Errorf("update %s %s: %w", repo, field, err)
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
