package infra

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func TestPlanRowResult(t *testing.T) {
	tests := []struct {
		name      string
		repo      repository.PlanSummary
		files     []fileset.PlanSummary
		wantMark  ui.Mark
		wantLabel string
	}{
		{name: "nothing to change", wantMark: ui.MarkNone, wantLabel: "up to date"},
		{name: "settings and files",
			repo:     repository.PlanSummary{Creates: 1, Updates: 3},
			files:    []fileset.PlanSummary{{Files: 2, Via: manifest.ViaPush}, {Files: 1, Via: manifest.ViaPush}},
			wantMark: ui.MarkChange, wantLabel: "4 changes · 3 files"},
		{name: "one change and files by pull request",
			repo:     repository.PlanSummary{Deletes: 1},
			files:    []fileset.PlanSummary{{Files: 2, Via: manifest.ViaPullRequest}},
			wantMark: ui.MarkChange, wantLabel: "1 change · 2 files (pull request)"},
		{name: "files only",
			files:    []fileset.PlanSummary{{Files: 1, Via: manifest.ViaPush}, {}},
			wantMark: ui.MarkChange, wantLabel: "1 file"},
		{name: "a new repository",
			repo:     repository.PlanSummary{Creates: 1, NewRepository: true},
			files:    []fileset.PlanSummary{{Files: 2, Via: manifest.ViaPush}},
			wantMark: ui.MarkCreate, wantLabel: "will be created"},
		{name: "a repository to delete",
			repo:     repository.PlanSummary{Deletes: 1, DeleteRepository: true},
			wantMark: ui.MarkDelete, wantLabel: "will be destroyed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := newPlanRows(nil) // a nil tracker shows nothing
			repoPlanRows{rows}.Done("o/a", tt.repo)
			for _, f := range tt.files {
				filePlanRows{rows}.Done("o/a", f)
			}
			mark, label := rows.rows["o/a"].result()
			if mark != tt.wantMark || label != tt.wantLabel {
				t.Errorf("got (%v, %q), want (%v, %q)", mark, label, tt.wantMark, tt.wantLabel)
			}
		})
	}
}

func TestSummarizePlans(t *testing.T) {
	repo := repository.SummarizePlan([]repository.Change{
		{Type: repository.ChangeCreate, Field: "repository"},
		{Type: repository.ChangeUpdate, Field: "description"},
		{Type: repository.ChangeNoOp, Field: "topics"},
	})
	if want := (repository.PlanSummary{Creates: 1, Updates: 1, NewRepository: true}); repo != want {
		t.Errorf("repository: got %+v, want %+v", repo, want)
	}
	files := fileset.SummarizePlan([]fileset.Change{
		{Type: fileset.ChangeNoOp, Via: manifest.ViaPullRequest},
		{Type: fileset.ChangeUpdate, Via: manifest.ViaPush},
		{Type: fileset.ChangeCreate, Via: manifest.ViaPush},
	})
	if want := (fileset.PlanSummary{Files: 2, Via: manifest.ViaPush}); files != want {
		t.Errorf("files: got %+v, want %+v", files, want)
	}
}

func TestPlanTasksSayWhatAnErrorMeans(t *testing.T) {
	tasks := planTasks([]string{"o/a", "o/b", "o/a"})
	if len(tasks) != 2 || tasks[0].Pending != 2 || tasks[0].ErrorPrefix != "skipped" || tasks[1].ErrorPrefix != "skipped" {
		t.Errorf("tasks = %+v", tasks)
	}
}

func TestApplyRowsLabel(t *testing.T) {
	createOnly := createOnlyRepos([]repository.Change{
		{Type: repository.ChangeCreate, Name: "o/new", Resource: manifest.ResourceRepository, Field: "repository"},
		{Type: repository.ChangeCreate, Name: "o/new2", Resource: manifest.ResourceRepository, Field: "repository"},
		{Type: repository.ChangeUpdate, Name: "o/a", Field: "description"},
	}, []fileset.Change{
		{Type: fileset.ChangeCreate, Target: "o/new2", Path: "README.md"},
	})
	if want := map[string]bool{"o/new": true}; !maps.Equal(createOnly, want) {
		t.Fatalf("createOnlyRepos = %v, want %v (o/new2 also gets a file)", createOnly, want)
	}

	totals := map[string]int{"o/a": 7, "o/b": 5, "o/c": 2, "o/new": 1, "o/x": 3}
	rows := newApplyRows(nil, totals, createOnly) // a nil tracker shows nothing
	steps := func(name string, results ...ui.StepResult) {
		for _, r := range results {
			rows.step(name, r)
		}
	}
	a, f, sk := ui.StepApplied, ui.StepFailed, ui.StepSkipped
	steps("o/a", a, a, a, f, a, a, a)
	steps("o/b", a, a, a, a, sk)
	steps("o/c", sk, sk)
	steps("o/new", a)
	steps("o/x", a) // then canceled: an error with no failed step
	rows.pullRequest("o/b", "https://github.com/acme/b/pull/42")
	rows.Error("o/x", 0, errors.New("context canceled"))

	for name, want := range map[string]struct {
		mark  ui.Mark
		label string
	}{
		"o/a":   {ui.MarkFailed, "6 of 7 applied, 1 failed"},
		"o/b":   {ui.MarkDone, "4 applied, 1 skipped · opened pull request #42"},
		"o/c":   {ui.MarkDone, "2 skipped"},
		"o/new": {ui.MarkDone, "created"},
		"o/x":   {ui.MarkFailed, "1 of 3 applied"},
	} {
		mark, label := rows.result(name, rows.rows[name])
		if mark != want.mark || label != want.label {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", name, mark, label, want.mark, want.label)
		}
	}
}

// A held archive is not applied: it counts as a failed change of its row,
// so the row's n/m reaches its total and its label says it failed.
func TestApplyArchives_HeldArchiveIsAFailedStep(t *testing.T) {
	rows := newApplyRows(nil, map[string]int{"o/r": 2}, nil)
	view := applyView{rows: rows}
	archive := repository.Change{Type: repository.ChangeUpdate, Name: "o/r", Field: "archived", OldValue: false, NewValue: true}
	failedBefore := []repository.ApplyResult{{Change: repository.Change{Name: "o/r", Field: "description"}, Err: errors.New("boom")}}
	rows.step("o/r", ui.StepFailed) // the description, which failed

	results := applyArchives(context.Background(), nil, []repository.Change{archive}, nil, failedBefore, nil, view)
	if len(results) != 1 || !errors.Is(results[0].Err, errNotArchived) {
		t.Fatalf("results = %+v, want the archive held", results)
	}
	if row := rows.rows["o/r"]; row.failed != 2 {
		t.Errorf("failed steps = %d, want 2 (the description and the held archive)", row.failed)
	}
	if mark, label := rows.result("o/r", rows.rows["o/r"]); mark != ui.MarkFailed || label != "0 of 2 applied, 2 failed" {
		t.Errorf("result = (%v, %q)", mark, label)
	}
}

func TestPrintApplyFollowUps(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)
	printApplyFollowUps(p, []repository.ApplyResult{
		{Change: repository.Change{Name: "o/a", Field: "description"}},
		{Change: repository.Change{Name: "o/a", Field: `ruleset "main"`}, Err: errors.New("HTTP 422: Validation Failed")},
	}, []fileset.ApplyResult{
		{Change: fileset.Change{Target: "o/b", Path: "CODEOWNERS"}, PRURL: "https://github.com/o/b/pull/42"},
		{Change: fileset.Change{Target: "o/c", Path: "LICENSE"}},
	})
	// Fields are padded as in the full list (printApplyResults).
	want := "  ✗ o/a\n      ✗ ruleset \"main\"      HTTP 422: Validation Failed\n\n  ↗ o/b  pull request #42  https://github.com/o/b/pull/42\n"
	if got := buf.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	buf.Reset()
	printApplyFollowUps(p, []repository.ApplyResult{{Change: repository.Change{Name: "o/a", Field: "description"}}}, nil)
	if buf.Len() != 0 {
		t.Errorf("all applied, no pull request: want nothing, got\n%s", buf.String())
	}
}
