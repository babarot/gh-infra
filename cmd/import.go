package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func newImportCmd() *cobra.Command {
	var (
		into   string
		dryRun bool
	)

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
				return runImportInto(args, into, dryRun)
			}
			return runImport(args)
		},
	}

	cmd.Flags().StringVar(&into, "into", "", "Import file content into local sources via the given manifest or directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be written without making changes (requires --into)")

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

func runImportInto(args []string, searchPath string, dryRun bool) error {
	p := ui.NewStandardPrinter()

	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}

	for _, target := range targets {
		fullName := target.owner + "/" + target.name
		if err := importIntoForRepo(p, target, fullName, searchPath, dryRun); err != nil {
			return err
		}
	}

	return nil
}

type importMatches struct {
	repositories   []*manifest.Repository
	fileSets       []*manifest.FileSet
	skippedRepoSet bool
}

type repoImportPlan struct {
	changes      []repository.Change
	manifestEdits map[string][]byte
	updatedDocs  int
}

func importIntoForRepo(p ui.Printer, target importTarget, fullName, searchPath string, dryRun bool) error {
	parsed, err := manifest.ParseAll(searchPath)
	if err != nil {
		return err
	}

	matches := findImportMatches(parsed, fullName)
	if len(matches.repositories) == 0 && len(matches.fileSets) == 0 {
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

	tasks := buildImportTasks(fullName, len(matches.repositories) > 0, len(matches.fileSets) > 0)

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

	var repoPlan repoImportPlan
	if len(matches.repositories) > 0 {
		key := "Importing " + fullName + " (repo)"
		repoPlan, err = planRepositoryImport(target, matches.repositories, runner, readManifestBytes, manifestBytes)
		if err != nil {
			failAll()
			return err
		}
		tracker.Done(key)
	}

	var importChanges []fileset.FileImportChange
	if len(matches.fileSets) > 0 {
		key := "Importing " + fullName + " (files)"
		processor := fileset.NewProcessor(runner, p)

		importChanges, err = fileset.PlanPull(processor, matches.fileSets, fullName)
		if err != nil {
			failAll()
			return err
		}
		tracker.Done(key)
	}
	tracker.Wait()

	hasChanges := repository.HasRealChanges(repoPlan.changes) || fileset.HasImportChanges(importChanges)

	if !hasChanges && !matches.skippedRepoSet {
		p.Message("\nNothing to import. Local state is up-to-date.")
		return nil
	}

	p.Separator()

	if matches.skippedRepoSet {
		p.Warning(fullName, "RepositorySet import not yet supported, use `gh infra import "+fullName+"`")
	}

	if hasChanges {
		printUnifiedImportPlan(p, repoPlan.changes, importChanges)
	}

	written, unchanged, skipped := fileset.ImportSummary(importChanges)
	totalWritten := repoPlan.updatedDocs + written

	if dryRun {
		p.Summary(fmt.Sprintf("Dry run: %s to write, %s unchanged, %s to skip.",
			ui.Bold.Render(fmt.Sprintf("%d", totalWritten)),
			ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
			ui.Bold.Render(fmt.Sprintf("%d", skipped)),
		))
		return nil
	}

	if totalWritten == 0 {
		p.Summary("Nothing to import. Local state is up-to-date.")
		return nil
	}

	// Read manifest bytes for file inline edits
	for _, fs := range matches.fileSets {
		if err := readManifestBytes(fs.SourcePath()); err != nil {
			return err
		}
	}

	// Apply file changes (source writes + inline edits)
	if err := fileset.ApplyImport(importChanges, manifestBytes); err != nil {
		return err
	}

	// Write back repo spec changes
	if err := writeRepoImportEdits(repoPlan.manifestEdits); err != nil {
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

func findImportMatches(parsed *manifest.ParseResult, fullName string) importMatches {
	var matches importMatches

	for _, repo := range parsed.Repositories {
		if repo.Metadata.FullName() != fullName {
			continue
		}
		if repo.FromSet() {
			matches.skippedRepoSet = true
			continue
		}
		matches.repositories = append(matches.repositories, repo)
	}

	for _, fs := range parsed.FileSets {
		for _, r := range fs.Spec.Repositories {
			if fs.RepoFullName(r.Name) == fullName {
				matches.fileSets = append(matches.fileSets, fs)
				break
			}
		}
	}

	return matches
}

func buildImportTasks(fullName string, includeRepo, includeFiles bool) []ui.RefreshTask {
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

func planRepositoryImport(target importTarget, repos []*manifest.Repository, runner gh.Runner, readManifestBytes func(string) error, manifestBytes map[string][]byte) (repoImportPlan, error) {
	fetcher := repository.NewFetcher(runner)
	resolver := manifest.NewResolver(runner, target.owner)

	githubState, err := fetcher.FetchRepository(target.owner, target.name)
	if err != nil {
		return repoImportPlan{}, err
	}

	imported := repository.ToManifest(githubState, resolver)
	plan := repoImportPlan{manifestEdits: make(map[string][]byte)}

	for _, repo := range repos {
		diffOpts := repository.DiffOptions{Resolver: resolver}
		changes := repository.Diff(repo, githubState, diffOpts)
		plan.changes = append(plan.changes, repository.ReverseChanges(changes)...)
		if !repository.HasRealChanges(changes) {
			continue
		}

		if err := readManifestBytes(repo.SourcePath()); err != nil {
			return repoImportPlan{}, err
		}
		data := manifestBytes[repo.SourcePath()]
		data, err = fileset.ReplaceYAMLNode(data, repo.DocIndex(), "$.spec", imported.Spec)
		if err != nil {
			return repoImportPlan{}, fmt.Errorf("update spec in %s: %w", repo.SourcePath(), err)
		}
		manifestBytes[repo.SourcePath()] = data
		plan.manifestEdits[repo.SourcePath()] = data
		plan.updatedDocs++
	}

	return plan, nil
}

func writeRepoImportEdits(manifestEdits map[string][]byte) error {
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
