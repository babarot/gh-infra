package infra

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// orderRunner records the commands it runs and fails those starting with
// one of fail.
type orderRunner struct {
	mu    sync.Mutex
	calls []string
	fail  []string
}

func (r *orderRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, cmd)
	r.mu.Unlock()
	for _, f := range r.fail {
		if strings.HasPrefix(cmd, f) {
			return nil, errors.New("boom")
		}
	}
	return nil, nil
}

func (r *orderRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

// archiveResult plans a description change and the archive of o/r, and a
// FileSet file for o/r, in Diff's order (archive last).
func archiveResult(runner *orderRunner) *PlanResult {
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec:     manifest.RepositorySpec{Description: manifest.Ptr("old"), Archived: manifest.Ptr(true)},
	}
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "o", Name: "files"},
		Spec:     manifest.FileSetSpec{Repositories: []manifest.FileSetRepository{{Name: "r"}}, Via: manifest.ViaPush},
	}
	return &PlanResult{
		RepoChanges: []repository.Change{
			{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/r", Field: "description", NewValue: "old"},
			{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/r", Field: "archived", OldValue: false, NewValue: true},
		},
		FileChanges: []fileset.Change{{FileSet: fs, FileSetID: fs.Identity(), Target: "o/r", Path: "f.txt", Type: fileset.ChangeCreate, Desired: "x", Via: manifest.ViaPush}},
		TargetRepos: []*manifest.Repository{repo},
		Parsed:      &manifest.ParseResult{Repositories: []*manifest.Repository{repo}, FileSets: []*manifest.FileSet{fs}},
		engine:      newEngine(runner, nil, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{})),
	}
}

func lastIndex(calls []string, prefix string) int {
	for i, c := range slices.Backward(calls) {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

// The archive goes after the repository's other changes and its FileSets.
func TestApply_ArchivesLast(t *testing.T) {
	for _, stream := range []bool{false, true} {
		runner := &orderRunner{}
		if _, err := Apply(context.Background(), archiveResult(runner), ApplyOptions{Stream: stream}); err != nil {
			t.Fatalf("stream=%v: %v", stream, err)
		}
		archive := lastIndex(runner.calls, "repo archive o/r")
		if archive < 0 || archive != len(runner.calls)-1 {
			t.Errorf("stream=%v: archive is not the last command: %q", stream, runner.calls)
		}
		if lastIndex(runner.calls, "repo edit o/r --description") < 0 || lastIndex(runner.calls, "repo view o/r --json defaultBranchRef") < 0 {
			t.Errorf("stream=%v: the description or the FileSet did not run: %q", stream, runner.calls)
		}
	}
}

// A repository with a failed change or FileSet is not archived, so the next
// apply can still fix it; the archive is reported as failed.
func TestApply_DoesNotArchiveAfterAFailure(t *testing.T) {
	for _, tt := range []struct {
		name string
		fail string
	}{
		{"repository change", "repo edit o/r --description"},
		{"FileSet", "repo view o/r --json defaultBranchRef"},
	} {
		for _, stream := range []bool{false, true} {
			runner := &orderRunner{fail: []string{tt.fail}}
			outcome, err := Apply(context.Background(), archiveResult(runner), ApplyOptions{Stream: stream})
			if err == nil {
				t.Errorf("%s, stream=%v: want an error", tt.name, stream)
			}
			if lastIndex(runner.calls, "repo archive") >= 0 {
				t.Errorf("%s, stream=%v: archived after a failure: %q", tt.name, stream, runner.calls)
			}
			var archiveErr error
			for _, r := range outcome.RepoResults {
				if r.Change.Field == "archived" {
					archiveErr = r.Err
				}
			}
			if !errors.Is(archiveErr, errNotArchived) {
				t.Errorf("%s, stream=%v: archive result = %v, want %v", tt.name, stream, archiveErr, errNotArchived)
			}
		}
	}
}

// The repository's line is done after its changes, its FileSet and its
// archive have each reported.
func TestApplyTasks_CountsTheArchiveStep(t *testing.T) {
	result := archiveResult(&orderRunner{})
	others, archives := splitArchives(result.RepoChanges)
	tasks := applyTasks(changedRepoNames(others), fileSetWorkOf(result), changedRepoNames(archives))
	want := []ui.RefreshTask{{Name: "o/r", Pending: 3, WaitLabel: "waiting for the next step..."}}
	if !slices.Equal(tasks, want) {
		t.Errorf("tasks = %+v, want %+v", tasks, want)
	}
}
