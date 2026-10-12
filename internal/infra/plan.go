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

// ErrAlreadyReported marks an error whose detail was already shown, per
// target, by the progress display: the command fails without printing it
// again.
var ErrAlreadyReported = errors.New("already reported")

// reportedError is err marked with ErrAlreadyReported.
type reportedError struct{ err error }

func (e reportedError) Error() string   { return e.err.Error() }
func (e reportedError) Unwrap() []error { return []error{e.err, ErrAlreadyReported} }

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

	// CreateNotes explains, per repository this plan creates, how its create
	// differs from what the plan shows, such as what a later plan applies.
	CreateNotes map[string][]string

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
// Canceling ctx stops it with context.Canceled.
func Plan(ctx context.Context, opts PlanOptions) (*PlanResult, error) {
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
		Context:       ctx,
	}
	parsed := &manifest.ParseResult{}
	var total, selected int
	var changedSince *ChangedSince
	if opts.ChangedSince != "" {
		parsed, total, selected, err = parseChanged(ctx, paths, opts.ChangedSince, parseOpt)
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

	// Bound to no owner: each repository resolves names against its own.
	resolver := manifest.NewResolver(runner)

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

	// One spinner line per repository, done when each repository and FileSet
	// unit planning it has reported.
	repoNames := repository.PlanTargetRepoNames(parsed.Repositories, opts.FilterRepo)
	fileNames := fileset.PlanTargetRepoNames(parsed.FileSets, opts.FilterRepo)
	allTasks := refreshTasks(append(repoNames, fileNames...), "planned desired changes")
	tracker, ctx, stop := startRefresh(ctx, runner, allTasks)
	defer stop()

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

	err = g.Wait()
	tracker.Wait()
	// Errors of interrupted work are not reported.
	if ctx.Err() != nil {
		return nil, context.Canceled
	}
	tracker.PrintErrors()
	if err != nil {
		// Planning stops on an error that the tracker has shown for its
		// target. Machine output silences the tracker, so there it is left
		// for the command to print.
		if !isMachineOutput(opts.Output) {
			err = reportedError{err}
		}
		return nil, err
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
		CreateNotes:   createNotes(repoChanges, targetRepos),
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

	printPlanWithNotes(p, repoChanges, fileChanges, result.CreateNotes, opts.ShowDiff)

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

// createNotes returns the notes on the create of each new repository.
func createNotes(changes []repository.Change, repos []*manifest.Repository) map[string][]string {
	created := make(map[string]bool)
	for _, c := range changes {
		if c.Type == repository.ChangeCreate && c.Resource == manifest.ResourceRepository && c.Field == "repository" {
			created[c.Name] = true
		}
	}
	var out map[string][]string
	for _, r := range repos {
		name := r.Metadata.FullName()
		if !created[name] {
			continue
		}
		if notes := repository.CreateNotes(r); len(notes) > 0 {
			if out == nil {
				out = make(map[string][]string)
			}
			out[name] = notes
		}
	}
	return out
}
