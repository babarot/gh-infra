package importer

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/yamlpatch"
)

func PlanRepository(repos []*manifest.RepositoryDocument, githubState *repository.CurrentState, imported *manifest.Repository, resolver *manifest.Resolver, readManifestBytes func(string) error, manifestBytes map[string][]byte) (RepoPlan, error) {
	plan := RepoPlan{ManifestEdits: make(map[string][]byte)}

	for _, repo := range repos {
		diffOpts := repository.DiffOptions{Resolver: resolver}
		changes := repository.Diff(repo.Resource, githubState, diffOpts)
		plan.Changes = append(plan.Changes, repository.ReverseChanges(changes)...)
		if !repository.HasRealChanges(changes) {
			continue
		}

		if err := readManifestBytes(repo.SourcePath); err != nil {
			return RepoPlan{}, err
		}
		data := manifestBytes[repo.SourcePath]
		var err error
		data, err = yamlpatch.ReplaceYAMLNode(data, repo.DocIndex, "$.spec", imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update spec in %s: %w", repo.SourcePath, err)
		}
		manifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}

func PlanRepositorySet(repos []*manifest.RepositoryDocument, githubState *repository.CurrentState, imported *manifest.Repository, resolver *manifest.Resolver, readManifestBytes func(string) error, manifestBytes map[string][]byte) (RepoPlan, error) {
	plan := RepoPlan{ManifestEdits: make(map[string][]byte)}

	for _, repo := range repos {
		diffOpts := repository.DiffOptions{Resolver: resolver}
		changes := repository.Diff(repo.Resource, githubState, diffOpts)
		plan.Changes = append(plan.Changes, repository.ReverseChanges(changes)...)
		if !repository.HasRealChanges(changes) {
			continue
		}
		if repo.SetEntryIndex < 0 {
			return RepoPlan{}, fmt.Errorf("repository set entry index unavailable for %s", repo.Resource.Metadata.FullName())
		}

		if err := readManifestBytes(repo.SourcePath); err != nil {
			return RepoPlan{}, err
		}
		data := manifestBytes[repo.SourcePath]
		var err error
		data, err = yamlpatch.ReplaceYAMLNode(data, repo.DocIndex, fmt.Sprintf("$.repositories[%d].spec", repo.SetEntryIndex), imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update repository set entry in %s: %w", repo.SourcePath, err)
		}
		manifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}
