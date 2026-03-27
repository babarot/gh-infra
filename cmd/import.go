package cmd

import (
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

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
	if matches.IsEmpty() {
		return fmt.Errorf("no manifests found for %s in %s", fullName, searchPath)
	}

	runner := gh.NewRunner(false)
	tasks := importer.BuildTasks(fullName, matches.HasRepo(), matches.HasFiles())

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

	plan, err := importer.PlanInto(matches, importer.Target{Owner: target.owner, Name: target.name}, runner, p, tracker)
	if err != nil {
		failAll()
		return err
	}
	tracker.Wait()

	if !plan.HasChanges() {
		p.Message("\nNothing to import. Local state is up-to-date.")
		return nil
	}

	p.Separator()

	printUnifiedImportPlan(p, plan.RepoChanges, plan.FileChanges)

	written, unchanged, skipped := plan.Summary()

	if written == 0 {
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

	if err := plan.Apply(); err != nil {
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
