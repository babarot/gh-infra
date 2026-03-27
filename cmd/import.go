package cmd

import (
	"fmt"
	"os"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func newImportCmd() *cobra.Command {
	var into string

	cmd := &cobra.Command{
		Use:   "import <owner/repo> [owner/repo ...]",
		Short: "Import existing repository settings and files from GitHub",
		Long: `Fetch current GitHub repository settings and output them as gh-infra YAML.
Multiple repositories can be specified to import them in parallel.

With --into, import file content from GitHub back to local template sources.
Specify a path to a File/FileSet manifest or a directory containing manifests.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if into != "" {
				return runImportInto(args, into)
			}
			return runImport(args)
		},
	}

	cmd.Flags().StringVar(&into, "into", "", "Import file content into local sources via the given manifest or directory")

	return cmd
}

type importTarget struct {
	owner string
	name  string
}

func parseImportTargets(args []string) ([]importTarget, error) {
	var targets []importTarget
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target: %q (expected owner/repo)", arg)
		}
		targets = append(targets, importTarget{owner: parts[0], name: parts[1]})
	}
	return targets, nil
}

func runImport(args []string) error {
	p := ui.NewStandardPrinter()

	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}

	runner := gh.NewRunner(false)
	fetcher := repository.NewFetcher(runner)
	resolver := manifest.NewResolver(runner, targets[0].owner)

	return importRepos(p, targets, fetcher, resolver)
}

func runImportInto(args []string, searchPath string) error {
	p := ui.NewStandardPrinter()

	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}
	if !ui.IsInteractive() {
		return fmt.Errorf("import --into requires an interactive terminal")
	}

	for _, target := range targets {
		fullName := target.owner + "/" + target.name
		if err := importIntoForRepo(p, target, fullName, searchPath); err != nil {
			return err
		}
	}

	return nil
}

func importIntoForRepo(p ui.Printer, target importTarget, fullName, searchPath string) error {
	parsed, err := manifest.ParseAll(searchPath)
	if err != nil {
		return err
	}

	matches := importer.FindMatches(parsed, fullName)
	if len(matches.Repositories) == 0 && len(matches.RepositorySets) == 0 && len(matches.FileSets) == 0 {
		return fmt.Errorf("no manifests found for %s in %s", fullName, searchPath)
	}

	runner := gh.NewRunner(false)
	manifestBytes := make(map[string][]byte)
	readManifestBytes := func(sourcePath string) error {
		if _, ok := manifestBytes[sourcePath]; !ok {
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				return fmt.Errorf("read manifest %s: %w", sourcePath, err)
			}
			manifestBytes[sourcePath] = data
		}
		return nil
	}

	tasks := importer.BuildTasks(fullName, len(matches.Repositories) > 0 || len(matches.RepositorySets) > 0, len(matches.FileSets) > 0)

	p.Phase(fmt.Sprintf("Importing from %s ...", fullName))
	p.BlankLine()
	tracker := ui.RunRefresh(tasks)

	// failAll marks all remaining spinner tasks as failed and waits.
	failAll := func() {
		for _, t := range tasks {
			tracker.Fail(t.Name)
		}
		tracker.Wait()
	}

	var repoPlan importer.RepoPlan
	if len(matches.Repositories) > 0 || len(matches.RepositorySets) > 0 {
		key := "Importing " + fullName + " (repo)"
		if len(matches.Repositories) > 0 {
			repoPlan, err = importer.PlanRepository(importer.Target{Owner: target.owner, Name: target.name}, matches.Repositories, runner, readManifestBytes, manifestBytes)
			if err != nil {
				failAll()
				return err
			}
		}
		if len(matches.RepositorySets) > 0 {
			setPlan, planErr := importer.PlanRepositorySet(importer.Target{Owner: target.owner, Name: target.name}, matches.RepositorySets, runner, readManifestBytes, manifestBytes)
			if planErr != nil {
				failAll()
				return planErr
			}
			repoPlan.Changes = append(repoPlan.Changes, setPlan.Changes...)
			if repoPlan.ManifestEdits == nil {
				repoPlan.ManifestEdits = make(map[string][]byte)
			}
			for path, data := range setPlan.ManifestEdits {
				repoPlan.ManifestEdits[path] = data
			}
			repoPlan.UpdatedDocs += setPlan.UpdatedDocs
		}
		tracker.Done(key)
	}

	var importChanges []fileset.FileImportChange
	if len(matches.FileSets) > 0 {
		key := "Importing " + fullName + " (files)"
		processor := fileset.NewProcessor(runner, p)

		importChanges, err = fileset.PlanPull(processor, matches.FileSets, fullName)
		if err != nil {
			failAll()
			return err
		}
		tracker.Done(key)
	}
	tracker.Wait()

	hasChanges := repository.HasRealChanges(repoPlan.Changes) || fileset.HasImportChanges(importChanges)

	if !hasChanges {
		p.Message("\nNothing to import. Local state is up-to-date.")
		return nil
	}

	p.Separator()

	if hasChanges {
		printUnifiedImportPlan(p, repoPlan.Changes, importChanges)
	}

	written, unchanged, skipped := fileset.ImportSummary(importChanges)
	totalWritten := repoPlan.UpdatedDocs + written

	if totalWritten == 0 {
		p.Summary("Nothing to import. Local state is up-to-date.")
		return nil
	}

	confirmed, err := p.Confirm("Do you want to import these changes?")
	if err != nil {
		return err
	}
	if !confirmed {
		p.Message("Import canceled.")
		return nil
	}

	// Read manifest bytes for file inline edits
	for _, fs := range matches.FileSets {
		if err := readManifestBytes(fs.SourcePath); err != nil {
			return err
		}
	}

	// Apply file changes (source writes + inline edits)
	if err := fileset.ApplyImport(importChanges, manifestBytes); err != nil {
		return err
	}

	// Write back repo spec changes
	if err := importer.WriteManifestEdits(repoPlan.ManifestEdits); err != nil {
		return err
	}

	p.Summary(fmt.Sprintf("Import complete! %s written, %s unchanged, %s skipped.",
		ui.Bold.Render(fmt.Sprintf("%d", totalWritten)),
		ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
		ui.Bold.Render(fmt.Sprintf("%d", skipped)),
	))

	return nil
}

const defaultImportParallel = 5

func importRepos(p ui.Printer, targets []importTarget, fetcher *repository.Fetcher, resolver *manifest.Resolver) error {
	label := "repository"
	if len(targets) != 1 {
		label = "repositories"
	}
	p.Phase(fmt.Sprintf("Importing %d %s from GitHub API ...", len(targets), label))
	p.BlankLine()

	// Start spinner display
	names := make([]string, len(targets))
	tasks := make([]ui.RefreshTask, len(targets))
	for i, t := range targets {
		fullName := t.owner + "/" + t.name
		names[i] = fullName
		tasks[i] = ui.RefreshTask{
			Name:      "Importing " + fullName,
			DoneLabel: "Imported " + fullName,
			FailLabel: "Failed " + fullName,
		}
	}
	tracker := ui.RunRefresh(tasks)

	// Fetch all repos in parallel
	type importResult struct {
		data []byte
		err  error
	}
	results := parallel.Map(targets, defaultImportParallel, func(_ int, t importTarget) importResult {
		fullName := t.owner + "/" + t.name
		key := "Importing " + fullName
		current, err := fetcher.FetchRepository(t.owner, t.name)
		if err != nil {
			tracker.Fail(key)
			return importResult{err: err}
		}
		if current.IsNew {
			tracker.Fail(key)
			return importResult{err: fmt.Errorf("repository %s not found on GitHub", fullName)}
		}
		m := repository.ToManifest(current, resolver)
		data, err := goyaml.Marshal(m)
		if err != nil {
			tracker.Fail(key)
		} else {
			tracker.Done(key)
		}
		return importResult{data: data, err: err}
	})
	tracker.Wait()

	// Count results
	succeeded := 0
	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	p.Separator()

	// Output YAML in order
	out := p.OutWriter()
	first := true
	for _, r := range results {
		if r.err != nil {
			continue
		}
		if !first {
			fmt.Fprintln(out, "---")
		}
		fmt.Fprint(out, string(r.data))
		first = false
	}

	// Print errors to stderr so they remain visible when stdout is redirected
	if failed > 0 {
		for i, r := range results {
			if r.err != nil {
				p.Warning(names[i], fmt.Sprintf("skipping: %v", r.err))
			}
		}
	}

	// Summary
	summaryMsg := fmt.Sprintf("Import complete! %s exported", ui.Bold.Render(fmt.Sprintf("%d", succeeded)))
	if failed > 0 {
		summaryMsg += fmt.Sprintf(", %s failed", ui.Bold.Render(fmt.Sprintf("%d", failed)))
	}
	summaryMsg += "."
	p.Summary(summaryMsg)
	return nil
}
