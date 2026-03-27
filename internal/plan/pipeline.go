package plan

import (
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// Options configures the shared plan pipeline.
type Options struct {
	Path          string
	FilterRepo    string
	FailOnUnknown bool
	ForceSecrets  bool // only meaningful for apply
	DryRun        bool // true = plan only (skip secret resolution)
}

// Result holds the outcome of the plan phase.
type Result struct {
	RepoChanges []repository.Change
	FileChanges []fileset.FileChange
	TargetRepos []*manifest.Repository
	Parsed      *manifest.ParseResult
	Printer     ui.Printer
	Runner      gh.Runner
	Resolver    *manifest.Resolver

	Creates int
	Updates int
	Deletes int

	HasChanges bool
}

// Run executes the shared plan phase used by both plan and apply commands.
// It parses manifests, fetches current state, computes diffs, and prints the plan.
func Run(opts Options) (*Result, error) {
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(opts.Path, manifest.ParseOptions{FailOnUnknown: opts.FailOnUnknown})
	if err != nil {
		return nil, err
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("deprecation", w)
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + opts.Path)
		return &Result{Printer: p}, nil
	}

	if !opts.DryRun {
		manifest.ResolveSecrets(parsed.Repositories)
	}

	runner := gh.NewRunner(false)

	var resolverOwner string
	if len(parsed.Repositories) > 0 {
		resolverOwner = parsed.Repositories[0].Metadata.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", opts.Path))
	p.Phase("Fetching current state from GitHub API ...")
	p.BlankLine()

	// Collect all target names and start a single spinner display
	var allTasks []ui.RefreshTask
	allTasks = append(allTasks, repository.FetchTargetNames(parsed.Repositories, opts.FilterRepo)...)
	allTasks = append(allTasks, fileset.PlanTargetNames(parsed.FileSets, opts.FilterRepo)...)
	tracker := ui.RunRefresh(allTasks)

	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	var fileChanges []fileset.FileChange

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		diffOpts := repository.DiffOptions{ForceSecrets: opts.ForceSecrets, Resolver: resolver}
		g.Go(func() error {
			var fetchErr error
			repoChanges, targetRepos, fetchErr = repository.FetchAllChanges(parsed.Repositories, opts.FilterRepo, fetcher, p, tracker, diffOpts)
			return fetchErr
		})
	}

	if len(parsed.FileSets) > 0 {
		processor := fileset.NewProcessor(runner, p)
		g.Go(func() error {
			var planErr error
			fileChanges, planErr = processor.Plan(parsed.FileSets, opts.FilterRepo, tracker)
			return planErr
		})
	}

	if err := g.Wait(); err != nil {
		tracker.Wait()
		return nil, err
	}
	tracker.Wait()

	hasRepo := repository.HasChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	result := &Result{
		RepoChanges: repoChanges,
		FileChanges: fileChanges,
		TargetRepos: targetRepos,
		Parsed:      parsed,
		Printer:     p,
		Runner:      runner,
		Resolver:    resolver,
		HasChanges:  hasRepo || hasFile,
	}

	if !result.HasChanges {
		p.Message("\nNo changes. Infrastructure is up-to-date.")
		return result, nil
	}

	// Count and print unified plan
	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, fileDeletes := fileset.CountChanges(fileChanges)
	result.Creates = repoCreates + fileCreates
	result.Updates = repoUpdates + fileUpdates
	result.Deletes = repoDeletes + fileDeletes

	p.Separator()
	p.Legend(result.Creates > 0, result.Updates > 0, result.Deletes > 0)

	PrintPlan(p, repoChanges, fileChanges)

	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", result.Creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", result.Updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", result.Deletes))),
	}
	p.Summary(fmt.Sprintf("Plan: %s", strings.Join(parts, ", ")))

	return result, nil
}
