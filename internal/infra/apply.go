package infra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// ApplyOptions configures the apply phase.
type ApplyOptions struct {
	Stream bool // true = stream output mode instead of spinner
}

// ApplyOutcome holds the result of each applied change.
type ApplyOutcome struct {
	RepoResults []repository.ApplyResult
	FileResults []fileset.ApplyResult
}

// Apply executes planned changes against GitHub. The outcome is returned
// even when some changes failed (with an error), so the caller can report
// each result. Canceling ctx stops it with context.Canceled; what was
// already applied stays applied.
func Apply(ctx context.Context, result *PlanResult, opts ApplyOptions) (*ApplyOutcome, error) {
	eng := result.engine
	p := eng.printer
	if eng.quiet {
		restore := ui.SetDefaultPrinter(p)
		defer restore()
	}

	if err := checkFileChangesMatched(result.Parsed.FileSets, result.FileChanges); err != nil {
		return nil, err
	}

	repoChanges, archives := splitArchives(result.RepoChanges)
	repoNames := changedRepoNames(repoChanges)
	fileSets := fileSetWorkOf(result)
	view, ctx, stop := startApplyView(ctx, opts.Stream, eng, applyTasks(repoNames, fileSets, changedRepoNames(archives)), createOnlyRepos(result.RepoChanges, result.FileChanges))
	defer stop()

	// The steps run in this order in every mode; a mode only changes how
	// progress is shown and whether the results are listed at the end.

	// Step 1: repositories. A FileSet may write to a repository this apply
	// creates, so they go first.
	var repoResults []repository.ApplyResult
	if len(repoNames) > 0 {
		repoResults = eng.repo.Apply(ctx, repoChanges, result.TargetRepos, view.reporter())
	}

	// Step 2: FileSets, in parallel with each other.
	var fileResults []fileset.ApplyResult
	if ctx.Err() == nil {
		fileResults = applyFileSets(ctx, eng, fileSets, view)
	}

	// Step 3: archives. An archived repository is read-only, so archiving
	// waits for everything else applied to the repository.
	if ctx.Err() == nil && len(archives) > 0 {
		repoResults = append(repoResults, applyArchives(ctx, eng, archives, result.TargetRepos, repoResults, fileResults, view)...)
	}

	view.wait()
	// Apply-time errors are rendered in full detail by printApplyResults
	// below (per field / per target). Calling tracker.PrintErrors here
	// would repeat the same message in a separate block.
	if ctx.Err() != nil {
		return nil, context.Canceled
	}

	if view.listsResults() {
		p.Separator()
		printApplyResults(p, repoResults, fileResults)
	}

	succeeded, failed := repository.CountApplyResults(repoResults)
	fs, ff := countFileResults(fileResults)
	succeeded += fs
	failed += ff

	summaryMsg := fmt.Sprintf("Apply complete! %d changes applied.", succeeded)
	if failed > 0 {
		summaryMsg = fmt.Sprintf("%s %d applied, %d failed", ui.Red.Render("Apply finished with errors:"), succeeded, failed)
	}
	p.Summary(summaryMsg)
	printAPIUsage(p, apiUsage(eng.runner))

	outcome := &ApplyOutcome{RepoResults: repoResults, FileResults: fileResults}
	if failed > 0 {
		return outcome, fmt.Errorf("apply had errors")
	}
	return outcome, nil
}

// applyView is how apply shows progress: the spinner, or with stream a line
// per event.
type applyView struct {
	tracker *ui.RefreshTracker // nil with stream
	rows    *applyRows         // nil with stream
	stream  *ui.StreamReporter
}

// startApplyView starts showing progress for tasks. Like startRefresh, it
// shows the runner's rate limit waits and returns a context that is
// canceled with ctx (and with the spinner's Ctrl+C). Call stop when done.
// createOnly names the repositories whose only change is creating them.
func startApplyView(ctx context.Context, stream bool, eng *engine, tasks []ui.RefreshTask, createOnly map[string]bool) (applyView, context.Context, func()) {
	const phase = "Applying changes to GitHub API ..."
	if !stream {
		heading := ui.RefreshHeading{
			Title:  "Applying changes to " + repositories(len(tasks)),
			Phase:  phase,
			Done:   "Applied changes to",
			Failed: "failed",
		}
		tracker, ctx, stop := startRefresh(ctx, eng.runner, heading, tasks)
		return applyView{tracker: tracker, rows: newApplyRows(tracker, createOnly)}, ctx, stop
	}
	p := eng.printer
	p.Phase(phase)
	p.BlankLine()
	stopWaits := showRateLimitWaits(eng.runner, func(text string) {
		if text != "" {
			p.Progress(text)
		}
	})
	return applyView{stream: ui.NewStreamReporter(p, "Applying", "Applied")}, ctx, stopWaits
}

// reporter reports the progress of the targets of one step. Every step of
// the spinner shares one applyRows, which adds up what each applied.
func (v applyView) reporter() ui.ProgressReporter {
	if v.rows == nil {
		return v.stream
	}
	return v.rows
}

func (v applyView) wait() {
	if v.tracker != nil {
		v.tracker.Wait()
	}
}

// listsResults reports whether the results are listed after the progress.
// With stream, the progress lines are the result.
func (v applyView) listsResults() bool { return v.tracker != nil }

// splitArchives separates the changes that archive a repository from the
// others, keeping their order.
func splitArchives(changes []repository.Change) (others, archives []repository.Change) {
	for _, c := range changes {
		if repository.IsArchive(c) {
			archives = append(archives, c)
		} else {
			others = append(others, c)
		}
	}
	return others, archives
}

// errNotArchived is the result of an archive left out because another change
// to the repository failed: once archived, the repository would refuse the
// next apply's fix.
var errNotArchived = errors.New("not archived: earlier changes to this repository failed")

// applyArchives archives the repositories that had no failure in the earlier
// steps, and reports the others as failed without archiving them.
func applyArchives(ctx context.Context, eng *engine, archives []repository.Change, repos []*manifest.Repository,
	repoResults []repository.ApplyResult, fileResults []fileset.ApplyResult, view applyView) []repository.ApplyResult {
	failed := make(map[string]bool)
	for _, r := range repoResults {
		if r.Err != nil {
			failed[r.Change.Name] = true
		}
	}
	for _, r := range fileResults {
		if r.Err != nil {
			failed[r.Change.Target] = true
		}
	}

	var ready, held []repository.Change
	for _, c := range archives {
		if failed[c.Name] {
			held = append(held, c)
		} else {
			ready = append(ready, c)
		}
	}

	var results []repository.ApplyResult
	if len(ready) > 0 {
		results = eng.repo.Apply(ctx, ready, repos, view.reporter())
	}
	if len(held) > 0 {
		reporter := view.reporter()
		for _, c := range held {
			reporter.Error(c.Name, 0, errNotArchived)
			results = append(results, repository.ApplyResult{Change: c, Err: errNotArchived})
		}
	}
	return results
}

// changedRepoNames returns the repositories with changes to apply, in order.
func changedRepoNames(changes []repository.Change) []string {
	var names []string
	for _, c := range changes {
		if c.Type != repository.ChangeNoOp {
			names = append(names, c.Name)
		}
	}
	return uniqueStrings(names)
}

// fileSetWork is what one FileSet applies.
type fileSetWork struct {
	changes []fileset.Change
	opts    fileset.ApplyOptions
	targets []string // repositories with changes, each reported once
}

// fileSetWorkOf returns the work of each FileSet with changes to apply, in
// manifest order.
func fileSetWorkOf(result *PlanResult) []fileSetWork {
	var work []fileSetWork
	for _, fs := range result.Parsed.FileSets {
		changes, opts := fileSetApplyArgs(fs, result.FileChanges)
		if len(changes) == 0 {
			continue
		}
		var targets []string
		for _, c := range changes {
			targets = append(targets, c.Target)
		}
		work = append(work, fileSetWork{changes: changes, opts: opts, targets: uniqueStrings(targets)})
	}
	return work
}

// applyFileSets applies the FileSets in parallel and returns their results
// in the order of work.
func applyFileSets(ctx context.Context, eng *engine, work []fileSetWork, view applyView) []fileset.ApplyResult {
	results := make([][]fileset.ApplyResult, len(work))
	var wg sync.WaitGroup
	for i, w := range work {
		wg.Go(func() {
			results[i] = eng.file.Apply(ctx, w.changes, w.opts, view.reporter())
		})
	}
	wg.Wait()
	return slices.Concat(results...)
}

// applyTasks returns the spinner tasks of an apply: a repository is done once
// per step that reports it (its changes, each FileSet changing it, its archive).
func applyTasks(repoNames []string, fileSets []fileSetWork, archiveNames []string) []ui.RefreshTask {
	names := slices.Clone(repoNames)
	for _, w := range fileSets {
		names = append(names, w.targets...)
	}
	names = append(names, archiveNames...)
	tasks := refreshTasks(names, "")
	for i := range tasks {
		tasks[i].WaitLabel = "waiting for the next step..."
	}
	return tasks
}

// refreshTasks returns a spinner task per name, in order of first
// appearance. A name given n times is done after n reports.
func refreshTasks(names []string, doneLabel string) []ui.RefreshTask {
	pending := make(map[string]int)
	var order []string
	for _, n := range names {
		if pending[n] == 0 {
			order = append(order, n)
		}
		pending[n]++
	}
	tasks := make([]ui.RefreshTask, 0, len(order))
	for _, n := range order {
		tasks = append(tasks, ui.RefreshTask{Name: n, DoneLabel: doneLabel, Pending: pending[n]})
	}
	return tasks
}

// checkFileChangesMatched returns an error if a change with work to do was not
// planned by one of fileSets. Apply hands each FileSet only its own changes, so
// such a change would otherwise be dropped without a word.
func checkFileChangesMatched(fileSets []*manifest.FileSet, changes []fileset.Change) error {
	planned := make(map[*manifest.FileSet]bool, len(fileSets))
	for _, fs := range fileSets {
		planned[fs] = true
	}
	for _, c := range changes {
		if c.Type != fileset.ChangeNoOp && !planned[c.FileSet] {
			return fmt.Errorf("internal error: planned %s of %s in %s belongs to no parsed FileSet", c.Type, c.Path, c.Target)
		}
	}
	return nil
}

func fileSetApplyArgs(fs *manifest.FileSet, allChanges []fileset.Change) ([]fileset.Change, fileset.ApplyOptions) {
	var fsChanges []fileset.Change
	for _, c := range allChanges {
		// Plan and Apply share the *FileSet pointers in parsed.FileSets.
		// A target with nothing to apply is left out, so each target
		// reports once per FileSet that changes it.
		if c.FileSet == fs && c.Type != fileset.ChangeNoOp {
			fsChanges = append(fsChanges, c)
		}
	}
	opts := fileset.ApplyOptions{
		CommitMessage: fs.Spec.CommitMessage,
		Via:           fs.Spec.Via,
		Branch:        fs.Spec.Branch,
		FileSetID:     fs.Identity(),
		PRTitle:       fs.Spec.PRTitle,
		PRBody:        fs.Spec.PRBody,
		SourceURL:     os.Getenv("GH_INFRA_SOURCE_URL"),
	}
	return fsChanges, opts
}

// countFileResults counts applied and failed file results. Skipped results
// (no commit was needed) count as neither.
func countFileResults(results []fileset.ApplyResult) (succeeded, failed int) {
	for _, r := range results {
		switch {
		case r.Err != nil:
			failed++
		case r.Skipped:
		default:
			succeeded++
		}
	}
	return
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
