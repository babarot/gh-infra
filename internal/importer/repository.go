package importer

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

func PlanRepository(target Target, repos []*manifest.RepositoryDocument, runner gh.Runner, readManifestBytes func(string) error, manifestBytes map[string][]byte) (RepoPlan, error) {
	fetcher := repository.NewFetcher(runner)
	resolver := manifest.NewResolver(runner, target.Owner)

	githubState, err := fetcher.FetchRepository(target.Owner, target.Name)
	if err != nil {
		return RepoPlan{}, err
	}

	imported := repository.ToManifest(githubState, resolver)
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
		data, err = manifest.ReplaceYAMLNode(data, repo.DocIndex, "$.spec", imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update spec in %s: %w", repo.SourcePath, err)
		}
		manifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}

func PlanRepositorySet(target Target, repos []*manifest.RepositoryDocument, runner gh.Runner, readManifestBytes func(string) error, manifestBytes map[string][]byte) (RepoPlan, error) {
	fetcher := repository.NewFetcher(runner)
	resolver := manifest.NewResolver(runner, target.Owner)

	githubState, err := fetcher.FetchRepository(target.Owner, target.Name)
	if err != nil {
		return RepoPlan{}, err
	}

	imported := repository.ToManifest(githubState, resolver)
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
		data, err = manifest.ReplaceYAMLNode(data, repo.DocIndex, fmt.Sprintf("$.repositories[%d].spec", repo.SetEntryIndex), imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update repository set entry in %s: %w", repo.SourcePath, err)
		}
		manifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}
