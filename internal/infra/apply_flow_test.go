package infra

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// The tests in this file run the same checks in both modes, so a change to
// one mode only fails them.

// flowRunner fails every command except setting variable A. It can hold
// the FileSet calls to see how many run at once, and report a rate limit
// wait on its first command.
type flowRunner struct {
	mu         sync.Mutex
	inFlight   int
	maxConc    int
	hold       time.Duration
	reportWait bool
	onWait     func(time.Time)
}

func (r *flowRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	r.mu.Lock()
	if r.reportWait && r.onWait != nil {
		r.reportWait = false
		r.onWait(time.Now().Add(time.Minute))
	}
	r.mu.Unlock()
	if strings.HasPrefix(cmd, "variable set A ") {
		return nil, nil
	}
	if strings.HasPrefix(cmd, "repo view") && r.hold > 0 {
		r.mu.Lock()
		r.inFlight++
		r.maxConc = max(r.maxConc, r.inFlight)
		r.mu.Unlock()
		time.Sleep(r.hold)
		r.mu.Lock()
		r.inFlight--
		r.mu.Unlock()
	}
	return nil, errors.New("boom: " + cmd)
}

func (r *flowRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

// SetOnWait makes flowRunner a rate limit waitNotifier.
func (r *flowRunner) SetOnWait(fn func(until time.Time)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onWait = fn
}

// flowResult is a plan with one repository (variables A, B and C; B and C
// fail) and two FileSets on other repositories, plus a FileSet whose only
// change has nothing to apply.
func flowResult(runner *flowRunner, printer ui.Printer) *PlanResult {
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec: manifest.RepositorySpec{Variables: []manifest.Variable{
			{Name: "A", Value: "1"}, {Name: "B", Value: "2"}, {Name: "C", Value: "3"},
		}},
	}
	var repoChanges []repository.Change
	for _, v := range repo.Spec.Variables {
		repoChanges = append(repoChanges, repository.Change{
			Type: repository.ChangeCreate, Resource: manifest.ResourceVariable, Name: "o/r", Field: v.Name, NewValue: v.Value,
		})
	}
	newFS := func(name, repo string) *manifest.FileSet {
		return &manifest.FileSet{
			Metadata: manifest.FileSetMetadata{Owner: "o", Name: name},
			Spec:     manifest.FileSetSpec{Repositories: []manifest.FileSetRepository{{Name: repo}}, Via: manifest.ViaPush},
		}
	}
	fs1, fs2, noop := newFS("one", "x"), newFS("two", "y"), newFS("noop", "x")
	change := func(fs *manifest.FileSet, target string, typ fileset.ChangeType) fileset.Change {
		return fileset.Change{FileSet: fs, FileSetID: fs.Identity(), Target: target, Path: "f.txt", Type: typ, Desired: "x", Via: manifest.ViaPush}
	}
	return &PlanResult{
		RepoChanges: repoChanges,
		FileChanges: []fileset.Change{
			change(fs1, "o/x", fileset.ChangeCreate),
			change(fs2, "o/y", fileset.ChangeCreate),
			change(noop, "o/x", fileset.ChangeNoOp),
		},
		TargetRepos: []*manifest.Repository{repo},
		Parsed: &manifest.ParseResult{
			Repositories: []*manifest.Repository{repo},
			FileSets:     []*manifest.FileSet{fs1, fs2, noop},
		},
		engine: newEngine(runner, nil, printer),
	}
}

func TestApply_SameResultsInBothModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		var out bytes.Buffer
		printer := ui.NewStandardPrinterWith(&out, &out)
		// Without a terminal the spinner falls back to lines through the
		// package printer.
		restore := ui.SetDefaultPrinter(printer)
		outcome, err := Apply(context.Background(), flowResult(&flowRunner{}, printer), ApplyOptions{Stream: stream})
		restore()

		if err == nil || err.Error() != "apply had errors" {
			t.Errorf("stream=%v: err = %v, want apply had errors", stream, err)
		}
		if outcome == nil {
			t.Fatalf("stream=%v: no outcome", stream)
		}
		rs, rf := repository.CountApplyResults(outcome.RepoResults)
		fs, ff := countFileResults(outcome.FileResults)
		if rs != 1 || rf != 2 || fs != 0 || ff != 2 {
			t.Errorf("stream=%v: repo %d ok %d failed, files %d ok %d failed; want 1/2 and 0/2", stream, rs, rf, fs, ff)
		}
		var targets []string
		for _, r := range outcome.FileResults {
			targets = append(targets, r.Change.Target)
		}
		if !slices.Equal(targets, []string{"o/x", "o/y"}) {
			t.Errorf("stream=%v: file results = %v, want o/x then o/y (FileSet order)", stream, targets)
		}
		// Both modes print the same summary, and with failures it does not
		// say the apply is complete.
		// One blank line before the summary: the results list (fallback mode)
		// ends with a pending blank line, which the summary's own replaces.
		// Without a terminal (go test), the spinner's fallback shows no
		// results, so every result is listed, the successful ones too.
		if !stream && !strings.Contains(lineWith(out.String(), "✓ A"), "created") {
			t.Errorf("stream=%v: want the full results list without a terminal:\n%s", stream, out.String())
		}
		// Without a terminal each error is also printed as it happens.
		if !stream && !strings.Contains(out.String(), "o/r  update o/r variable:B") {
			t.Errorf("stream=%v: want the error line printed during the apply:\n%s", stream, out.String())
		}
		if strings.Contains(out.String(), "\n\n\nApply finished") {
			t.Errorf("stream=%v: two blank lines before the summary:\n%s", stream, out.String())
		}
		if !strings.Contains(out.String(), "\n\nApply finished with errors: 1 applied, 4 failed\n") || strings.Contains(out.String(), "Apply complete!") {
			t.Errorf("stream=%v: summary missing:\n%s", stream, out.String())
		}
		if stream {
			// Every failure of the repository is shown, on its line.
			line := lineWith(out.String(), "o/r: Error after")
			if !strings.Contains(line, "variable:B") || !strings.Contains(line, "variable:C") {
				t.Errorf("stream line for o/r = %q, want both failures", line)
			}
		}
	}
}

func lineWith(s, sub string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestApply_FileSetsRunInParallelInBothModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		runner := &flowRunner{hold: 100 * time.Millisecond}
		printer := ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{})
		_, _ = Apply(context.Background(), flowResult(runner, printer), ApplyOptions{Stream: stream})
		if runner.maxConc < 2 {
			t.Errorf("stream=%v: at most %d FileSet at once, want them in parallel", stream, runner.maxConc)
		}
	}
}

func TestApply_ShowsRateLimitWaitsInBothModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		var out bytes.Buffer
		printer := ui.NewStandardPrinterWith(&out, &out)
		// Without a terminal the spinner falls back to lines through the
		// package printer.
		restore := ui.SetDefaultPrinter(printer)
		runner := &flowRunner{reportWait: true}
		_, _ = Apply(context.Background(), flowResult(runner, printer), ApplyOptions{Stream: stream})
		restore()
		if !strings.Contains(out.String(), "Waiting for the GitHub API rate limit until") {
			t.Errorf("stream=%v: no rate limit line:\n%s", stream, out.String())
		}
	}
}

func TestRefreshTasks(t *testing.T) {
	got := refreshTasks([]string{"o/a", "o/b", "o/a"}, "done")
	want := []ui.RefreshTask{{Name: "o/a", DoneLabel: "done", Pending: 2}, {Name: "o/b", DoneLabel: "done", Pending: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("refreshTasks = %+v, want %+v", got, want)
	}
}

// A repository is done once per FileSet that changes it: a FileSet with
// nothing to apply there does not count.
func TestFileSetWorkOf_CountsOnlyFileSetsWithChanges(t *testing.T) {
	result := flowResult(&flowRunner{}, discardPrinter())
	var names []string
	for _, w := range fileSetWorkOf(result) {
		names = append(names, w.targets...)
	}
	if !slices.Equal(names, []string{"o/x", "o/y"}) {
		t.Errorf("targets = %v, want o/x and o/y once each", names)
	}
}
