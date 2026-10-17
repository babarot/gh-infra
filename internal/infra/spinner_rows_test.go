package infra

import (
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

	rows := newApplyRows(nil, createOnly)
	if got := rows.add("o/a", 3); got != "3 applied" {
		t.Errorf("settings step: got %q", got)
	}
	if got := rows.add("o/a", 2); got != "5 applied" {
		t.Errorf("after the FileSet step: got %q, want the steps added up", got)
	}
	if got := rows.add("o/new", 1); got != "created" {
		t.Errorf("create only: got %q", got)
	}
}
