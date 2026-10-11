package infra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// ErrNoResources is returned by Plan when the paths hold no gh-infra
// manifests. The message has already been printed.
var ErrNoResources = errors.New("no resources found")

// PlanOptions configures the plan phase.
type PlanOptions struct {
	Paths         []string
	FilterRepo    string
	FailOnUnknown bool
	ForceSecrets  bool   // only meaningful when followed by Apply
	DryRun        bool   // true = plan only (skip secret resolution)
	ShowDiff      bool   // emit unified diff for each file change in plan output
	Output        string // OutputText (default), OutputJSON or OutputMarkdown; the latter two print nothing while planning
	ChangedSince  string // git ref: plan only the manifests changed since it (with the local files they read)

	// Runner runs every gh call of this plan and the apply that follows.
	// The command shares one runner so its concurrency limit covers all of
	// them; nil uses a new default runner.
	Runner gh.Runner
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

	// FailedTargets lists repositories whose current state could not be
	// fetched or planned. Their changes are missing from this result.
	FailedTargets []string

	// TargetErrors are the errors behind FailedTargets.
	TargetErrors []ui.TaskError

	// Warnings are the warnings from parsing the manifests.
	Warnings []string

	// ChangedSince is what --changed-since selected; nil without the flag.
	ChangedSince *ChangedSince

	runner gh.Runner // the command's runner, for the API usage

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
	var p ui.Printer = ui.NewStandardPrinter()
	if isMachineOutput(opts.Output) {
		// Print nothing but the document: silence this printer, and the
		// package printer that the progress fallback and tracker use.
		p = discardPrinter()
		restore := ui.SetDefaultPrinter(p)
		defer restore()
	}

	paths, err := manifest.ResolvePaths(opts.Paths)
	if err != nil {
		return nil, err
	}

	runner := opts.Runner
	if runner == nil {
		runner = gh.NewRunner(false)
	}
	sourceResolver := manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
		return runner.Run(ctx, args...)
	})

	parseOpt := manifest.ParseOptions{
		FailOnUnknown: opts.FailOnUnknown,
		Resolver:      sourceResolver,
	}
	parsed := &manifest.ParseResult{}
	var total, selected int
	var changedSince *ChangedSince
	if opts.ChangedSince != "" {
		parsed, total, selected, err = parseChanged(context.Background(), paths, opts.ChangedSince, parseOpt)
		if err != nil {
			return nil, err
		}
		changedSince = &ChangedSince{Ref: opts.ChangedSince, Selected: selected, Total: total}
		if selected == 0 {
			p.Message(fmt.Sprintf("No manifests changed since %s.", opts.ChangedSince))
			return &PlanResult{Parsed: parsed, ChangedSince: changedSince, runner: runner}, nil
		}
	} else {
		for _, path := range paths {
			result, err := manifest.ParseAll(path, parseOpt)
			if err != nil {
				return nil, err
			}
			parsed.Merge(result)
		}
		if err := parsed.Validate(); err != nil {
			return nil, err
		}
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("warning", w)
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + strings.Join(paths, ", "))
		return nil, ErrNoResources
	}

	if !opts.DryRun {
		if err := manifest.ResolveSecrets(parsed.Repositories); err != nil {
			return nil, err
		}
	}

	var resolverOwner string
	if len(parsed.Repositories) > 0 {
		resolverOwner = parsed.Repositories[0].Metadata.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)

	eng := newEngine(runner, resolver, p)
	eng.quiet = isMachineOutput(opts.Output)

	displayPaths := make([]string, len(paths))
	for i, path := range paths {
		displayPaths[i] = tildePath(path)
	}
	p.Phase(fmt.Sprintf("Reading desired state from %s ...", strings.Join(displayPaths, ", ")))
	if opts.ChangedSince != "" {
		p.Phase(fmt.Sprintf("Planning %d of %d resources changed since %s ...", selected, total, opts.ChangedSince))
	}
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
			Name:      n,
			DoneLabel: "planned desired changes",
			Pending:   taskMap[n],
		})
	}
	tracker := ui.RunRefresh(allTasks)
	defer showRateLimitWaits(runner, tracker)()

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
		ChangedSince:  changedSince,
		RepoChanges:   repoChanges,
		FileChanges:   fileChanges,
		TargetRepos:   targetRepos,
		Parsed:        parsed,
		HasChanges:    hasRepo || hasFile,
		FailedTargets: failedTargets(tracker.Errors()),
		TargetErrors:  tracker.Errors(),
		Warnings:      parsed.Warnings,
		engine:        eng,
		runner:        runner,
	}

	if !result.HasChanges {
		if len(result.FailedTargets) > 0 {
			p.Message("\nNo changes computed. Some repositories were skipped due to errors above.")
		} else {
			p.Message("\nNo changes. Infrastructure is up-to-date.")
		}
		if opts.DryRun {
			printAPIUsage(p, apiUsage(runner))
		}
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

	printPlan(p, repoChanges, fileChanges, opts.ShowDiff)

	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", result.Creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", result.Updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", result.Deletes))),
	}
	p.Summary(fmt.Sprintf("Plan: %s", strings.Join(parts, ", ")))

	if opts.DryRun {
		printAPIUsage(p, apiUsage(runner))
	}
	return result, nil
}

// failedTargets returns the unique task names that reported an error.
func failedTargets(errs []ui.TaskError) []string {
	names := make([]string, len(errs))
	for i, e := range errs {
		names[i] = e.Name
	}
	return uniqueStrings(names)
}

// FailedTargetsError returns an error naming the repositories that could not
// be planned, or nil if every repository was planned.
func (r *PlanResult) FailedTargetsError() error {
	if r == nil || len(r.FailedTargets) == 0 {
		return nil
	}
	noun := "repositories"
	if len(r.FailedTargets) == 1 {
		noun = "repository"
	}
	return fmt.Errorf("could not plan %d %s: %s",
		len(r.FailedTargets), noun, strings.Join(r.FailedTargets, ", "))
}

// PrintAPIUsage prints the API usage line for an apply that had nothing to
// apply (Apply prints it otherwise).
func (r *PlanResult) PrintAPIUsage() {
	if r == nil || (r.engine != nil && r.engine.quiet) {
		return
	}
	printAPIUsage(r.Printer(), apiUsage(r.runner))
}
