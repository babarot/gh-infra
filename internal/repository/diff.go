package repository

import (
	"context"
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

// diffContext carries common fields shared across all changes within a diff function.
type diffContext struct {
	resource string
	name     string
}

// appendChanged appends an update Change if desired differs from current,
// setting Resource and Name from the diffContext.
func appendChanged[T comparable](dc diffContext, changes *[]Change, field string, desired *T, current T) {
	if desired != nil && *desired != current {
		*changes = append(*changes, Change{
			Type:     ChangeUpdate,
			Resource: dc.resource,
			Name:     dc.name,
			Field:    field,
			OldValue: current,
			NewValue: *desired,
		})
	}
}

// appendChildChanged appends an update Change for child fields (no Resource/Name).
func appendChildChanged[T comparable](changes *[]Change, field string, desired *T, current T) {
	if desired != nil && *desired != current {
		*changes = append(*changes, Change{
			Type:     ChangeUpdate,
			Field:    field,
			OldValue: current,
			NewValue: *desired,
		})
	}
}

// appendIfSet appends a ChangeCreate child if val is non-nil.
func appendIfSet[T any](children *[]Change, field string, val *T) {
	if val != nil {
		*children = append(*children, Change{
			Type: ChangeCreate, Field: field, NewValue: *val,
		})
	}
}

// group collects child changes and wraps them in a parent Change if non-empty.
func (dc diffContext) group(field string, childFn func(cc *[]Change)) []Change {
	var children []Change
	childFn(&children)
	if len(children) == 0 {
		return nil
	}
	return []Change{{
		Type:     ChangeUpdate,
		Resource: dc.resource,
		Name:     dc.name,
		Field:    field,
		Children: children,
	}}
}

// ValidateDependencies verifies cross-field dependencies between the desired
// manifest and the current GitHub state that cannot be checked by the offline
// validate step (which has no access to remote state).
//
// The "effective" state of a field is `desired` when the manifest sets it and
// `current` otherwise (omission means "leave as-is"). A dependency is violated
// when the effective state of a dependent field is incompatible.
//
// Currently checks:
//   - actions.fork_pr_approval is unsupported for private repositories.
//   - security.automated_security_fixes requires security.vulnerability_alerts
//     to be effectively true. Required by the GitHub API.
func ValidateDependencies(desired *manifest.Repository, current *CurrentState) error {
	if desired.Spec.Actions != nil && desired.Spec.Actions.ForkPRApproval != nil && effectiveVisibility(desired, current) == manifest.VisibilityPrivate {
		return fmt.Errorf("actions.fork_pr_approval is not supported for private repositories (remove actions.fork_pr_approval or make the repository public/internal)")
	}

	if current.IsNew {
		return nil
	}

	s := desired.Spec.Security
	if s == nil || s.AutomatedSecurityFixes == nil || !*s.AutomatedSecurityFixes {
		return nil
	}
	effectiveAlerts := current.Security.VulnerabilityAlerts
	if s.VulnerabilityAlerts != nil {
		effectiveAlerts = *s.VulnerabilityAlerts
	}
	if !effectiveAlerts {
		return fmt.Errorf("security.automated_security_fixes: true requires security.vulnerability_alerts to be enabled (current state is disabled and the manifest does not enable it)")
	}
	return nil
}

func effectiveVisibility(desired *manifest.Repository, current *CurrentState) string {
	if desired.Spec.Visibility != nil {
		return *desired.Spec.Visibility
	}
	if current.IsNew {
		return manifest.VisibilityPrivate
	}
	return current.Visibility
}

// Diff compares desired state with current state and returns changes.
// If the repository does not exist (current.IsNew), a single ChangeCreate is returned.
func Diff(ctx context.Context, desired *manifest.Repository, current *CurrentState, opts ...DiffOptions) []Change {
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
	changes = append(changes, diffRulesets(ctx, name, desired, current, opt.Resolver)...)
	changes = append(changes, diffSecrets(name, desired, current, opt.ForceSecrets)...)
	changes = append(changes, diffVariables(name, desired, current)...)
	changes = append(changes, diffLabels(name, desired, current, manifest.LabelSyncMode(desired.Spec.LabelSync))...)
	changes = append(changes, diffMilestones(name, desired, current)...)
	changes = append(changes, diffActions(name, desired, current)...)
	changes = append(changes, diffSecurity(name, desired, current)...)

	return changes
}

func diffSecurity(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.Security == nil {
		return nil
	}
	dc := diffContext{resource: manifest.ResourceRepository, name: name}
	s := desired.Spec.Security
	return dc.group("security", func(cc *[]Change) {
		appendChildChanged(cc, "vulnerability_alerts", s.VulnerabilityAlerts, current.Security.VulnerabilityAlerts)
		appendChildChanged(cc, "automated_security_fixes", s.AutomatedSecurityFixes, current.Security.AutomatedSecurityFixes)
		appendChildChanged(cc, "private_vulnerability_reporting", s.PrivateVulnerabilityReporting, current.Security.PrivateVulnerabilityReporting)
	})
}

func diffRepoSettings(name string, desired *manifest.Repository, current *CurrentState) []Change {
	dc := diffContext{resource: manifest.ResourceRepository, name: name}
	var changes []Change

	appendChanged(dc, &changes, "description", desired.Spec.Description, current.Description)
	appendChanged(dc, &changes, "homepage", desired.Spec.Homepage, current.Homepage)
	appendChanged(dc, &changes, "visibility", desired.Spec.Visibility, current.Visibility)
	appendChanged(dc, &changes, "archived", desired.Spec.Archived, current.Archived)
	appendChanged(dc, &changes, "release_immutability", desired.Spec.ReleaseImmutability, current.ReleaseImmutability)

	if len(desired.Spec.Topics) > 0 || len(current.Topics) > 0 {
		if !stringSliceEqual(desired.Spec.Topics, current.Topics) {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: dc.resource,
				Name:     dc.name,
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
	dc := diffContext{resource: manifest.ResourceRepository, name: name}
	f := desired.Spec.Features
	return dc.group("features", func(cc *[]Change) {
		appendChildChanged(cc, "issues", f.Issues, current.Features.Issues)
		appendChildChanged(cc, "projects", f.Projects, current.Features.Projects)
		appendChildChanged(cc, "wiki", f.Wiki, current.Features.Wiki)
		appendChildChanged(cc, "discussions", f.Discussions, current.Features.Discussions)
	})
}

func diffMergeStrategy(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.MergeStrategy == nil {
		return nil
	}
	dc := diffContext{resource: manifest.ResourceRepository, name: name}
	ms := desired.Spec.MergeStrategy
	return dc.group("merge_strategy", func(cc *[]Change) {
		appendChildChanged(cc, "allow_merge_commit", ms.AllowMergeCommit, current.MergeStrategy.AllowMergeCommit)
		appendChildChanged(cc, "allow_squash_merge", ms.AllowSquashMerge, current.MergeStrategy.AllowSquashMerge)
		appendChildChanged(cc, "allow_rebase_merge", ms.AllowRebaseMerge, current.MergeStrategy.AllowRebaseMerge)
		appendChildChanged(cc, "allow_auto_merge", ms.AllowAutoMerge, current.MergeStrategy.AllowAutoMerge)
		appendChildChanged(cc, "auto_delete_head_branches", ms.AutoDeleteHeadBranches, current.MergeStrategy.AutoDeleteHeadBranches)
		appendChildChanged(cc, "merge_commit_title", ms.MergeCommitTitle, current.MergeStrategy.MergeCommitTitle)
		appendChildChanged(cc, "merge_commit_message", ms.MergeCommitMessage, current.MergeStrategy.MergeCommitMessage)
		appendChildChanged(cc, "squash_merge_commit_title", ms.SquashMergeCommitTitle, current.MergeStrategy.SquashMergeCommitTitle)
		appendChildChanged(cc, "squash_merge_commit_message", ms.SquashMergeCommitMessage, current.MergeStrategy.SquashMergeCommitMessage)
	})
}

func diffBranchProtection(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	for _, dbp := range desired.Spec.BranchProtection {
		cbp, exists := current.BranchProtection[dbp.Pattern]
		resource := fmt.Sprintf("%s[%s]", manifest.ResourceBranchProtection, dbp.Pattern)

		if !exists {
			children := []Change{
				{Type: ChangeCreate, Field: "pattern", NewValue: dbp.Pattern},
			}
			appendIfSet(&children, "required_reviews", dbp.RequiredReviews)
			appendIfSet(&children, "dismiss_stale_reviews", dbp.DismissStaleReviews)
			appendIfSet(&children, "require_code_owner_reviews", dbp.RequireCodeOwnerReviews)
			appendIfSet(&children, "enforce_admins", dbp.EnforceAdmins)
			appendIfSet(&children, "allow_force_pushes", dbp.AllowForcePushes)
			appendIfSet(&children, "allow_deletions", dbp.AllowDeletions)
			if dbp.RequireStatusChecks != nil {
				children = append(children, Change{
					Type: ChangeCreate, Field: "require_status_checks.strict", NewValue: dbp.RequireStatusChecks.Strict,
				})
				if len(dbp.RequireStatusChecks.Contexts) > 0 {
					children = append(children, Change{
						Type: ChangeCreate, Field: "require_status_checks.contexts", NewValue: dbp.RequireStatusChecks.Contexts,
					})
				}
			}
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "branch_protection",
				NewValue: dbp.Pattern,
				Children: children,
			})
			continue
		}

		var fieldChanges []Change
		appendChildChanged(&fieldChanges, "required_reviews", dbp.RequiredReviews, cbp.RequiredReviews)
		appendChildChanged(&fieldChanges, "dismiss_stale_reviews", dbp.DismissStaleReviews, cbp.DismissStaleReviews)
		appendChildChanged(&fieldChanges, "require_code_owner_reviews", dbp.RequireCodeOwnerReviews, cbp.RequireCodeOwnerReviews)
		appendChildChanged(&fieldChanges, "enforce_admins", dbp.EnforceAdmins, cbp.EnforceAdmins)
		appendChildChanged(&fieldChanges, "allow_force_pushes", dbp.AllowForcePushes, cbp.AllowForcePushes)
		appendChildChanged(&fieldChanges, "allow_deletions", dbp.AllowDeletions, cbp.AllowDeletions)

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

func diffRulesets(ctx context.Context, name string, desired *manifest.Repository, current *CurrentState, resolver *manifest.Resolver) []Change {
	var changes []Change

	for _, drs := range desired.Spec.Rulesets {
		crs, exists := current.Rulesets[drs.Name]
		resource := fmt.Sprintf("%s[%s]", manifest.ResourceRuleset, drs.Name)

		if !exists {
			children := []Change{
				{Type: ChangeCreate, Field: "name", NewValue: drs.Name},
			}
			appendIfSet(&children, "enforcement", drs.Enforcement)
			appendIfSet(&children, "target", drs.Target)
			appendIfSet(&children, "rules.non_fast_forward", drs.Rules.NonFastForward)
			appendIfSet(&children, "rules.deletion", drs.Rules.Deletion)
			appendIfSet(&children, "rules.creation", drs.Rules.Creation)
			appendIfSet(&children, "rules.required_linear_history", drs.Rules.RequiredLinearHistory)
			appendIfSet(&children, "rules.required_signatures", drs.Rules.RequiredSignatures)
			if drs.Rules.PullRequest != nil {
				children = append(children, Change{
					Type: ChangeCreate, Field: "rules.pull_request", NewValue: "enabled",
				})
			}
			if drs.Rules.RequiredStatusChecks != nil {
				children = append(children, Change{
					Type: ChangeCreate, Field: "rules.required_status_checks", NewValue: "enabled",
				})
			}
			if len(drs.BypassActors) > 0 {
				children = append(children, Change{
					Type: ChangeCreate, Field: "bypass_actors", NewValue: fmt.Sprintf("%d actors", len(drs.BypassActors)),
				})
			}
			if drs.Conditions != nil {
				children = append(children, Change{
					Type: ChangeCreate, Field: "conditions", NewValue: formatConditions(drs.Conditions.RefName.Include, drs.Conditions.RefName.Exclude),
				})
			}
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: resource,
				Name:     name,
				Field:    "ruleset",
				NewValue: drs.Name,
				Children: children,
			})
			continue
		}

		var fieldChanges []Change

		appendChildChanged(&fieldChanges, "enforcement", drs.Enforcement, crs.Enforcement)
		appendChildChanged(&fieldChanges, "target", drs.Target, crs.Target)

		// bypass_actors
		if !rulesetBypassActorsEqual(ctx, drs.BypassActors, crs.BypassActors, resolver) {
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
				OldValue: oldCond, NewValue: newCond,
			})
		}

		// toggle rules
		appendChildChanged(&fieldChanges, "rules.non_fast_forward", drs.Rules.NonFastForward, crs.Rules.NonFastForward)
		appendChildChanged(&fieldChanges, "rules.deletion", drs.Rules.Deletion, crs.Rules.Deletion)
		appendChildChanged(&fieldChanges, "rules.creation", drs.Rules.Creation, crs.Rules.Creation)
		appendChildChanged(&fieldChanges, "rules.required_linear_history", drs.Rules.RequiredLinearHistory, crs.Rules.RequiredLinearHistory)
		appendChildChanged(&fieldChanges, "rules.required_signatures", drs.Rules.RequiredSignatures, crs.Rules.RequiredSignatures)

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
				appendChildChanged(&fieldChanges, "rules.pull_request.dismiss_stale_reviews_on_push", pr.DismissStaleReviewsOnPush, cpr.DismissStaleReviewsOnPush)
				appendChildChanged(&fieldChanges, "rules.pull_request.require_code_owner_review", pr.RequireCodeOwnerReview, cpr.RequireCodeOwnerReview)
				appendChildChanged(&fieldChanges, "rules.pull_request.require_last_push_approval", pr.RequireLastPushApproval, cpr.RequireLastPushApproval)
				appendChildChanged(&fieldChanges, "rules.pull_request.required_review_thread_resolution", pr.RequiredReviewThreadResolution, cpr.RequiredReviewThreadResolution)
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
				if !rulesetStatusChecksEqual(ctx, sc.Contexts, csc.Contexts, resolver) {
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

func rulesetBypassActorsEqual(ctx context.Context, desired []manifest.RulesetBypassActor, current []CurrentRulesetBypassActor, resolver *manifest.Resolver) bool {
	if len(desired) != len(current) {
		return false
	}
	dm := make(map[string]bool)
	if resolver != nil {
		resolved, err := resolver.ResolveBypassActors(ctx, desired)
		if err != nil {
			return false
		}
		for _, a := range resolved {
			dm[fmt.Sprintf("%d:%s:%s", a.ActorID, a.ActorType, a.BypassMode)] = true
		}
	}
	for _, a := range current {
		if a.ActorType == "OrganizationAdmin" {
			if !dm[fmt.Sprintf("%d:%s:%s", 1, a.ActorType, a.BypassMode)] {
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

func rulesetStatusChecksEqual(ctx context.Context, desired []manifest.RulesetStatusCheck, current []CurrentRulesetStatusCheck, resolver *manifest.Resolver) bool {
	if len(desired) != len(current) {
		return false
	}
	dm := make(map[string]bool)
	if resolver != nil {
		resolved, err := resolver.ResolveStatusChecks(ctx, desired)
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

func diffLabels(name string, desired *manifest.Repository, current *CurrentState, labelSync string) []Change {
	var changes []Change

	desiredSet := make(map[string]bool)
	for _, dl := range desired.Spec.Labels {
		desiredSet[dl.Name] = true
		cl, exists := current.Labels[dl.Name]
		if !exists {
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: manifest.ResourceLabel,
				Name:     name,
				Field:    dl.Name,
				NewValue: labelSummary(dl.Color, dl.Description),
			})
			continue
		}

		var children []Change
		if dl.Color != cl.Color {
			children = append(children, Change{
				Type:     ChangeUpdate,
				Field:    "color",
				OldValue: cl.Color,
				NewValue: dl.Color,
			})
		}
		if dl.Description != cl.Description {
			children = append(children, Change{
				Type:     ChangeUpdate,
				Field:    "description",
				OldValue: cl.Description,
				NewValue: dl.Description,
			})
		}
		if len(children) > 0 {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceLabel,
				Name:     name,
				Field:    dl.Name,
				Children: children,
			})
		}
	}

	// Mirror mode: delete labels not in the manifest
	if labelSync == manifest.LabelSyncMirror {
		for labelName, cl := range current.Labels {
			if !desiredSet[labelName] {
				changes = append(changes, Change{
					Type:     ChangeDelete,
					Resource: manifest.ResourceLabel,
					Name:     name,
					Field:    labelName,
					OldValue: labelSummary(cl.Color, cl.Description),
				})
			}
		}
	}

	return changes
}

func labelSummary(color, description string) string {
	if description != "" {
		return fmt.Sprintf("#%s %q", color, description)
	}
	return "#" + color
}

func diffMilestones(name string, desired *manifest.Repository, current *CurrentState) []Change {
	var changes []Change

	for _, dm := range desired.Spec.Milestones {
		desiredState := manifest.MilestoneState(dm.State)
		desiredDueOn := ""
		if dm.DueOn != nil {
			desiredDueOn = *dm.DueOn
		}

		cm, exists := current.Milestones[dm.Title]
		if !exists {
			changes = append(changes, Change{
				Type:     ChangeCreate,
				Resource: manifest.ResourceMilestone,
				Name:     name,
				Field:    dm.Title,
				NewValue: milestoneSummary(desiredState, desiredDueOn, dm.Description),
			})
			continue
		}

		var children []Change
		if desiredState != cm.State {
			children = append(children, Change{
				Type:     ChangeUpdate,
				Field:    "state",
				OldValue: cm.State,
				NewValue: desiredState,
			})
		}
		if dm.Description != cm.Description {
			children = append(children, Change{
				Type:     ChangeUpdate,
				Field:    "description",
				OldValue: cm.Description,
				NewValue: dm.Description,
			})
		}
		if desiredDueOn != cm.DueOn {
			children = append(children, Change{
				Type:     ChangeUpdate,
				Field:    "due_on",
				OldValue: cm.DueOn,
				NewValue: desiredDueOn,
			})
		}
		if len(children) > 0 {
			changes = append(changes, Change{
				Type:     ChangeUpdate,
				Resource: manifest.ResourceMilestone,
				Name:     name,
				Field:    dm.Title,
				Children: children,
			})
		}
	}

	return changes
}

func milestoneSummary(state, dueOn, description string) string {
	s := state
	if dueOn != "" {
		s += " due:" + dueOn
	}
	if description != "" {
		s += fmt.Sprintf(" %q", description)
	}
	return s
}

func diffActions(name string, desired *manifest.Repository, current *CurrentState) []Change {
	if desired.Spec.Actions == nil {
		return nil
	}
	dc := diffContext{resource: manifest.ResourceActions, name: name}
	a := desired.Spec.Actions
	return dc.group("actions", func(cc *[]Change) {
		appendChildChanged(cc, "enabled", a.Enabled, current.Actions.Enabled)
		appendChildChanged(cc, "allowed_actions", a.AllowedActions, current.Actions.AllowedActions)
		appendChildChanged(cc, "sha_pinning_required", a.SHAPinningRequired, current.Actions.SHAPinningRequired)
		appendChildChanged(cc, "workflow_permissions", a.WorkflowPermissions, current.Actions.WorkflowPermissions)
		appendChildChanged(cc, "can_approve_pull_requests", a.CanApprovePullRequests, current.Actions.CanApprovePullRequests)

		if a.SelectedActions != nil {
			sa := a.SelectedActions
			var currentSA CurrentSelectedActions
			if current.Actions.SelectedActions != nil {
				currentSA = *current.Actions.SelectedActions
			}
			appendChildChanged(cc, "selected_actions.github_owned_allowed", sa.GithubOwnedAllowed, currentSA.GithubOwnedAllowed)
			appendChildChanged(cc, "selected_actions.verified_allowed", sa.VerifiedAllowed, currentSA.VerifiedAllowed)
			if !stringSliceEqual(sa.PatternsAllowed, currentSA.PatternsAllowed) {
				*cc = append(*cc, Change{
					Type:     ChangeUpdate,
					Field:    "selected_actions.patterns_allowed",
					OldValue: currentSA.PatternsAllowed,
					NewValue: sa.PatternsAllowed,
				})
			}
		}

		appendChildChanged(cc, "fork_pr_approval", a.ForkPRApproval, current.Actions.ForkPRApproval)
	})
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
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
