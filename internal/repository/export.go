package repository

import (
	"github.com/babarot/gh-infra/internal/manifest"
)

// ToManifest converts current state to a manifest Repository for export (import command).
// If resolver is provided, numeric IDs are reverse-resolved to human-readable names.
func ToManifest(r *CurrentState, resolver *manifest.Resolver) *manifest.Repository {
	repo := &manifest.Repository{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.KindRepository,
		Metadata: manifest.RepositoryMetadata{
			Name:  r.Name,
			Owner: r.Owner,
		},
		Spec: manifest.RepositorySpec{
			Description: manifest.Ptr(r.Description),
			Visibility:  manifest.Ptr(r.Visibility),
			Archived:    manifest.Ptr(r.Archived),
			Topics:      r.Topics,
			Features: &manifest.Features{
				Issues:      manifest.Ptr(r.Features.Issues),
				Projects:    manifest.Ptr(r.Features.Projects),
				Wiki:        manifest.Ptr(r.Features.Wiki),
				Discussions: manifest.Ptr(r.Features.Discussions),
			},
			MergeStrategy: &manifest.MergeStrategy{
				AllowMergeCommit:         manifest.Ptr(r.MergeStrategy.AllowMergeCommit),
				AllowSquashMerge:         manifest.Ptr(r.MergeStrategy.AllowSquashMerge),
				AllowRebaseMerge:         manifest.Ptr(r.MergeStrategy.AllowRebaseMerge),
				AutoDeleteHeadBranches:   manifest.Ptr(r.MergeStrategy.AutoDeleteHeadBranches),
				MergeCommitTitle:         manifest.Ptr(r.MergeStrategy.MergeCommitTitle),
				MergeCommitMessage:       manifest.Ptr(r.MergeStrategy.MergeCommitMessage),
				SquashMergeCommitTitle:   manifest.Ptr(r.MergeStrategy.SquashMergeCommitTitle),
				SquashMergeCommitMessage: manifest.Ptr(r.MergeStrategy.SquashMergeCommitMessage),
			},
		},
	}

	if r.Homepage != "" {
		repo.Spec.Homepage = manifest.Ptr(r.Homepage)
	}

	for _, bp := range r.BranchProtection {
		mbp := manifest.BranchProtection{
			Pattern:                 bp.Pattern,
			RequiredReviews:         manifest.Ptr(bp.RequiredReviews),
			DismissStaleReviews:     manifest.Ptr(bp.DismissStaleReviews),
			RequireCodeOwnerReviews: manifest.Ptr(bp.RequireCodeOwnerReviews),
			EnforceAdmins:           manifest.Ptr(bp.EnforceAdmins),
			AllowForcePushes:        manifest.Ptr(bp.AllowForcePushes),
			AllowDeletions:          manifest.Ptr(bp.AllowDeletions),
		}
		if bp.RequireStatusChecks != nil {
			mbp.RequireStatusChecks = &manifest.StatusChecks{
				Strict:   bp.RequireStatusChecks.Strict,
				Contexts: bp.RequireStatusChecks.Contexts,
			}
		}
		repo.Spec.BranchProtection = append(repo.Spec.BranchProtection, mbp)
	}

	for _, rs := range r.Rulesets {
		mrs := manifest.Ruleset{
			Name:        rs.Name,
			Target:      manifest.Ptr(rs.Target),
			Enforcement: manifest.Ptr(rs.Enforcement),
			Rules: manifest.RulesetRules{
				NonFastForward:        manifest.Ptr(rs.Rules.NonFastForward),
				Deletion:              manifest.Ptr(rs.Rules.Deletion),
				Creation:              manifest.Ptr(rs.Rules.Creation),
				RequiredLinearHistory: manifest.Ptr(rs.Rules.RequiredLinearHistory),
				RequiredSignatures:    manifest.Ptr(rs.Rules.RequiredSignatures),
			},
		}
		for _, ba := range rs.BypassActors {
			if resolver != nil {
				mrs.BypassActors = append(mrs.BypassActors, resolver.ReverseBypassActor(ba.ActorID, ba.ActorType, ba.BypassMode, r.Name))
			} else {
				// Fallback: use role name for known IDs, raw format otherwise
				mrs.BypassActors = append(mrs.BypassActors, manifest.RulesetBypassActor{
					Role:       manifest.RoleNameFromID(ba.ActorID, ba.ActorType),
					BypassMode: ba.BypassMode,
				})
			}
		}
		if rs.Conditions != nil && rs.Conditions.RefName != nil {
			mrs.Conditions = &manifest.RulesetConditions{
				RefName: &manifest.RulesetRefCondition{
					Include: rs.Conditions.RefName.Include,
					Exclude: rs.Conditions.RefName.Exclude,
				},
			}
		}
		if rs.Rules.PullRequest != nil {
			mrs.Rules.PullRequest = &manifest.RulesetPullRequest{
				RequiredApprovingReviewCount:   manifest.Ptr(rs.Rules.PullRequest.RequiredApprovingReviewCount),
				DismissStaleReviewsOnPush:      manifest.Ptr(rs.Rules.PullRequest.DismissStaleReviewsOnPush),
				RequireCodeOwnerReview:         manifest.Ptr(rs.Rules.PullRequest.RequireCodeOwnerReview),
				RequireLastPushApproval:        manifest.Ptr(rs.Rules.PullRequest.RequireLastPushApproval),
				RequiredReviewThreadResolution: manifest.Ptr(rs.Rules.PullRequest.RequiredReviewThreadResolution),
			}
		}
		if rs.Rules.RequiredStatusChecks != nil {
			sc := &manifest.RulesetStatusChecks{
				StrictRequiredStatusChecksPolicy: manifest.Ptr(rs.Rules.RequiredStatusChecks.StrictRequiredStatusChecksPolicy),
			}
			for _, c := range rs.Rules.RequiredStatusChecks.Contexts {
				if resolver != nil {
					sc.Contexts = append(sc.Contexts, resolver.ReverseStatusCheck(c.Context, c.IntegrationID, r.Name))
				} else {
					sc.Contexts = append(sc.Contexts, manifest.RulesetStatusCheck{Context: c.Context})
				}
			}
			mrs.Rules.RequiredStatusChecks = sc
		}
		repo.Spec.Rulesets = append(repo.Spec.Rulesets, mrs)
	}

	for name, value := range r.Variables {
		repo.Spec.Variables = append(repo.Spec.Variables, manifest.Variable{
			Name:  name,
			Value: value,
		})
	}

	// Actions
	if r.Actions.Enabled || r.Actions.AllowedActions != "" {
		actions := &manifest.Actions{
			Enabled:                manifest.Ptr(r.Actions.Enabled),
			CanApprovePullRequests: manifest.Ptr(r.Actions.CanApprovePullRequests),
		}
		if r.Actions.AllowedActions != "" {
			actions.AllowedActions = manifest.Ptr(r.Actions.AllowedActions)
		}
		if r.Actions.WorkflowPermissions != "" {
			actions.WorkflowPermissions = manifest.Ptr(r.Actions.WorkflowPermissions)
		}
		if r.Actions.SelectedActions != nil {
			actions.SelectedActions = &manifest.SelectedActions{
				GithubOwnedAllowed: manifest.Ptr(r.Actions.SelectedActions.GithubOwnedAllowed),
				VerifiedAllowed:    manifest.Ptr(r.Actions.SelectedActions.VerifiedAllowed),
				PatternsAllowed:    r.Actions.SelectedActions.PatternsAllowed,
			}
		}
		if r.Actions.ForkPRApproval != "" {
			actions.ForkPRApproval = manifest.Ptr(r.Actions.ForkPRApproval)
		}
		repo.Spec.Actions = actions
	}

	return repo
}
