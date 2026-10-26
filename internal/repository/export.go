package repository

import (
	"context"
	"sort"

	"github.com/babarot/gh-infra/internal/manifest"
)

// ToManifest converts current state to a manifest Repository for export (import command).
// If resolver is provided, numeric IDs are reverse-resolved to human-readable names.
func ToManifest(ctx context.Context, r *CurrentState, resolver *manifest.Resolver) *manifest.Repository {
	resolver = resolver.For(r.Owner) // names belong to this repository's organization; nil stays nil
	repo := &manifest.Repository{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.KindRepository,
		Metadata: manifest.RepositoryMetadata{
			Name:  r.Name,
			Owner: r.Owner,
		},
		Spec: manifest.RepositorySpec{
			Description:         manifest.Ptr(r.Description),
			Visibility:          manifest.Ptr(r.Visibility),
			Archived:            manifest.Ptr(r.Archived),
			Topics:              r.Topics,
			ReleaseImmutability: manifest.Ptr(r.ReleaseImmutability),
			Security: &manifest.Security{
				VulnerabilityAlerts:           manifest.Ptr(r.Security.VulnerabilityAlerts),
				AutomatedSecurityFixes:        manifest.Ptr(r.Security.AutomatedSecurityFixes),
				PrivateVulnerabilityReporting: manifest.Ptr(r.Security.PrivateVulnerabilityReporting),
			},
			Features: exportFeatures(r),
			MergeStrategy: &manifest.MergeStrategy{
				AllowMergeCommit:       manifest.Ptr(r.MergeStrategy.AllowMergeCommit),
				AllowSquashMerge:       manifest.Ptr(r.MergeStrategy.AllowSquashMerge),
				AllowRebaseMerge:       manifest.Ptr(r.MergeStrategy.AllowRebaseMerge),
				AllowAutoMerge:         manifest.Ptr(r.MergeStrategy.AllowAutoMerge),
				AutoDeleteHeadBranches: manifest.Ptr(r.MergeStrategy.AutoDeleteHeadBranches),
				// Empty when GET /repos could not be read: not a value a
				// manifest accepts, and plan does not compare an omitted one.
				MergeCommitTitle:         nonEmpty(r.MergeStrategy.MergeCommitTitle),
				MergeCommitMessage:       nonEmpty(r.MergeStrategy.MergeCommitMessage),
				SquashMergeCommitTitle:   nonEmpty(r.MergeStrategy.SquashMergeCommitTitle),
				SquashMergeCommitMessage: nonEmpty(r.MergeStrategy.SquashMergeCommitMessage),
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
			RestrictPushes:          manifest.Ptr(bp.RestrictPushes),
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
		if rs.Rules.Update == nil {
			mrs.Rules.Update = &manifest.RulesetUpdate{Enabled: manifest.Ptr(false)}
		} else {
			mrs.Rules.Update = &manifest.RulesetUpdate{
				Enabled:             manifest.Ptr(true),
				AllowsFetchAndMerge: rs.Rules.Update.AllowsFetchAndMerge,
			}
		}
		for _, ba := range rs.BypassActors {
			if resolver != nil {
				mrs.BypassActors = append(mrs.BypassActors, resolver.ReverseBypassActor(ctx, ba.ActorID, ba.ActorType, ba.BypassMode, r.Name))
			} else {
				// Fallback: use role name for known IDs, raw format otherwise
				mrs.BypassActors = append(mrs.BypassActors, manifest.RulesetBypassActor{
					Role:       manifest.RoleNameFromID(ba.ActorID, ba.ActorType),
					BypassMode: ba.BypassMode,
				})
			}
		}
		// A ruleset targeting no branch has an empty include, which a
		// manifest rejects; leave conditions out (plan does not compare them).
		if rs.Conditions != nil && rs.Conditions.RefName != nil && len(rs.Conditions.RefName.Include) > 0 {
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
					sc.Contexts = append(sc.Contexts, resolver.ReverseStatusCheck(ctx, c.Context, c.IntegrationID, r.Name))
				} else {
					sc.Contexts = append(sc.Contexts, manifest.RulesetStatusCheck{Context: c.Context})
				}
			}
			mrs.Rules.RequiredStatusChecks = sc
		}
		repo.Spec.Rulesets = append(repo.Spec.Rulesets, mrs)
	}

	varNames := make([]string, 0, len(r.Variables))
	for name := range r.Variables {
		varNames = append(varNames, name)
	}
	sort.Strings(varNames)
	for _, name := range varNames {
		repo.Spec.Variables = append(repo.Spec.Variables, manifest.Variable{
			Name:  name,
			Value: r.Variables[name],
		})
	}

	repo.Spec.Environments = exportEnvironments(r.Environments)

	labelNames := make([]string, 0, len(r.Labels))
	for name := range r.Labels {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	for _, name := range labelNames {
		label := r.Labels[name]
		ml := manifest.Label{Name: label.Name, Color: label.Color}
		if label.Description != "" {
			ml.Description = manifest.Ptr(label.Description)
		}
		repo.Spec.Labels = append(repo.Spec.Labels, ml)
	}

	msNames := make([]string, 0, len(r.Milestones))
	for title := range r.Milestones {
		msNames = append(msNames, title)
	}
	sort.Strings(msNames)
	for _, title := range msNames {
		ms := r.Milestones[title]
		m := manifest.Milestone{
			Title: ms.Title,
			State: manifest.Ptr(ms.State),
		}
		if ms.Description != "" {
			m.Description = manifest.Ptr(ms.Description)
		}
		if ms.DueOn != "" {
			m.DueOn = manifest.Ptr(ms.DueOn)
		}
		repo.Spec.Milestones = append(repo.Spec.Milestones, m)
	}

	// Actions
	if r.Actions.Enabled || r.Actions.AllowedActions != "" || r.Actions.SHAPinningRequired {
		actions := &manifest.Actions{
			Enabled:                manifest.Ptr(r.Actions.Enabled),
			SHAPinningRequired:     manifest.Ptr(r.Actions.SHAPinningRequired),
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

func exportFeatures(r *CurrentState) *manifest.Features {
	pr := &manifest.PullRequests{
		Enabled: manifest.Ptr(r.Features.PullRequests),
	}
	// The creation policy only applies, and is only written to YAML, while
	// pull requests are enabled.
	if r.Features.PullRequests && r.Features.PullRequestCreation != "" && r.Features.PullRequestCreation != manifest.PullRequestCreationAll {
		pr.Creation = manifest.Ptr(r.Features.PullRequestCreation)
	}
	return &manifest.Features{
		Issues:       manifest.Ptr(r.Features.Issues),
		Projects:     manifest.Ptr(r.Features.Projects),
		Wiki:         manifest.Ptr(r.Features.Wiki),
		Discussions:  manifest.Ptr(r.Features.Discussions),
		PullRequests: pr,
	}
}

// nonEmpty returns a pointer to s, or nil for "".
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// exportEnvironments writes every environment, including the ones GitHub
// made (github-pages): gh-infra never deletes one, so writing them does no
// harm, and leaving them out by name would not last. Secrets are not
// written, as their values cannot be read; prevent_self_review is written
// only with reviewers, whose rule holds it.
func exportEnvironments(envs map[string]*CurrentEnvironment) []manifest.Environment {
	keys := make([]string, 0, len(envs))
	for k := range envs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []manifest.Environment
	for _, k := range keys {
		e := envs[k]
		env := manifest.Environment{
			Name:            e.Name,
			WaitTimer:       manifest.Ptr(e.WaitTimer),
			CanAdminsBypass: manifest.Ptr(e.CanAdminsBypass),
			Reviewers:       currentReviewers(e.Reviewers),
			ReviewersSet:    true,
		}
		// A rule GitHub left with no reviewers cannot take
		// prevent_self_review.
		if e.HasReviewersRule && len(e.Reviewers) > 0 {
			env.PreventSelfReview = manifest.Ptr(e.PreventSelfReview)
		}
		policy := &manifest.DeploymentBranchPolicy{}
		switch e.BranchPolicy {
		case manifest.DeploymentBranchesProtected:
			policy.ProtectedBranches = manifest.Ptr(true)
		case manifest.DeploymentBranchesCustom:
			policy.CustomBranchPoliciesSet = true
			policy.CustomBranchPolicies = []manifest.DeploymentBranchRule{}
			for _, r := range e.BranchRules {
				rule := manifest.DeploymentBranchRule{Name: r.Name}
				if r.Type == manifest.DeploymentBranchTypeTag {
					rule.Type = r.Type
				}
				policy.CustomBranchPolicies = append(policy.CustomBranchPolicies, rule)
			}
		}
		env.DeploymentBranchPolicy = policy
		names := make([]string, 0, len(e.Variables))
		for n := range e.Variables {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			env.Variables = append(env.Variables, manifest.Variable{Name: n, Value: e.Variables[n]})
		}
		out = append(out, env)
	}
	return out
}
