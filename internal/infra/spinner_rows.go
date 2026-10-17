package infra

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// fetchHeading is the spinner heading of plan and import --into, which
// fetch the current state of the repositories they read. failed names a
// repository whose state could not be fetched.
func fetchHeading(failed string) ui.RefreshHeading {
	return ui.RefreshHeading{
		Title:  "Fetching current state from GitHub API",
		Phase:  "Fetching current state from GitHub API ...",
		Done:   "Fetched",
		Failed: failed,
	}
}

// planRows shows what the plan of each repository found on its spinner row.
// A row is reported by the repository plan and by each FileSet planning the
// repository; its result adds up what they found, and the spinner shows the
// result sent with the last report.
type planRows struct {
	tracker *ui.RefreshTracker
	mu      sync.Mutex // also keeps each row's reports in order
	rows    map[string]*planRow
}

type planRow struct {
	repo  repository.PlanSummary
	files int
	viaPR bool
}

func newPlanRows(tracker *ui.RefreshTracker) *planRows {
	return &planRows{tracker: tracker, rows: make(map[string]*planRow)}
}

// done adds one report to the row of name and sends the row's result so far.
func (r *planRows) done(name string, add func(*planRow)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[name]
	if row == nil {
		row = &planRow{}
		r.rows[name] = row
	}
	add(row)
	mark, label := row.result()
	r.tracker.DoneWith(name, mark, label)
}

// result is how the row shows what was found: the counts of changes and
// files, as in "4 changes · 3 files", or what happens to the repository.
func (row *planRow) result() (ui.Mark, string) {
	switch {
	case row.repo.NewRepository:
		return ui.MarkCreate, "will be created"
	case row.repo.DeleteRepository:
		return ui.MarkDelete, "will be destroyed"
	}
	var parts []string
	if n := row.repo.Creates + row.repo.Updates + row.repo.Deletes; n > 0 {
		parts = append(parts, plural(n, "change"))
	}
	if row.files > 0 {
		files := plural(row.files, "file")
		if row.viaPR {
			files += " (pull request)"
		}
		parts = append(parts, files)
	}
	if len(parts) == 0 {
		return ui.MarkNone, "up to date"
	}
	return ui.MarkChange, strings.Join(parts, " · ")
}

// repoPlanRows is planRows as the repository plan reports to it.
type repoPlanRows struct{ *planRows }

func (r repoPlanRows) UpdateStatus(name, status string) { r.tracker.UpdateStatus(name, status) }
func (r repoPlanRows) Checkpoint(name, status string)   { r.tracker.Checkpoint(name, status) }
func (r repoPlanRows) Error(name string, err error)     { r.tracker.Error(name, err) }
func (r repoPlanRows) Done(name string, s repository.PlanSummary) {
	r.done(name, func(row *planRow) { row.repo = s })
}

// filePlanRows is planRows as a FileSet plan reports to it.
type filePlanRows struct{ *planRows }

func (r filePlanRows) UpdateStatus(name, status string) { r.tracker.UpdateStatus(name, status) }
func (r filePlanRows) Error(name string, err error)     { r.tracker.Error(name, err) }
func (r filePlanRows) Done(name string, s fileset.PlanSummary) {
	r.done(name, func(row *planRow) {
		row.files += s.Files
		row.viaPR = row.viaPR || s.Via == manifest.ViaPullRequest
	})
}

// planTasks returns the spinner tasks of a plan: a row per repository, done
// once each repository plan and FileSet planning it has reported. An error
// leaves the repository out of the plan, which its row says.
func planTasks(names []string) []ui.RefreshTask {
	tasks := refreshTasks(names, "")
	for i := range tasks {
		tasks[i].ErrorPrefix = "skipped"
	}
	return tasks
}

// applyRows shows what was applied to each repository on its spinner row. A
// row is reported by each step that changes the repository (its settings,
// each FileSet, its archive), which report how each change ended (Step);
// its label adds them up.
type applyRows struct {
	tracker    *ui.RefreshTracker
	totals     map[string]int  // changes to apply per repository
	createOnly map[string]bool // repositories whose only change is creating them
	mu         sync.Mutex      // also keeps each row's reports in order
	rows       map[string]*applyRow
}

type applyRow struct {
	applied, failed, skipped int
	errored                  bool   // a step reported an error, as when canceled
	pullRequest              string // URL of the pull request opened, if any
}

func newApplyRows(tracker *ui.RefreshTracker, totals map[string]int, createOnly map[string]bool) *applyRows {
	return &applyRows{tracker: tracker, totals: totals, createOnly: createOnly, rows: make(map[string]*applyRow)}
}

func (r *applyRows) row(name string) *applyRow {
	row := r.rows[name]
	if row == nil {
		row = &applyRow{}
		r.rows[name] = row
	}
	return row
}

func (r *applyRows) Start(name string, fields []string) {
	if len(fields) > 0 {
		r.tracker.UpdateStatus(name, "applying "+fields[0]+"...")
	}
}

func (r *applyRows) UpdateStatus(name, status string) { r.tracker.UpdateStatus(name, status) }

// step counts how one change of name ended.
func (r *applyRows) step(name string, result ui.StepResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.row(name)
	switch result {
	case ui.StepApplied:
		row.applied++
	case ui.StepFailed:
		row.failed++
	case ui.StepSkipped:
		row.skipped++
	}
	r.tracker.Step(name, result)
}

// pullRequest records the pull request opened for name.
func (r *applyRows) pullRequest(name, url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.row(name).pullRequest = url
}

// Done and Error end one step of name's row and send the row's result so
// far. An error does not end the row: it keeps running until its other
// steps end, and then shows how many changes failed. Without a terminal
// the error is printed as it happens, as before the rows counted changes.
func (r *applyRows) Done(name string, _ time.Duration, _ int) { r.end(name, false) }

func (r *applyRows) Error(name string, _ time.Duration, err error) {
	if !r.tracker.Interactive() {
		r.tracker.Error(name, err)
	}
	r.end(name, true)
}

func (r *applyRows) end(name string, errored bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.row(name)
	row.errored = row.errored || errored
	mark, label := r.result(name, row)
	r.tracker.DoneWith(name, mark, label)
}

// result is how name's row shows what was applied: "7 applied",
// "4 applied, 1 skipped", "created", or "6 of 7 applied, 1 failed", and the
// pull request it opened.
func (r *applyRows) result(name string, row *applyRow) (ui.Mark, string) {
	var label string
	mark := ui.MarkDone
	switch {
	case row.failed > 0 || row.errored:
		mark = ui.MarkFailed
		label = fmt.Sprintf("%d of %d applied", row.applied, r.totals[name])
		if row.failed > 0 {
			label += fmt.Sprintf(", %d failed", row.failed)
		}
	case r.createOnly[name] && row.applied == 1:
		label = "created"
	case row.applied > 0:
		label = fmt.Sprintf("%d applied", row.applied)
	}
	if row.skipped > 0 {
		if label != "" {
			label += ", "
		}
		label += fmt.Sprintf("%d skipped", row.skipped)
	}
	if n := pullRequestNumber(row.pullRequest); n != "" {
		label += " · opened pull request #" + n
	}
	return mark, label
}

func (r *applyRows) Wait() {}

// pullRequestNumber returns the number at the end of a pull request URL.
func pullRequestNumber(url string) string {
	_, n, ok := strings.Cut(url, "/pull/")
	if !ok {
		return ""
	}
	return strings.TrimRight(n, "/")
}

// repoApplyProgress is what the repository apply reports to: the spinner's
// applyRows, or the stream's lines (rows is nil). It adds the repository's
// Step to them.
type repoApplyProgress struct {
	ui.ProgressReporter
	rows *applyRows
}

func (r repoApplyProgress) Step(name string, o repository.StepOutcome) {
	if r.rows == nil {
		return
	}
	switch o {
	case repository.StepFailed:
		r.rows.step(name, ui.StepFailed)
	case repository.StepSkipped:
		r.rows.step(name, ui.StepSkipped)
	default:
		r.rows.step(name, ui.StepApplied)
	}
}

// fileApplyProgress is repoApplyProgress for a FileSet's apply, which also
// reports the pull requests it opens.
type fileApplyProgress struct {
	ui.ProgressReporter
	rows *applyRows
}

func (r fileApplyProgress) Step(name string, o fileset.StepOutcome) {
	if r.rows == nil {
		return
	}
	switch o {
	case fileset.StepFailed:
		r.rows.step(name, ui.StepFailed)
	case fileset.StepSkipped:
		r.rows.step(name, ui.StepSkipped)
	default:
		r.rows.step(name, ui.StepApplied)
	}
}

func (r fileApplyProgress) PullRequest(name, url string) {
	if r.rows != nil {
		r.rows.pullRequest(name, url)
	}
}

// applyTotals counts the changes to apply per repository: its settings,
// archive and files, without those the user chose to skip (no-ops).
func applyTotals(repoChanges []repository.Change, fileChanges []fileset.Change) map[string]int {
	totals := make(map[string]int)
	for _, c := range repoChanges {
		if c.Type != repository.ChangeNoOp {
			totals[c.Name]++
		}
	}
	for _, c := range fileChanges {
		if c.Type != fileset.ChangeNoOp {
			totals[c.Target]++
		}
	}
	return totals
}

// createOnlyRepos returns the repositories whose only change is creating
// them: no settings after the create and no files.
func createOnlyRepos(repoChanges []repository.Change, fileChanges []fileset.Change) map[string]bool {
	counts := make(map[string]int)
	created := createdRepos(repoChanges)
	for _, c := range repoChanges {
		if c.Type != repository.ChangeNoOp {
			counts[c.Name]++
		}
	}
	for _, c := range fileChanges {
		if c.Type != fileset.ChangeNoOp {
			counts[c.Target]++
		}
	}
	only := make(map[string]bool)
	for name := range created {
		if counts[name] == 1 {
			only[name] = true
		}
	}
	return only
}
