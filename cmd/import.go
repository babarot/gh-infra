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
				return runImportFiles(args, into, dryRun)
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

func runImportFiles(args []string, searchPath string, dryRun bool) error {
	p := ui.NewStandardPrinter()

	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}

	// For file import, process one repo at a time
	for _, target := range targets {
		fullName := target.owner + "/" + target.name
		if err := importFilesForRepo(p, fullName, searchPath, dryRun); err != nil {
			return err
		}
	}

	return nil
}

func importFilesForRepo(p ui.Printer, filterRepo, searchPath string, dryRun bool) error {
	// Discover FileSet manifests from the given path
	parsed, err := manifest.ParseAll(searchPath)
	if err != nil {
		return err
	}

	// Filter to FileSets that reference this repo
	var matchedFileSets []*manifest.FileSet
	for _, fs := range parsed.FileSets {
		for _, r := range fs.Spec.Repositories {
			if fs.RepoFullName(r.Name) == filterRepo {
				matchedFileSets = append(matchedFileSets, fs)
				break
			}
		}
	}

	if len(matchedFileSets) == 0 {
		return fmt.Errorf("no File or FileSet found for %s in the current directory", filterRepo)
	}

	runner := gh.NewRunner(false)
	processor := fileset.NewProcessor(runner, p)

	// Count total files for spinner
	var fetchTasks []ui.RefreshTask
	fetchTasks = append(fetchTasks, ui.RefreshTask{
		Name:      "Importing " + filterRepo + " (files)",
		DoneLabel: "Imported " + filterRepo + " (files)",
		FailLabel: "Failed " + filterRepo + " (files)",
	})
	tracker := ui.RunRefresh(fetchTasks)

	p.Phase(fmt.Sprintf("Importing file content from %s ...", filterRepo))
	p.BlankLine()

	changes, err := fileset.PlanPull(processor, matchedFileSets, filterRepo)
	if err != nil {
		tracker.Fail(fetchTasks[0].Name)
		tracker.Wait()
		return err
	}
	tracker.Done(fetchTasks[0].Name)
	tracker.Wait()

	written, unchanged, skipped := fileset.PullSummary(changes)

	// Display results using existing printer patterns
	p.Separator()

	for _, c := range changes {
		switch c.Type {
		case fileset.PullWriteSource:
			relTarget := relativePath(c.LocalTarget)
			p.ResultSuccess(c.Path, fmt.Sprintf("→ %s", relTarget))
		case fileset.PullWriteInline:
			relManifest := relativePath(c.ManifestPath)
			p.ResultSuccess(c.Path, fmt.Sprintf("→ %s (inline)", relManifest))
		case fileset.PullNoOp:
			p.Detail(fmt.Sprintf("  %s  unchanged", c.Path))
		case fileset.PullSkip:
			p.ResultWarning(c.Path, c.Reason)
		}
	}

	// Print warnings
	for _, c := range changes {
		for _, w := range c.Warnings {
			p.Warning(c.Path, w)
		}
	}

	if dryRun {
		p.Summary(fmt.Sprintf("Dry run: %s to write, %s unchanged, %s to skip.",
			ui.Bold.Render(fmt.Sprintf("%d", written)),
			ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
			ui.Bold.Render(fmt.Sprintf("%d", skipped)),
		))
		return nil
	}

	if written == 0 {
		p.Summary("Nothing to import. Local files are up-to-date.")
		return nil
	}

	// Read manifest bytes for inline edits
	manifestBytes := make(map[string][]byte)
	for _, fs := range matchedFileSets {
		sp := fs.SourcePath()
		if _, ok := manifestBytes[sp]; !ok {
			data, err := os.ReadFile(sp)
			if err != nil {
				return fmt.Errorf("read manifest %s: %w", sp, err)
			}
			manifestBytes[sp] = data
		}
	}

	if err := fileset.ApplyPull(changes, manifestBytes); err != nil {
		return err
	}

	p.Summary(fmt.Sprintf("Import complete! %s written, %s unchanged, %s skipped.",
		ui.Bold.Render(fmt.Sprintf("%d", written)),
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
