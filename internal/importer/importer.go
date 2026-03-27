package importer

import (
	"fmt"
	"os"
	"sort"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

type Target struct {
	Owner string
	Name  string
}

func (t Target) FullName() string {
	return t.Owner + "/" + t.Name
}

type Matches struct {
	Repositories   []*manifest.RepositoryDocument
	RepositorySets []*manifest.RepositoryDocument
	FileSets       []*manifest.FileSetDocument
}

type RepoPlan struct {
	Changes       []repository.Change
	ManifestEdits map[string][]byte
	UpdatedDocs   int
}

func FindMatches(parsed *manifest.ParseResult, fullName string) Matches {
	var matches Matches

	for _, repo := range parsed.RepositoryDocs {
		if repo.Resource.Metadata.FullName() != fullName {
			continue
		}
		if repo.FromSet {
			matches.RepositorySets = append(matches.RepositorySets, repo)
			continue
		}
		matches.Repositories = append(matches.Repositories, repo)
	}

	for _, fs := range parsed.FileSetDocs {
		for _, r := range fs.Resource.Spec.Repositories {
			if fs.Resource.RepoFullName(r.Name) == fullName {
				matches.FileSets = append(matches.FileSets, fs)
				break
			}
		}
	}

	return matches
}

func BuildTasks(fullName string, includeRepo, includeFiles bool) []ui.RefreshTask {
	var tasks []ui.RefreshTask
	if includeRepo {
		tasks = append(tasks, ui.RefreshTask{
			Name:      "Importing " + fullName + " (repo)",
			DoneLabel: "Imported " + fullName + " (repo)",
			FailLabel: "Failed " + fullName + " (repo)",
		})
	}
	if includeFiles {
		tasks = append(tasks, ui.RefreshTask{
			Name:      "Importing " + fullName + " (files)",
			DoneLabel: "Imported " + fullName + " (files)",
			FailLabel: "Failed " + fullName + " (files)",
		})
	}
	return tasks
}

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
		data, err = fileset.ReplaceYAMLNode(data, repo.DocIndex, "$.spec", imported.Spec)
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
		data, err = fileset.ReplaceYAMLNode(data, repo.DocIndex, fmt.Sprintf("$.repositories[%d].spec", repo.SetEntryIndex), imported.Spec)
		if err != nil {
			return RepoPlan{}, fmt.Errorf("update repository set entry in %s: %w", repo.SourcePath, err)
		}
		manifestBytes[repo.SourcePath] = data
		plan.ManifestEdits[repo.SourcePath] = data
		plan.UpdatedDocs++
	}

	return plan, nil
}

func WriteManifestEdits(manifestEdits map[string][]byte) error {
	if len(manifestEdits) == 0 {
		return nil
	}
	paths := make([]string, 0, len(manifestEdits))
	for path := range manifestEdits {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := os.WriteFile(path, manifestEdits[path], 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
