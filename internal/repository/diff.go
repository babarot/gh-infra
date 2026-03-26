package repository

import (
	"fmt"
	"sort"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// DiffOptions controls diff behavior.
type DiffOptions struct {
	ForceSecrets bool               // Always re-set existing secrets
	Resolver     *manifest.Resolver // Name resolver for rulesets (optional; nil = skip resolution)
}

// appendIfChanged appends a Change if desired differs from current.
func appendIfChanged[T comparable](changes *[]Change, field string, desired *T, current T) {
	if desired != nil && *desired != current {
		*changes = append(*changes, Change{
			Type:     ChangeUpdate,
			Field:    field,
			OldValue: current,
			NewValue: *desired,
		})
	}
}

// Diff compares desired state with current state and returns changes.
// If the repository does not exist (current.IsNew), a single ChangeCreate is returned.
func Diff(desired *manifest.Repository, current *CurrentState, opts ...DiffOptions) []Change {
	var opt DiffOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	name := desired.Metadata.FullName()

	if current.IsNew {
		return []Change{{
			Type:     ChangeCreate,
			Resource: manifest.ResourceRepository,
			Name:     name,
			Field:    "repository",
			NewValue: name,
		}}
	}

	var changes []Change
	changes = append(changes, diffRepoSettings(name, desired, current)...)
	changes = append(changes, diffFeatures(name, desired, current)...)
	changes = append(changes, diffMergeStrategy(name, desired, current)...)
	changes = append(changes, diffBranchProtection(name, desired, current)...)
	changes = append(changes, diffRulesets(name, desired, current, opt.Resolver)...)
	changes = append(changes, diffSecrets(name, desired, current, opt.ForceSecrets)...)
	changes = append(changes, diffVariables(name, desired, current)...)
	changes = append(changes, diffActions(name, desired, current)...)

	return changes
}

func diffRepoSettings(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	if desired.Spec.Description != nil && *desired.Spec.Description != current.Description {
		changes = append(changes, Change{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceRepository,
			Name:     name,
			Field:    "description",
			OldValue: current.Description,
			NewValue: *desired.Spec.Description,
		})
	}

	if desired.Spec.Homepage != nil && *desired.Spec.Homepage != current.Homepage {
		changes = append(changes, Change{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceRepository,
			Name:     name,
			Field:    "homepage",
			OldValue: current.Homepage,
			NewValue: *desired.Spec.Homepage,
		})
	}

	if desired.Spec.Visibility != nil && *desired.Spec.Visibility != current.Visibility {
		changes = append(changes, Change{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceRepository,
			Name:     name,
			Field:    "visibility",
			OldValue: current.Visibility,
			NewValue: *desired.Spec.Visibility,
		})
	}

	if desired.Spec.Archived != nil && *desired.Spec.Archived != current.Archived {
		changes = append(changes, Change{
			Type:     ChangeUpdate,
			Resource: manifest.ResourceRepository,
			Name:     name,
			Field:    "archived",
			OldValue: current.Archived,
			NewValue: *desired.Spec.Archived,
		})
	}

	if len(desired.Spec.Topics) > 0 || len(current.Topics) > 0 {
		if !stringSliceEqual(desired.Spec.Topics, current.Topics) {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceRepository,
				Name:     name,
				Field:    "topics",
				OldValue: current.Topics,
				NewValue: desired.Spec.Topics,
			})
		}
	}

	return changes
}

func diffFeatures(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.Features == nil {
		return nil
	}

	var fieldChanges []Change
	f := desired.Spec.Features

	appendIfChanged(&fieldChanges, "issues", f.Issues, current.Features.Issues)
	appendIfChanged(&fieldChanges, "projects", f.Projects, current.Features.Projects)
	appendIfChanged(&fieldChanges, "wiki", f.Wiki, current.Features.Wiki)
	appendIfChanged(&fieldChanges, "discussions", f.Discussions, current.Features.Discussions)

	if len(fieldChanges) == 0 {
		return nil
	}

	return []Change{{
		Type:     ChangeUpdate,
		Resource: manifest.ResourceRepository,
		Name:     name,
		Field:    "features",
		Children: fieldChanges,
	}}
}

func diffMergeStrategy(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.MergeStrategy == nil {
		return nil
	}

	var fieldChanges []Change
	ms := desired.Spec.MergeStrategy

	appendIfChanged(&fieldChanges, "allow_merge_commit", ms.AllowMergeCommit, current.MergeStrategy.AllowMergeCommit)
	appendIfChanged(&fieldChanges, "allow_squash_merge", ms.AllowSquashMerge, current.MergeStrategy.AllowSquashMerge)
	appendIfChanged(&fieldChanges, "allow_rebase_merge", ms.AllowRebaseMerge, current.MergeStrategy.AllowRebaseMerge)
	appendIfChanged(&fieldChanges, "auto_delete_head_branches", ms.AutoDeleteHeadBranches, current.MergeStrategy.AutoDeleteHeadBranches)

	appendIfChanged(&fieldChanges, "merge_commit_title", ms.MergeCommitTitle, current.MergeStrategy.MergeCommitTitle)
	appendIfChanged(&fieldChanges, "merge_commit_message", ms.MergeCommitMessage, current.MergeStrategy.MergeCommitMessage)
	appendIfChanged(&fieldChanges, "squash_merge_commit_title", ms.SquashMergeCommitTitle, current.MergeStrategy.SquashMergeCommitTitle)
	appendIfChanged(&fieldChanges, "squash_merge_commit_message", ms.SquashMergeCommitMessage, current.MergeStrategy.SquashMergeCommitMessage)

	if len(fieldChanges) == 0 {
		return nil
	}

	return []Change{{
		Type:     ChangeUpdate,
		Resource: manifest.ResourceRepository,
		Name:     name,
		Field:    "merge_strategy",
		Children: fieldChanges,
	}}
}

func diffBranchProtection(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	for _, dbp := range desired.Spec.BranchProtection {
		cbp, exists := current.BranchProtection[dbp.Pattern]
		resource := fmt.Sprintf("%s[%s]", manifest.ResourceBranchProtection, dbp.Pattern)

		if !exists {
			parent := Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "branch_protection",
				NewValue: dbp.Pattern,
				Children: []Change{
					{Type: ChangeCreate, Field: "pattern", NewValue: dbp.Pattern},
				},
			}
			if dbp.RequiredReviews != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "required_reviews", NewValue: *dbp.RequiredReviews,
				})
			}
			if dbp.DismissStaleReviews != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "dismiss_stale_reviews", NewValue: *dbp.DismissStaleReviews,
				})
			}
			if dbp.RequireCodeOwnerReviews != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "require_code_owner_reviews", NewValue: *dbp.RequireCodeOwnerReviews,
				})
			}
			if dbp.EnforceAdmins != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "enforce_admins", NewValue: *dbp.EnforceAdmins,
				})
			}
			if dbp.AllowForcePushes != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "allow_force_pushes", NewValue: *dbp.AllowForcePushes,
				})
			}
			if dbp.AllowDeletions != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "allow_deletions", NewValue: *dbp.AllowDeletions,
				})
			}
			if dbp.RequireStatusChecks != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "require_status_checks.strict", NewValue: dbp.RequireStatusChecks.Strict,
				})
				if len(dbp.RequireStatusChecks.Contexts) > 0 {
					parent.Children = append(parent.Children, Change{
						Type: ChangeCreate, Field: "require_status_checks.contexts", NewValue: dbp.RequireStatusChecks.Contexts,
					})
				}
			}
			changes = append(changes, parent)
			continue
		}

		var fieldChanges []Change

		if dbp.RequiredReviews != nil && *dbp.RequiredReviews != cbp.RequiredReviews {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "required_reviews",
				OldValue: cbp.RequiredReviews, NewValue: *dbp.RequiredReviews,
			})
		}

		if dbp.DismissStaleReviews != nil && *dbp.DismissStaleReviews != cbp.DismissStaleReviews {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "dismiss_stale_reviews",
				OldValue: cbp.DismissStaleReviews, NewValue: *dbp.DismissStaleReviews,
			})
		}

		if dbp.RequireCodeOwnerReviews != nil && *dbp.RequireCodeOwnerReviews != cbp.RequireCodeOwnerReviews {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "require_code_owner_reviews",
				OldValue: cbp.RequireCodeOwnerReviews, NewValue: *dbp.RequireCodeOwnerReviews,
			})
		}

		if dbp.EnforceAdmins != nil && *dbp.EnforceAdmins != cbp.EnforceAdmins {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "enforce_admins",
				OldValue: cbp.EnforceAdmins, NewValue: *dbp.EnforceAdmins,
			})
		}

		if dbp.AllowForcePushes != nil && *dbp.AllowForcePushes != cbp.AllowForcePushes {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "allow_force_pushes",
				OldValue: cbp.AllowForcePushes, NewValue: *dbp.AllowForcePushes,
			})
		}

		if dbp.AllowDeletions != nil && *dbp.AllowDeletions != cbp.AllowDeletions {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "allow_deletions",
				OldValue: cbp.AllowDeletions, NewValue: *dbp.AllowDeletions,
			})
		}

		if dbp.RequireStatusChecks != nil {
			if cbp.RequireStatusChecks == nil {
				fieldChanges = append(fieldChanges, Change{
					Type: ChangeCreate, Field: "require_status_checks.strict",
					NewValue: dbp.RequireStatusChecks.Strict,
				})
				if len(dbp.RequireStatusChecks.Contexts) > 0 {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeCreate, Field: "require_status_checks.contexts",
						NewValue: dbp.RequireStatusChecks.Contexts,
					})
				}
			} else {
				if dbp.RequireStatusChecks.Strict != cbp.RequireStatusChecks.Strict {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeUpdate, Field: "require_status_checks.strict",
						OldValue: cbp.RequireStatusChecks.Strict, NewValue: dbp.RequireStatusChecks.Strict,
					})
				}
				if !stringSliceEqual(dbp.RequireStatusChecks.Contexts, cbp.RequireStatusChecks.Contexts) {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeUpdate, Field: "require_status_checks.contexts",
						OldValue: cbp.RequireStatusChecks.Contexts, NewValue: dbp.RequireStatusChecks.Contexts,
					})
				}
			}
		}

		if len(fieldChanges) > 0 {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "branch_protection",
				NewValue: dbp.Pattern,
				Children: fieldChanges,
			})
		}
	}

	return changes
}

func diffRulesets(name string, desired *manifest.Repository, current *CurrentState, resolver *manifest.Resolver) []Change {
	var changes []Change

	for _, drs := range desired.Spec.Rulesets {
		crs, exists := current.Rulesets[drs.Name]
		resource := fmt.Sprintf("%s[%s]", manifest.ResourceRuleset, drs.Name)

		if !exists {
			parent := Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "ruleset",
				NewValue: drs.Name,
				Children: []Change{
					{Type: ChangeCreate, Field: "name", NewValue: drs.Name},
				},
			}
			if drs.Enforcement != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "enforcement", NewValue: *drs.Enforcement,
				})
			}
			if drs.Target != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "target", NewValue: *drs.Target,
				})
			}
			if drs.Rules.NonFastForward != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.non_fast_forward", NewValue: *drs.Rules.NonFastForward,
				})
			}
			if drs.Rules.Deletion != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.deletion", NewValue: *drs.Rules.Deletion,
				})
			}
			if drs.Rules.Creation != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.creation", NewValue: *drs.Rules.Creation,
				})
			}
			if drs.Rules.RequiredLinearHistory != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.required_linear_history", NewValue: *drs.Rules.RequiredLinearHistory,
				})
			}
			if drs.Rules.RequiredSignatures != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.required_signatures", NewValue: *drs.Rules.RequiredSignatures,
				})
			}
			if drs.Rules.PullRequest != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.pull_request", NewValue: "enabled",
				})
			}
			if drs.Rules.RequiredStatusChecks != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "rules.required_status_checks", NewValue: "enabled",
				})
			}
			if len(drs.BypassActors) > 0 {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "bypass_actors", NewValue: fmt.Sprintf("%d actors", len(drs.BypassActors)),
				})
			}
			if drs.Conditions != nil {
				parent.Children = append(parent.Children, Change{
					Type: ChangeCreate, Field: "conditions", NewValue: formatConditions(drs.Conditions.RefName.Include, drs.Conditions.RefName.Exclude),
				})
			}
			changes = append(changes, parent)
			continue
		}

		var fieldChanges []Change

		// enforcement
		if drs.Enforcement != nil && *drs.Enforcement != crs.Enforcement {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "enforcement",
				OldValue: crs.Enforcement, NewValue: *drs.Enforcement,
			})
		}

		// target
		if drs.Target != nil && *drs.Target != crs.Target {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "target",
				OldValue: crs.Target, NewValue: *drs.Target,
			})
		}

		// bypass_actors
		if !rulesetBypassActorsEqual(drs.BypassActors, crs.BypassActors, resolver) {
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "bypass_actors",
				OldValue: fmt.Sprintf("%d actors", len(crs.BypassActors)),
				NewValue: fmt.Sprintf("%d actors", len(drs.BypassActors)),
			})
		}

		// conditions
		if !rulesetConditionsEqual(drs.Conditions, crs.Conditions) {
			oldCond, newCond := "(none)", "(none)"
			if crs.Conditions != nil && crs.Conditions.RefName != nil {
				oldCond = formatConditions(crs.Conditions.RefName.Include, crs.Conditions.RefName.Exclude)
			}
			if drs.Conditions != nil && drs.Conditions.RefName != nil {
				newCond = formatConditions(drs.Conditions.RefName.Include, drs.Conditions.RefName.Exclude)
			}
			fieldChanges = append(fieldChanges, Change{
				Type: ChangeUpdate, Field: "conditions",
				OldValue: oldCond,
				NewValue: newCond,
			})
		}

		// toggle rules
		appendIfChanged(&fieldChanges, "rules.non_fast_forward", drs.Rules.NonFastForward, crs.Rules.NonFastForward)
		appendIfChanged(&fieldChanges, "rules.deletion", drs.Rules.Deletion, crs.Rules.Deletion)
		appendIfChanged(&fieldChanges, "rules.creation", drs.Rules.Creation, crs.Rules.Creation)
		appendIfChanged(&fieldChanges, "rules.required_linear_history", drs.Rules.RequiredLinearHistory, crs.Rules.RequiredLinearHistory)
		appendIfChanged(&fieldChanges, "rules.required_signatures", drs.Rules.RequiredSignatures, crs.Rules.RequiredSignatures)

		// pull_request rule
		if drs.Rules.PullRequest != nil {
			if crs.Rules.PullRequest == nil {
				fieldChanges = append(fieldChanges, Change{
					Type: ChangeCreate, Field: "rules.pull_request", NewValue: "enabled",
				})
			} else {
				pr := drs.Rules.PullRequest
				cpr := crs.Rules.PullRequest
				if pr.RequiredApprovingReviewCount != nil && *pr.RequiredApprovingReviewCount != cpr.RequiredApprovingReviewCount {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeUpdate, Field: "rules.pull_request.required_approving_review_count",
						OldValue: cpr.RequiredApprovingReviewCount, NewValue: *pr.RequiredApprovingReviewCount,
					})
				}
				appendIfChanged(&fieldChanges, "rules.pull_request.dismiss_stale_reviews_on_push", pr.DismissStaleReviewsOnPush, cpr.DismissStaleReviewsOnPush)
				appendIfChanged(&fieldChanges, "rules.pull_request.require_code_owner_review", pr.RequireCodeOwnerReview, cpr.RequireCodeOwnerReview)
				appendIfChanged(&fieldChanges, "rules.pull_request.require_last_push_approval", pr.RequireLastPushApproval, cpr.RequireLastPushApproval)
				appendIfChanged(&fieldChanges, "rules.pull_request.required_review_thread_resolution", pr.RequiredReviewThreadResolution, cpr.RequiredReviewThreadResolution)
			}
		}

		// required_status_checks rule
		if drs.Rules.RequiredStatusChecks != nil {
			if crs.Rules.RequiredStatusChecks == nil {
				fieldChanges = append(fieldChanges, Change{
					Type: ChangeCreate, Field: "rules.required_status_checks", NewValue: "enabled",
				})
			} else {
				sc := drs.Rules.RequiredStatusChecks
				csc := crs.Rules.RequiredStatusChecks
				if sc.StrictRequiredStatusChecksPolicy != nil && *sc.StrictRequiredStatusChecksPolicy != csc.StrictRequiredStatusChecksPolicy {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeUpdate, Field: "rules.required_status_checks.strict",
						OldValue: csc.StrictRequiredStatusChecksPolicy, NewValue: *sc.StrictRequiredStatusChecksPolicy,
					})
				}
				if !rulesetStatusChecksEqual(sc.Contexts, csc.Contexts, resolver) {
					fieldChanges = append(fieldChanges, Change{
						Type: ChangeUpdate, Field: "rules.required_status_checks.contexts",
						OldValue: statusCheckContexts(csc.Contexts), NewValue: desiredStatusCheckContexts(sc.Contexts),
					})
				}
			}
		}

		if len(fieldChanges) > 0 {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "ruleset",
				NewValue: drs.Name,
				Children: fieldChanges,
			})
		}
	}

	return changes
}

func rulesetBypassActorsEqual(desired []manifest.RulesetBypassActor, current []CurrentRulesetBypassActor, resolver *manifest.Resolver) bool {
	if len(desired) != len(current) {
		return false
	}
	// Resolve desired names to IDs for comparison
	dm := make(map[string]bool)
	if resolver != nil {
		resolved, err := resolver.ResolveBypassActors(desired)
		if err != nil {
			return false // resolution failure = not equal (will trigger update)
		}
		for _, a := range resolved {
			dm[fmt.Sprintf("%d:%s:%s", a.ActorID, a.ActorType, a.BypassMode)] = true
		}
	}
	for _, a := range current {
		// For OrganizationAdmin, ignore actor_id (GitHub returns inconsistent values)
		if a.ActorType == "OrganizationAdmin" {
			if !dm[fmt.Sprintf("%d:%s:%s", 1, a.ActorType, a.BypassMode)] {
				// Try with any actor_id
				found := false
				for k := range dm {
					if strings.Contains(k, ":OrganizationAdmin:"+a.BypassMode) {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		} else if !dm[fmt.Sprintf("%d:%s:%s", a.ActorID, a.ActorType, a.BypassMode)] {
			return false
		}
	}
	return true
}

func rulesetConditionsEqual(desired *manifest.RulesetConditions, current *CurrentRulesetConditions) bool {
	if desired == nil && current == nil {
		return true
	}
	if desired == nil || current == nil {
		return false
	}
	if desired.RefName == nil && current.RefName == nil {
		return true
	}
	if desired.RefName == nil || current.RefName == nil {
		return false
	}
	return stringSliceEqual(desired.RefName.Include, current.RefName.Include) &&
		stringSliceEqual(desired.RefName.Exclude, current.RefName.Exclude)
}

func formatConditions(include, exclude []string) string {
	return fmt.Sprintf("include:%v exclude:%v", include, exclude)
}

func rulesetStatusChecksEqual(desired []manifest.RulesetStatusCheck, current []CurrentRulesetStatusCheck, resolver *manifest.Resolver) bool {
	if len(desired) != len(current) {
		return false
	}
	dm := make(map[string]bool)
	if resolver != nil {
		resolved, err := resolver.ResolveStatusChecks(desired)
		if err != nil {
			return false
		}
		for _, c := range resolved {
			dm[fmt.Sprintf("%s:%d", c.Context, c.IntegrationID)] = true
		}
	}
	for _, c := range current {
		if !dm[fmt.Sprintf("%s:%d", c.Context, c.IntegrationID)] {
			return false
		}
	}
	return true
}

func statusCheckContexts(checks []CurrentRulesetStatusCheck) []string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Context
	}
	return names
}

func desiredStatusCheckContexts(checks []manifest.RulesetStatusCheck) []string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Context
	}
	return names
}

func diffSecrets(name string, desired *manifest.Repository, current *CurrentState, forceSecrets bool) []Change {
	var changes []Change

	currentSet := make(map[string]bool)
	for _, s := range current.Secrets {
		currentSet[s] = true
	}

	for _, ds := range desired.Spec.Secrets {
		if !currentSet[ds.Name] {
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: manifest.ResourceSecret,
				Name:     name,
				Field:    ds.Name,
				NewValue: "(new)",
			})
		}
		// Existing secrets are opaque (can't compare values), so we skip by default.
		// Use `apply --force-secrets` to always re-set all secrets.
		if forceSecrets {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceSecret,
				Name:     name,
				Field:    ds.Name,
				OldValue: "(exists)",
				NewValue: "(force update)",
			})
		}
	}

	return changes
}

func diffVariables(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	for _, dv := range desired.Spec.Variables {
		cv, exists := current.Variables[dv.Name]
		if !exists {
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: manifest.ResourceVariable,
				Name:     name,
				Field:    dv.Name,
				NewValue: dv.Value,
			})
		} else if cv != dv.Value {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceVariable,
				Name:     name,
				Field:    dv.Name,
				OldValue: cv,
				NewValue: dv.Value,
			})
		}
	}

	return changes
}

func diffActions(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.Actions == nil {
		return nil
	}

	var fieldChanges []Change
	a := desired.Spec.Actions

	// Order matters: enabled/allowed_actions must be applied before selected_actions
	appendIfChanged(&fieldChanges, "enabled", a.Enabled, current.Actions.Enabled)
	appendIfChanged(&fieldChanges, "allowed_actions", a.AllowedActions, current.Actions.AllowedActions)
	appendIfChanged(&fieldChanges, "sha_pinning_required", a.SHAPinningRequired, current.Actions.SHAPinningRequired)
	appendIfChanged(&fieldChanges, "workflow_permissions", a.WorkflowPermissions, current.Actions.WorkflowPermissions)
	appendIfChanged(&fieldChanges, "can_approve_pull_requests", a.CanApprovePullRequests, current.Actions.CanApprovePullRequests)

	// selected_actions sub-fields (only when allowed_actions == "selected")
	if a.SelectedActions != nil {
		sa := a.SelectedActions
		var currentSA CurrentSelectedActions
		if current.Actions.SelectedActions != nil {
			currentSA = *current.Actions.SelectedActions
		}
		appendIfChanged(&fieldChanges, "selected_actions.github_owned_allowed", sa.GithubOwnedAllowed, currentSA.GithubOwnedAllowed)
		appendIfChanged(&fieldChanges, "selected_actions.verified_allowed", sa.VerifiedAllowed, currentSA.VerifiedAllowed)
		if !stringSliceEqual(sa.PatternsAllowed, currentSA.PatternsAllowed) {
			fieldChanges = append(fieldChanges, Change{
				Type:     ChangeUpdate,
				Field:    "selected_actions.patterns_allowed",
				OldValue: currentSA.PatternsAllowed,
				NewValue: sa.PatternsAllowed,
			})
		}
	}

	appendIfChanged(&fieldChanges, "fork_pr_approval", a.ForkPRApproval, current.Actions.ForkPRApproval)

	if len(fieldChanges) == 0 {
		return nil
	}

	return []Change{{
		Type:     ChangeUpdate,
		Resource: manifest.ResourceActions,
		Name:     name,
		Field:    "actions",
		Children: fieldChanges,
	}}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	sa := make([]string, len(a))
	sb := make([]string, len(b))
	copy(sa, a)
	copy(sb, b)
	sort.Strings(sa)
	sort.Strings(sb)
	return strings.Join(sa, ",") == strings.Join(sb, ",")
}
