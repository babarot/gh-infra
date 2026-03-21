package repository

import (
	"fmt"
	"sort"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// DiffOptions controls diff behavior.
type DiffOptions struct {
	ForceSecrets bool // Always re-set existing secrets
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
	changes = append(changes, diffBranchProtection(name, desired, current)...)
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
	boolDiff("merge_commit", f.MergeCommit, current.Features.MergeCommit)
	boolDiff("squash_merge", f.SquashMerge, current.Features.SquashMerge)
	boolDiff("rebase_merge", f.RebaseMerge, current.Features.RebaseMerge)
	boolDiff("auto_delete_head_branches", f.AutoDeleteHeadBranches, current.Features.AutoDeleteHeadBranches)

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

	stringDiff("merge_commit_title", f.MergeCommitTitle, current.Features.MergeCommitTitle)
	stringDiff("merge_commit_message", f.MergeCommitMessage, current.Features.MergeCommitMessage)
	stringDiff("squash_merge_commit_title", f.SquashMergeCommitTitle, current.Features.SquashMergeCommitTitle)
	stringDiff("squash_merge_commit_message", f.SquashMergeCommitMessage, current.Features.SquashMergeCommitMessage)

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
