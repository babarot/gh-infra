package infra

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// stepsRunner answers the gh commands of an apply the way GitHub would, so
// FileSets get as far as a commit: a default branch, its HEAD, and a tree
// that differs from HEAD's (or not, with sameTree). A command starting with
// one of fail fails; a repository in empty has no commits.
type stepsRunner struct {
	fail     []string
	empty    []string
	sameTree bool
	mu       sync.Mutex
}

func (r *stepsRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	return r.run(args)
}

func (r *stepsRunner) RunWithStdin(_ context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.run(args)
}

func (r *stepsRunner) run(args []string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := strings.Join(args, " ")
	for _, prefix := range r.fail {
		if strings.HasPrefix(cmd, prefix) {
			return nil, errors.New("boom: " + cmd)
		}
	}
	switch {
	case strings.HasPrefix(cmd, "repo view") && strings.Contains(cmd, "defaultBranchRef"):
		for _, repo := range r.empty {
			if strings.HasPrefix(cmd, "repo view "+repo+" ") {
				return []byte(""), nil
			}
		}
		return []byte("main"), nil
	case strings.Contains(cmd, "/git/ref/heads/main"):
		return []byte("head123"), nil
	case strings.Contains(cmd, "/git/commits/head123 --jq .tree.sha"):
		return []byte("base-tree\n"), nil
	case strings.Contains(cmd, "/git/trees --method POST"):
		if r.sameTree {
			return []byte("base-tree\n"), nil
		}
		return []byte("new-tree\n"), nil
	}
	return nil, nil
}

// stepsPlan is a plan for the step counting test: settings changes on o/r
// (a create first when create is set, an archive last when archive is set)
// and a FileSet of two files on o/r.
func stepsPlan(runner *stepsRunner, printer ui.Printer, create, archive bool, files []string) *PlanResult {
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec: manifest.RepositorySpec{Variables: []manifest.Variable{
			{Name: "A", Value: "1"}, {Name: "B", Value: "2"},
		}},
	}
	var repoChanges []repository.Change
	if create {
		repoChanges = append(repoChanges, repository.Change{Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: "o/r", Field: "repository", NewValue: "o/r"})
	}
	for _, v := range repo.Spec.Variables {
		repoChanges = append(repoChanges, repository.Change{
			Type: repository.ChangeCreate, Resource: manifest.ResourceVariable, Name: "o/r", Field: v.Name, NewValue: v.Value,
		})
	}
	if archive {
		archived := true
		repo.Spec.Archived = &archived
		repoChanges = append(repoChanges, repository.Change{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/r", Field: "archived", OldValue: false, NewValue: true})
	}
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "o", Name: "sync"},
		Spec:     manifest.FileSetSpec{Repositories: []manifest.FileSetRepository{{Name: "r"}}, Via: manifest.ViaPush},
	}
	var fileChanges []fileset.Change
	for _, path := range files {
		fileChanges = append(fileChanges, fileset.Change{FileSet: fs, FileSetID: fs.Identity(), Target: "o/r", Path: path, Type: fileset.ChangeCreate, Desired: path + "\n", Via: manifest.ViaPush})
	}
	return &PlanResult{
		RepoChanges: repoChanges,
		FileChanges: fileChanges,
		TargetRepos: []*manifest.Repository{repo},
		Parsed:      &manifest.ParseResult{Repositories: []*manifest.Repository{repo}, FileSets: []*manifest.FileSet{fs}},
		engine:      newEngine(runner, nil, printer),
	}
}

// TestApplySteps_AddUpToTotals checks that the steps an apply reports for
// each repository add up to the changes the spinner counts for it
// (applyTotals), however the changes end. A step left unreported would
// stop the heading at "Applied 5 of 6 changes" with nothing failed or
// skipped to explain it.
func TestApplySteps_AddUpToTotals(t *testing.T) {
	files := []string{"a.txt", "b.txt"}
	// want is how the steps of o/r end (applied, failed, skipped), which
	// shows each case takes the path it is named after.
	tests := []struct {
		name    string
		runner  *stepsRunner
		create  bool
		archive bool
		want    [3]int
	}{
		{"everything applies", &stepsRunner{}, false, false, [3]int{4, 0, 0}},
		{"the FileSet commit fails", &stepsRunner{fail: []string{"api graphql"}}, false, false, [3]int{2, 2, 0}},
		{"GitHub has the files already", &stepsRunner{sameTree: true}, false, false, [3]int{2, 0, 2}},
		{"the create fails, so the settings after it do not apply", &stepsRunner{fail: []string{"repo create"}}, true, false, [3]int{2, 3, 0}},
		{"the archive is held after a failure", &stepsRunner{fail: []string{"variable set A"}}, false, true, [3]int{3, 2, 0}},
		{"files are pushed to a repository with no commits", &stepsRunner{empty: []string{"o/r"}}, true, false, [3]int{5, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			printer := ui.NewStandardPrinterWith(&out, &out)
			restore := ui.SetDefaultPrinter(printer) // the spinner's fallback prints through it
			defer restore()

			result := stepsPlan(tt.runner, printer, tt.create, tt.archive, files)
			work := applyWorkOf(result)
			view, ctx, stop := startApplyView(context.Background(), false, result.engine, work.tasks, work.totals, work.createOnly)
			applySteps(ctx, result.engine, result, work, view)
			view.wait()
			stop()

			for name, total := range work.totals {
				row := view.rows.rows[name]
				if row == nil {
					t.Fatalf("no steps reported for %s, want %d", name, total)
				}
				if got := [3]int{row.applied, row.failed, row.skipped}; got != tt.want {
					t.Errorf("%s: applied, failed, skipped = %v, want %v", name, got, tt.want)
				}
				if ended := row.applied + row.failed + row.skipped; ended != total {
					t.Errorf("%s: %d applied + %d failed + %d skipped = %d steps, want %d (applyTotals)",
						name, row.applied, row.failed, row.skipped, ended, total)
				}
			}
		})
	}
}
