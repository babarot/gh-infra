package repository

import (
	"fmt"
	"sort"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// DiffOptions controls diff behavior.
type DiffOptions struct {
	ForceSecrets bool              // Always re-set existing secrets
	Resolver     *manifest.Resolver // Name resolver for rulesets (optional; nil = skip resolution)
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

	var changes []Change
	f := desired.Spec.Features

	boolDiff := func(field string, desiredVal *bool, currentVal bool) {
		if desiredVal != nil && *desiredVal != currentVal {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceRepository,
				Name:     name,
				Field:    field,
				OldValue: currentVal,
				NewValue: *desiredVal,
			})
		}
	}

	boolDiff("issues", f.Issues, current.Features.Issues)
	boolDiff("projects", f.Projects, current.Features.Projects)
	boolDiff("wiki", f.Wiki, current.Features.Wiki)
	boolDiff("discussions", f.Discussions, current.Features.Discussions)

	return changes
}

func diffMergeStrategy(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.MergeStrategy == nil {
		return nil
	}

	var changes []Change
	ms := desired.Spec.MergeStrategy

	boolDiff := func(field string, desiredVal *bool, currentVal bool) {
		if desiredVal != nil && *desiredVal != currentVal {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceRepository,
				Name:     name,
				Field:    field,
				OldValue: currentVal,
				NewValue: *desiredVal,
			})
		}
	}

	boolDiff("allow_merge_commit", ms.AllowMergeCommit, current.MergeStrategy.AllowMergeCommit)
	boolDiff("allow_squash_merge", ms.AllowSquashMerge, current.MergeStrategy.AllowSquashMerge)
	boolDiff("allow_rebase_merge", ms.AllowRebaseMerge, current.MergeStrategy.AllowRebaseMerge)
	boolDiff("auto_delete_head_branches", ms.AutoDeleteHeadBranches, current.MergeStrategy.AutoDeleteHeadBranches)

	stringDiff := func(field string, desiredVal *string, currentVal string) {
		if desiredVal != nil && *desiredVal != currentVal {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceRepository,
				Name:     name,
				Field:    field,
				OldValue: currentVal,
				NewValue: *desiredVal,
			})
		}
	}

	stringDiff("merge_commit_title", ms.MergeCommitTitle, current.MergeStrategy.MergeCommitTitle)
	stringDiff("merge_commit_message", ms.MergeCommitMessage, current.MergeStrategy.MergeCommitMessage)
	stringDiff("squash_merge_commit_title", ms.SquashMergeCommitTitle, current.MergeStrategy.SquashMergeCommitTitle)
	stringDiff("squash_merge_commit_message", ms.SquashMergeCommitMessage, current.MergeStrategy.SquashMergeCommitMessage)

	return changes
}

func diffBranchProtection(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	for _, dbp := range desired.Spec.BranchProtection {
		cbp, exists := current.BranchProtection[dbp.Pattern]
		resource := fmt.Sprintf("%s[%s]", manifest.ResourceBranchProtection, dbp.Pattern)

		if !exists {
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "branch_protection",
				NewValue: dbp.Pattern,
			})
			continue
		}

		if dbp.RequiredReviews != nil && *dbp.RequiredReviews != cbp.RequiredReviews {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "required_reviews",
				OldValue: cbp.RequiredReviews,
				NewValue: *dbp.RequiredReviews,
			})
		}

		if dbp.DismissStaleReviews != nil && *dbp.DismissStaleReviews != cbp.DismissStaleReviews {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "dismiss_stale_reviews",
				OldValue: cbp.DismissStaleReviews,
				NewValue: *dbp.DismissStaleReviews,
			})
		}

		if dbp.RequireCodeOwnerReviews != nil && *dbp.RequireCodeOwnerReviews != cbp.RequireCodeOwnerReviews {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "require_code_owner_reviews",
				OldValue: cbp.RequireCodeOwnerReviews,
				NewValue: *dbp.RequireCodeOwnerReviews,
			})
		}

		if dbp.EnforceAdmins != nil && *dbp.EnforceAdmins != cbp.EnforceAdmins {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "enforce_admins",
				OldValue: cbp.EnforceAdmins,
				NewValue: *dbp.EnforceAdmins,
			})
		}

		if dbp.AllowForcePushes != nil && *dbp.AllowForcePushes != cbp.AllowForcePushes {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "allow_force_pushes",
				OldValue: cbp.AllowForcePushes,
				NewValue: *dbp.AllowForcePushes,
			})
		}

		if dbp.AllowDeletions != nil && *dbp.AllowDeletions != cbp.AllowDeletions {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "allow_deletions",
				OldValue: cbp.AllowDeletions,
				NewValue: *dbp.AllowDeletions,
			})
		}

		if dbp.RequireStatusChecks != nil {
			if cbp.RequireStatusChecks == nil {
				changes = append(changes, Change{
					Type:     ChangeCreate,
					Resource: resource,
					Name:     name,
					Field:    "require_status_checks",
					NewValue: dbp.RequireStatusChecks,
				})
			} else {
				if dbp.RequireStatusChecks.Strict != cbp.RequireStatusChecks.Strict {
					changes = append(changes, Change{
						Type:     ChangeUpdate,
						Resource: resource,
						Name:     name,
						Field:    "require_status_checks.strict",
						OldValue: cbp.RequireStatusChecks.Strict,
						NewValue: dbp.RequireStatusChecks.Strict,
					})
				}
				if !stringSliceEqual(dbp.RequireStatusChecks.Contexts, cbp.RequireStatusChecks.Contexts) {
					changes = append(changes, Change{
						Type:     ChangeUpdate,
						Resource: resource,
						Name:     name,
						Field:    "require_status_checks.contexts",
						OldValue: cbp.RequireStatusChecks.Contexts,
						NewValue: dbp.RequireStatusChecks.Contexts,
					})
				}
			}
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
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "ruleset",
				NewValue: drs.Name,
			})
			continue
		}

		// enforcement
		if drs.Enforcement != nil && *drs.Enforcement != crs.Enforcement {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "enforcement",
				OldValue: crs.Enforcement,
				NewValue: *drs.Enforcement,
			})
		}

		// target
		if drs.Target != nil && *drs.Target != crs.Target {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "target",
				OldValue: crs.Target,
				NewValue: *drs.Target,
			})
		}

		// bypass_actors
		if !rulesetBypassActorsEqual(drs.BypassActors, crs.BypassActors, resolver) {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "bypass_actors",
				OldValue: fmt.Sprintf("%d actors", len(crs.BypassActors)),
				NewValue: fmt.Sprintf("%d actors", len(drs.BypassActors)),
			})
		}

		// conditions
		if !rulesetConditionsEqual(drs.Conditions, crs.Conditions) {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: resource,
				Name:     name,
				Field:    "conditions",
				OldValue: rulesetConditionsSummary(crs.Conditions),
				NewValue: rulesetConditionsSummary2(drs.Conditions),
			})
		}

		// toggle rules
		rulesetBoolDiff := func(field string, desired *bool, current bool) {
			if desired != nil && *desired != current {
				changes = append(changes, Change{
					Type:     ChangeUpdate,
					Resource: resource,
					Name:     name,
					Field:    field,
					OldValue: current,
					NewValue: *desired,
				})
			}
		}
		rulesetBoolDiff("rules.non_fast_forward", drs.Rules.NonFastForward, crs.Rules.NonFastForward)
		rulesetBoolDiff("rules.deletion", drs.Rules.Deletion, crs.Rules.Deletion)
		rulesetBoolDiff("rules.creation", drs.Rules.Creation, crs.Rules.Creation)
		rulesetBoolDiff("rules.required_linear_history", drs.Rules.RequiredLinearHistory, crs.Rules.RequiredLinearHistory)
		rulesetBoolDiff("rules.required_signatures", drs.Rules.RequiredSignatures, crs.Rules.RequiredSignatures)

		// pull_request rule
		if drs.Rules.PullRequest != nil {
			if crs.Rules.PullRequest == nil {
				changes = append(changes, Change{
					Type:     ChangeCreate,
					Resource: resource,
					Name:     name,
					Field:    "rules.pull_request",
					NewValue: "enabled",
				})
			} else {
				pr := drs.Rules.PullRequest
				cpr := crs.Rules.PullRequest
				if pr.RequiredApprovingReviewCount != nil && *pr.RequiredApprovingReviewCount != cpr.RequiredApprovingReviewCount {
					changes = append(changes, Change{
						Type:     ChangeUpdate,
						Resource: resource,
						Name:     name,
						Field:    "rules.pull_request.required_approving_review_count",
						OldValue: cpr.RequiredApprovingReviewCount,
						NewValue: *pr.RequiredApprovingReviewCount,
					})
				}
				prBoolDiff := func(field string, desired *bool, current bool) {
					if desired != nil && *desired != current {
						changes = append(changes, Change{
							Type:     ChangeUpdate,
							Resource: resource,
							Name:     name,
							Field:    "rules.pull_request." + field,
							OldValue: current,
							NewValue: *desired,
						})
					}
				}
				prBoolDiff("dismiss_stale_reviews_on_push", pr.DismissStaleReviewsOnPush, cpr.DismissStaleReviewsOnPush)
				prBoolDiff("require_code_owner_review", pr.RequireCodeOwnerReview, cpr.RequireCodeOwnerReview)
				prBoolDiff("require_last_push_approval", pr.RequireLastPushApproval, cpr.RequireLastPushApproval)
				prBoolDiff("required_review_thread_resolution", pr.RequiredReviewThreadResolution, cpr.RequiredReviewThreadResolution)
			}
		}

		// required_status_checks rule
		if drs.Rules.RequiredStatusChecks != nil {
			if crs.Rules.RequiredStatusChecks == nil {
				changes = append(changes, Change{
					Type:     ChangeCreate,
					Resource: resource,
					Name:     name,
					Field:    "rules.required_status_checks",
					NewValue: "enabled",
				})
			} else {
				sc := drs.Rules.RequiredStatusChecks
				csc := crs.Rules.RequiredStatusChecks
				if sc.StrictRequiredStatusChecksPolicy != nil && *sc.StrictRequiredStatusChecksPolicy != csc.StrictRequiredStatusChecksPolicy {
					changes = append(changes, Change{
						Type:     ChangeUpdate,
						Resource: resource,
						Name:     name,
						Field:    "rules.required_status_checks.strict",
						OldValue: csc.StrictRequiredStatusChecksPolicy,
						NewValue: *sc.StrictRequiredStatusChecksPolicy,
					})
				}
				if !rulesetStatusChecksEqual(sc.Contexts, csc.Contexts, resolver) {
					changes = append(changes, Change{
						Type:     ChangeUpdate,
						Resource: resource,
						Name:     name,
						Field:    "rules.required_status_checks.contexts",
						OldValue: rulesetStatusCheckNames(csc.Contexts),
						NewValue: rulesetStatusCheckNames2(sc.Contexts),
					})
				}
			}
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

func rulesetConditionsSummary(c *CurrentRulesetConditions) string {
	if c == nil || c.RefName == nil {
		return "(none)"
	}
	return fmt.Sprintf("include:%v exclude:%v", c.RefName.Include, c.RefName.Exclude)
}

func rulesetConditionsSummary2(c *manifest.RulesetConditions) string {
	if c == nil || c.RefName == nil {
		return "(none)"
	}
	return fmt.Sprintf("include:%v exclude:%v", c.RefName.Include, c.RefName.Exclude)
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

func rulesetStatusCheckNames(checks []CurrentRulesetStatusCheck) []string {
	var names []string
	for _, c := range checks {
		names = append(names, c.Context)
	}
	return names
}

func rulesetStatusCheckNames2(checks []manifest.RulesetStatusCheck) []string {
	var names []string
	for _, c := range checks {
		names = append(names, c.Context)
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
