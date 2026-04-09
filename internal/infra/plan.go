package infra

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// PlanOptions configures the plan phase.
type PlanOptions struct {
	Paths         []string
	FilterRepo    string
	FailOnUnknown bool
	ForceSecrets  bool // only meaningful when followed by Apply
	DryRun        bool // true = plan only (skip secret resolution)
}

// PlanResult holds the outcome of the plan phase.
type PlanResult struct {
	RepoChanges []repository.Change
	FileChanges []fileset.Change
	TargetRepos []*manifest.Repository
	Parsed      *manifest.ParseResult

	Creates int
	Updates int
	Deletes int

	HasChanges bool

	engine *engine // unexported runtime context for Apply
}

// Printer returns the printer used during this plan session.
func (r *PlanResult) Printer() ui.Printer {
	if r.engine == nil {
		return ui.NewStandardPrinter()
	}
	return r.engine.printer
}

// Plan parses manifests, fetches current state, computes diffs, and prints the plan.
func Plan(opts PlanOptions) (*PlanResult, error) {
	p := ui.NewStandardPrinter()

	paths, err := manifest.ResolvePaths(opts.Paths)
	if err != nil {
		return nil, err
	}

	runner := gh.NewRunner(false)
	sourceResolver := manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
		return runner.Run(ctx, args...)
	})

	parsed := &manifest.ParseResult{}
	for _, path := range paths {
		result, err := manifest.ParseAll(path, manifest.ParseOptions{
			FailOnUnknown: opts.FailOnUnknown,
			Resolver:      sourceResolver,
		})
		if err != nil {
			return nil, err
		}
		parsed.Merge(result)
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("deprecation", w)
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + strings.Join(paths, ", "))
		return nil, context.Canceled
	}

	if !opts.DryRun {
		manifest.ResolveSecrets(parsed.Repositories)
	}

	var resolverOwner string
	if len(parsed.Repositories) > 0 {
		resolverOwner = parsed.Repositories[0].Metadata.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)

	eng := newEngine(runner, resolver, p)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", strings.Join(paths, ", ")))
	p.Phase("Fetching current state from GitHub API ...")
	p.BlankLine()

	// Collect all target names and start a single spinner display.
	// Deduplicate: one line per repo, with a pending count for how many
	// sources (repository / fileset) will call Done().
	repoNames := repository.PlanTargetRepoNames(parsed.Repositories, opts.FilterRepo)
	fileNames := fileset.PlanTargetRepoNames(parsed.FileSets, opts.FilterRepo)
	taskMap := make(map[string]int) // repo full name → pending count
	for _, n := range repoNames {
		taskMap[n]++
	}
	for _, n := range fileNames {
		taskMap[n]++
	}
	// Build tasks preserving order (repos first, then fileset-only)
	var allTasks []ui.RefreshTask
	seen := make(map[string]bool)
	for _, n := range append(repoNames, fileNames...) {
		if seen[n] {
			continue
		}
		seen[n] = true
		allTasks = append(allTasks, ui.RefreshTask{
			Name:    n,
			Pending: taskMap[n],
		})
	}
	tracker := ui.RunRefresh(allTasks)

	// Create a cancellable context; cancel when the spinner is interrupted via Ctrl+C.
	ctx, cancel := withTrackerCancelContext(tracker)
	defer cancel()

	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	var fileChanges []fileset.Change

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		g.Go(func() error {
			var fetchErr error
			repoChanges, targetRepos, fetchErr = eng.repo.Plan(ctx, parsed.Repositories, repository.PlanOptions{
				FilterRepo:   opts.FilterRepo,
				ForceSecrets: opts.ForceSecrets,
			}, tracker)
			return fetchErr
		})
	}

	if len(parsed.FileSets) > 0 {
		g.Go(func() error {
			var planErr error
			fileChanges, planErr = eng.file.Plan(ctx, parsed.FileSets, opts.FilterRepo, tracker)
			return planErr
		})
	}

	if err := g.Wait(); err != nil {
		tracker.Wait()
		tracker.PrintErrors()
		if ctx.Err() != nil {
			return nil, context.Canceled
		}
		return nil, err
	}
	tracker.Wait()
	tracker.PrintErrors()

	if ctx.Err() != nil {
		return nil, context.Canceled
	}

	hasRepo := repository.HasChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	result := &PlanResult{
		RepoChanges: repoChanges,
		FileChanges: fileChanges,
		TargetRepos: targetRepos,
		Parsed:      parsed,
		HasChanges:  hasRepo || hasFile,
		engine:      eng,
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

	printPlan(p, repoChanges, fileChanges)

	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", result.Creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", result.Updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", result.Deletes))),
	}
	p.Summary(fmt.Sprintf("Plan: %s", strings.Join(parts, ", ")))

	return result, nil
}
