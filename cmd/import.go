package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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

func importIntoForRepo(p ui.Printer, target importTarget, fullName, searchPath string, dryRun bool) error {
	parsed, err := manifest.ParseAll(searchPath)
	if err != nil {
		return err
	}

	// Find matching Repository manifests (skip RepositorySet — reverse merge not yet supported)
	var matchedRepos []*manifest.Repository
	var skippedRepoSet bool
	for _, repo := range parsed.Repositories {
		if repo.Metadata.FullName() == fullName {
			if repo.FromSet() {
				skippedRepoSet = true
				continue
			}
			matchedRepos = append(matchedRepos, repo)
		}
	}

	// Find matching FileSet manifests
	var matchedFileSets []*manifest.FileSet
	for _, fs := range parsed.FileSets {
		for _, r := range fs.Spec.Repositories {
			if fs.RepoFullName(r.Name) == fullName {
				matchedFileSets = append(matchedFileSets, fs)
				break
			}
		}
	}

	if len(matchedRepos) == 0 && len(matchedFileSets) == 0 {
		return fmt.Errorf("no manifests found for %s in %s", fullName, searchPath)
	}

	runner := gh.NewRunner(false)

	// Build spinner tasks
	var tasks []ui.RefreshTask
	if len(matchedRepos) > 0 {
		tasks = append(tasks, ui.RefreshTask{
			Name:      "Importing " + fullName + " (repo)",
			DoneLabel: "Imported " + fullName + " (repo)",
			FailLabel: "Failed " + fullName + " (repo)",
		})
	}
	if len(matchedFileSets) > 0 {
		tasks = append(tasks, ui.RefreshTask{
			Name:      "Importing " + fullName + " (files)",
			DoneLabel: "Imported " + fullName + " (files)",
			FailLabel: "Failed " + fullName + " (files)",
		})
	}

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

	// Collect manifest bytes for in-place edits (repo spec + inline content)
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

	// --- Repository import ---
	var repoUpdated int
	var repoChanges []repository.Change
	if len(matchedRepos) > 0 {
		key := "Importing " + fullName + " (repo)"
		fetcher := repository.NewFetcher(runner)
		resolver := manifest.NewResolver(runner, target.owner)

		githubState, err := fetcher.FetchRepository(target.owner, target.name)
		if err != nil {
			failAll()
			return err
		}

		imported := repository.ToManifest(githubState, resolver)

		// Compute diff: local manifest vs GitHub state
		// Diff returns "what would change if we apply local to GitHub"
		// For import display, we swap OldValue/NewValue to show "GitHub → local"
		for _, repo := range matchedRepos {
			diffOpts := repository.DiffOptions{Resolver: resolver}
			changes := repository.Diff(repo, githubState, diffOpts)
			repoChanges = append(repoChanges, swapChanges(changes)...)

			if err := readManifestBytes(repo.SourcePath()); err != nil {
				failAll()
				return err
			}

			data := manifestBytes[repo.SourcePath()]
			data, err = fileset.ReplaceYAMLNode(data, repo.DocIndex(), "$.spec", imported.Spec)
			if err != nil {
				failAll()
				return fmt.Errorf("update spec in %s: %w", repo.SourcePath(), err)
			}
			manifestBytes[repo.SourcePath()] = data
			repoUpdated++
		}
		tracker.Done(key)
	}

	// --- FileSet import ---
	var importChanges []fileset.FileImportChange
	if len(matchedFileSets) > 0 {
		key := "Importing " + fullName + " (files)"
		processor := fileset.NewProcessor(runner, p)

		importChanges, err = fileset.PlanPull(processor, matchedFileSets, fullName)
		if err != nil {
			failAll()
			return err
		}
		tracker.Done(key)
	}
	tracker.Wait()

	// --- Display using unified plan output ---
	hasChanges := repository.HasRealChanges(repoChanges) || fileset.HasImportChanges(importChanges)

	if !hasChanges && !skippedRepoSet {
		p.Message("\nNothing to import. Local state is up-to-date.")
		return nil
	}

	p.Separator()

	if skippedRepoSet {
		p.Warning(fullName, "RepositorySet import not yet supported, use `gh infra import "+fullName+"`")
	}

	if hasChanges {
		printUnifiedImportPlan(p, repoChanges, importChanges)
	}

	written, unchanged, skipped := fileset.ImportSummary(importChanges)
	totalWritten := repoUpdated + written

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
	for _, fs := range matchedFileSets {
		if err := readManifestBytes(fs.SourcePath()); err != nil {
			return err
		}
	}

	// Apply file changes (source writes + inline edits)
	if err := fileset.ApplyImport(importChanges, manifestBytes); err != nil {
		return err
	}

	// Write back repo spec changes
	if repoUpdated > 0 {
		for _, repo := range matchedRepos {
			data := manifestBytes[repo.SourcePath()]
			if err := os.WriteFile(repo.SourcePath(), data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", repo.SourcePath(), err)
			}
		}
	}

	p.Summary(fmt.Sprintf("Import complete! %s written, %s unchanged, %s skipped.",
		ui.Bold.Render(fmt.Sprintf("%d", totalWritten)),
		ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
		ui.Bold.Render(fmt.Sprintf("%d", skipped)),
	))

	return nil
}

// swapChanges reverses repository changes for import display.
// Diff produces changes that would update GitHub from local manifest state.
// For import, we need the opposite direction: local state updated from GitHub.
func swapChanges(changes []repository.Change) []repository.Change {
	swapped := make([]repository.Change, len(changes))
	for i, c := range changes {
		if len(c.Children) > 0 {
			c.Children = swapChanges(c.Children)
		} else {
			c.OldValue, c.NewValue = c.NewValue, c.OldValue
		}

		switch c.Type {
		case repository.ChangeCreate:
			c.Type = repository.ChangeDelete
		case repository.ChangeDelete:
			c.Type = repository.ChangeCreate
		}

		swapped[i] = c
	}
	return swapped
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

// relativePath attempts to make a path relative to the current working directory.
func relativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
