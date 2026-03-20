package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/plan"
)

// Executor applies planned changes to GitHub.
type Executor struct {
	runner gh.Runner
}

func NewExecutor(runner gh.Runner) *Executor {
	return &Executor{runner: runner}
}

// Apply executes all changes in the plan result.
func (e *Executor) Apply(changes []plan.Change, repos []*manifest.Repository) []ApplyResult {
	// Group changes by repo name
	repoMap := make(map[string]*manifest.Repository)
	for _, r := range repos {
		repoMap[r.Metadata.FullName()] = r
	}

	var results []ApplyResult
	for _, c := range changes {
		if c.Type == plan.ChangeNoOp {
			continue
		}
		result := e.applyChange(c, repoMap[c.Name])
		results = append(results, result)
	}
	return results
}

type ApplyResult struct {
	Change plan.Change
	Err    error
}

func (e *Executor) applyChange(c plan.Change, repo *manifest.Repository) ApplyResult {
	var err error

	switch {
	case c.Resource == "Repository":
		err = e.applyRepoSetting(c, repo)
	case strings.HasPrefix(c.Resource, "BranchProtection"):
		err = e.applyBranchProtection(c, repo)
	case c.Resource == "Secret":
		err = e.applySecret(c, repo)
	case c.Resource == "Variable":
		err = e.applyVariable(c, repo)
	default:
		err = fmt.Errorf("unknown resource type: %s", c.Resource)
	}

	return ApplyResult{Change: c, Err: err}
}

func (e *Executor) applyRepoSetting(c plan.Change, repo *manifest.Repository) error {
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
		return e.toggleFeature(fullName, "issues", c.NewValue.(bool))
	case "projects":
		return e.toggleFeature(fullName, "projects", c.NewValue.(bool))
	case "wiki":
		return e.toggleFeature(fullName, "wiki", c.NewValue.(bool))
	case "discussions":
		return e.toggleFeature(fullName, "discussions", c.NewValue.(bool))
	case "merge_commit":
		return e.toggleFeature(fullName, "merge-commit", c.NewValue.(bool))
	case "squash_merge":
		return e.toggleFeature(fullName, "squash-merge", c.NewValue.(bool))
	case "rebase_merge":
		return e.toggleFeature(fullName, "rebase-merge", c.NewValue.(bool))
	case "auto_delete_head_branches":
		return e.toggleFeature(fullName, "delete-branch-on-merge", c.NewValue.(bool))
	}

	return nil
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

func (e *Executor) applyBranchProtection(c plan.Change, repo *manifest.Repository) error {
	owner := repo.Metadata.Owner
	name := repo.Metadata.Name

	// Find the matching branch protection rule from desired state
	var pattern string
	// Extract pattern from resource name like "BranchProtection[main]"
	if strings.HasPrefix(c.Resource, "BranchProtection[") {
		pattern = strings.TrimSuffix(strings.TrimPrefix(c.Resource, "BranchProtection["), "]")
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
		"enforce_admins":   derefBool(bp.EnforceAdmins),
		"restrictions":     nil,
		"allow_force_pushes": derefBool(bp.AllowForcePushes),
		"allow_deletions":  derefBool(bp.AllowDeletions),
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

func (e *Executor) applySecret(c plan.Change, repo *manifest.Repository) error {
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

func (e *Executor) applyVariable(c plan.Change, repo *manifest.Repository) error {
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
