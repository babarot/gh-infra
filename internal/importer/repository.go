package importer

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/yamlpatch"
)

// RepoPlanInput holds all inputs needed for PlanRepository and PlanRepositorySet.
type RepoPlanInput struct {
	Repos             []*manifest.RepositoryDocument
	GitHubState       *repository.CurrentState
	Imported          *manifest.Repository
	Resolver          *manifest.Resolver
	ReadManifestBytes func(string) error
	ManifestBytes     map[string][]byte
}

func PlanRepository(input RepoPlanInput) (RepoPlan, error) {
	plan := RepoPlan{ManifestEdits: make(map[string][]byte)}

	for _, repo := range input.Repos {
		diffOpts := repository.DiffOptions{Resolver: input.Resolver}
		changes := repository.Diff(repo.Resource, input.GitHubState, diffOpts)
		plan.Changes = append(plan.Changes, repository.ReverseChanges(changes)...)
		if !repository.HasRealChanges(changes) {
			continue
		}

		if err := input.ReadManifestBytes(repo.SourcePath); err != nil {
			return RepoPlan{}, err
		}
		data := input.ManifestBytes[repo.SourcePath]
		var err error
		data, err = yamlpatch.ReplaceYAMLNode(data, repo.DocIndex, "$.spec", input.Imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update spec in %s: %w", repo.SourcePath, err)
		}
		input.ManifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}

func PlanRepositorySet(input RepoPlanInput) (RepoPlan, error) {
	plan := RepoPlan{ManifestEdits: make(map[string][]byte)}

	for _, repo := range input.Repos {
		diffOpts := repository.DiffOptions{Resolver: input.Resolver}
		changes := repository.Diff(repo.Resource, input.GitHubState, diffOpts)
		plan.Changes = append(plan.Changes, repository.ReverseChanges(changes)...)
		if !repository.HasRealChanges(changes) {
			continue
		}
		if repo.SetEntryIndex < 0 {
			return RepoPlan{}, fmt.Errorf("repository set entry index unavailable for %s", repo.Resource.Metadata.FullName())
		}

		if err := input.ReadManifestBytes(repo.SourcePath); err != nil {
			return RepoPlan{}, err
		}
		data := input.ManifestBytes[repo.SourcePath]
		var err error
		data, err = yamlpatch.ReplaceYAMLNode(data, repo.DocIndex, fmt.Sprintf("$.repositories[%d].spec", repo.SetEntryIndex), input.Imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update repository set entry in %s: %w", repo.SourcePath, err)
		}
		input.ManifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}
