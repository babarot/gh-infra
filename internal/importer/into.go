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

func PlanInto(matches Matches, target Target, runner gh.Runner, printer ui.Printer, tracker *ui.RefreshTracker) (IntoPlan, error) {
	manifestBytes := make(map[string][]byte)
	readManifestBytes := func(sourcePath string) error {
		if _, ok := manifestBytes[sourcePath]; ok {
			return nil
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("read manifest %s: %w", sourcePath, err)
		}
		manifestBytes[sourcePath] = data
		return nil
	}

	plan := IntoPlan{
		ManifestEdits: make(map[string][]byte),
		matches:       matches,
		manifestBytes: manifestBytes,
		readManifest:  readManifestBytes,
	}

	if matches.HasRepo() {
		key := "Importing " + target.FullName() + " (repo)"

		// Fetch GitHub state once for both Repository and RepositorySet planning
		fetcher := repository.NewFetcher(runner)
		resolver := manifest.NewResolver(runner, target.Owner)
		githubState, err := fetcher.FetchRepository(target.Owner, target.Name)
		if err != nil {
			return IntoPlan{}, err
		}
		imported := repository.ToManifest(githubState, resolver)

		if len(matches.Repositories) > 0 {
			repoPlan, err := PlanRepository(matches.Repositories, githubState, imported, resolver, readManifestBytes, manifestBytes)
			if err != nil {
				return IntoPlan{}, err
			}
			plan.AddRepoPlan(repoPlan)
		}
		if len(matches.RepositorySets) > 0 {
			repoPlan, err := PlanRepositorySet(matches.RepositorySets, githubState, imported, resolver, readManifestBytes, manifestBytes)
			if err != nil {
				return IntoPlan{}, err
			}
			plan.AddRepoPlan(repoPlan)
		}
		if tracker != nil {
			tracker.Done(key)
		}
	}

	if matches.HasFiles() {
		key := "Importing " + target.FullName() + " (files)"
		processor := fileset.NewProcessor(runner, printer)
		changes, err := PlanImport(processor.FetchFileContent, matches.FileSets, target.FullName())
		if err != nil {
			return IntoPlan{}, err
		}
		plan.FileChanges = changes
		if tracker != nil {
			tracker.Done(key)
		}
	}

	return plan, nil
}

func (p *IntoPlan) Apply() error {
	for _, fs := range p.matches.FileSets {
		if err := p.readManifest(fs.SourcePath); err != nil {
			return err
		}
	}
	if err := ApplyImport(p.FileChanges, p.manifestBytes); err != nil {
		return err
	}
	return WriteManifestEdits(p.ManifestEdits)
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
