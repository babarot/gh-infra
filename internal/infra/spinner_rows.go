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
// each FileSet, its archive); its label adds up what they applied.
type applyRows struct {
	tracker    *ui.RefreshTracker
	createOnly map[string]bool // repositories whose only change is creating them
	mu         sync.Mutex      // also keeps each row's reports in order
	applied    map[string]int
}

func newApplyRows(tracker *ui.RefreshTracker, createOnly map[string]bool) *applyRows {
	return &applyRows{tracker: tracker, createOnly: createOnly, applied: make(map[string]int)}
}

func (r *applyRows) Start(name string, fields []string) {
	if len(fields) > 0 {
		r.tracker.UpdateStatus(name, "applying "+fields[0]+"...")
	}
}

func (r *applyRows) UpdateStatus(name, status string) { r.tracker.UpdateStatus(name, status) }

func (r *applyRows) Done(name string, _ time.Duration, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracker.DoneWith(name, ui.MarkDone, r.add(name, count))
}

// add counts what one step applied to name and returns the row's label.
func (r *applyRows) add(name string, count int) string {
	r.applied[name] += count
	if r.createOnly[name] {
		return "created"
	}
	return fmt.Sprintf("%d applied", r.applied[name])
}

func (r *applyRows) Error(name string, _ time.Duration, err error) { r.tracker.Error(name, err) }

func (r *applyRows) Wait() {}

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
