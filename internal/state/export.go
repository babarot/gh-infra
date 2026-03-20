package state

import (
	"github.com/babarot/gh-infra/internal/manifest"
)

// ToManifest converts current state to a manifest Repository for export (import command).
func ToManifest(r *Repository) *manifest.Repository {
	repo := &manifest.Repository{
		APIVersion: "gh-infra/v1",
		Kind:       "Repository",
		Metadata: manifest.RepositoryMetadata{
			Name:  r.Name,
			Owner: r.Owner,
		},
		Spec: manifest.RepositorySpec{
			Description: manifest.StringPtr(r.Description),
			Visibility:  manifest.StringPtr(r.Visibility),
			Topics:      r.Topics,
			Features: &manifest.Features{
				Issues:                 manifest.BoolPtr(r.Features.Issues),
				Projects:               manifest.BoolPtr(r.Features.Projects),
				Wiki:                   manifest.BoolPtr(r.Features.Wiki),
				Discussions:            manifest.BoolPtr(r.Features.Discussions),
				MergeCommit:            manifest.BoolPtr(r.Features.MergeCommit),
				SquashMerge:            manifest.BoolPtr(r.Features.SquashMerge),
				RebaseMerge:            manifest.BoolPtr(r.Features.RebaseMerge),
				AutoDeleteHeadBranches: manifest.BoolPtr(r.Features.AutoDeleteHeadBranches),
			},
		},
	}

	if r.Homepage != "" {
		repo.Spec.Homepage = manifest.StringPtr(r.Homepage)
	}

	for _, bp := range r.BranchProtection {
		mbp := manifest.BranchProtection{
			Pattern:                 bp.Pattern,
			RequiredReviews:         manifest.IntPtr(bp.RequiredReviews),
			DismissStaleReviews:     manifest.BoolPtr(bp.DismissStaleReviews),
			RequireCodeOwnerReviews: manifest.BoolPtr(bp.RequireCodeOwnerReviews),
			EnforceAdmins:           manifest.BoolPtr(bp.EnforceAdmins),
			AllowForcePushes:        manifest.BoolPtr(bp.AllowForcePushes),
			AllowDeletions:          manifest.BoolPtr(bp.AllowDeletions),
		}
		if bp.RequireStatusChecks != nil {
			mbp.RequireStatusChecks = &manifest.StatusChecks{
				Strict:   bp.RequireStatusChecks.Strict,
				Contexts: bp.RequireStatusChecks.Contexts,
			}
		}
		repo.Spec.BranchProtection = append(repo.Spec.BranchProtection, mbp)
	}

	for name, value := range r.Variables {
		repo.Spec.Variables = append(repo.Spec.Variables, manifest.Variable{
			Name:  name,
			Value: value,
		})
	}

	return repo
}
