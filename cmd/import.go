package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <owner/repo> [owner/repo ...]",
		Short: "Export existing repository settings as YAML",
		Long:  "Fetch current GitHub repository settings and output them as gh-infra YAML.\nMultiple repositories can be specified to import them in parallel.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(args)
		},
	}
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

const defaultImportParallel = 5

func importRepos(p ui.Printer, targets []importTarget, fetcher *repository.Fetcher, resolver *manifest.Resolver) error {
	label := "repository"
	if len(targets) != 1 {
		label = "repositories"
	}
	p.Phase(fmt.Sprintf("Importing %d %s from GitHub API ...", len(targets), label))
	fmt.Fprintln(p.ErrWriter())

	// Start spinner display
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.owner + "/" + t.name
	}
	tracker := ui.RunRefresh(names)

	// Fetch all repos in parallel
	type importResult struct {
		data []byte
		err  error
	}
	results := make([]importResult, len(targets))
	sem := semaphore.NewWeighted(defaultImportParallel)
	var wg sync.WaitGroup

	for i, t := range targets {
		wg.Add(1)
		go func(idx int, owner, name string) {
			defer wg.Done()
			_ = sem.Acquire(context.Background(), 1)
			defer sem.Release(1)

			fullName := owner + "/" + name
			current, err := fetcher.FetchRepository(owner, name)
			if err != nil {
				results[idx] = importResult{err: err}
				tracker.Error(fullName, err)
				return
			}
			m := repository.ToManifest(current, resolver)
			data, err := goyaml.Marshal(m)
			results[idx] = importResult{data: data, err: err}
			if err != nil {
				tracker.Error(fullName, err)
			} else {
				tracker.Done(fullName)
			}
		}(i, t.owner, t.name)
	}

	wg.Wait()
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
	first := true
	for _, r := range results {
		if r.err != nil {
			continue
		}
		if !first {
			fmt.Println("---")
		}
		fmt.Fprint(os.Stdout, string(r.data))
		first = false
	}

	// Print errors
	if failed > 0 {
		fmt.Fprintln(os.Stdout)
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
