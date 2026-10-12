package infra

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

type fakeCanceler struct{ ch chan struct{} }

func (f fakeCanceler) Canceled() <-chan struct{} { return f.ch }

func waitDone(t *testing.T, ctx context.Context, what string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("%s did not cancel the context", what)
	}
}

func TestWithTrackerCancel(t *testing.T) {
	t.Run("Ctrl+C", func(t *testing.T) {
		c := fakeCanceler{ch: make(chan struct{})}
		ctx, cancel := withTrackerCancel(context.Background(), c)
		defer cancel()
		close(c.ch)
		waitDone(t, ctx, "the tracker")
	})
	t.Run("parent", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.Background())
		ctx, cancel := withTrackerCancel(parent, fakeCanceler{ch: make(chan struct{})})
		defer cancel()
		cancelParent()
		waitDone(t, ctx, "the parent")
	})
	t.Run("no spinner", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.Background())
		ctx, cancel := withTrackerCancel(parent, fakeCanceler{})
		defer cancel()
		cancelParent()
		waitDone(t, ctx, "the parent")
	})
}

func TestStartRefresh_ParentCancelInterruptsTracker(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	tracker, ctx, stop := startRefresh(parent, nil, []ui.RefreshTask{{Name: "o/r"}})
	defer stop()

	cancel()
	waitDone(t, ctx, "the parent")
	// Interrupt runs in a goroutine of its own: report errors until one is
	// no longer collected.
	deadline := time.Now().Add(time.Second)
	for {
		n := len(tracker.Errors())
		tracker.Error("o/r", errors.New("gh exited with code -1"))
		if len(tracker.Errors()) == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("errors after the parent was canceled are still collected")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// cancelingRunner cancels the run on its first command and fails it as an
// interrupted gh process would (not with context.Canceled). It records every
// command.
type cancelingRunner struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	calls  []string
}

func (r *cancelingRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(args, " "))
	first := len(r.calls) == 1
	r.mu.Unlock()
	if first {
		r.cancel()
	}
	return nil, errors.New("signal: killed")
}

func (r *cancelingRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func newCancelingRunner(t *testing.T) (context.Context, *cancelingRunner) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx, &cancelingRunner{cancel: cancel}
}

const cancelTestManifest = `apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: o
  name: r
spec:
  description: d
`

func writeCancelTestManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "repo.yaml"), []byte(cancelTestManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The tests below run without a terminal, so the spinner falls back to plain
// progress and cannot be interrupted by a key: only ctx stops the work.

func TestPlan_Canceled(t *testing.T) {
	ctx, runner := newCancelingRunner(t)
	_, err := Plan(ctx, PlanOptions{Runner: runner, Paths: []string{writeCancelTestManifest(t)}, DryRun: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// ctxRunner fails with the context's error once it is canceled.
type ctxRunner struct{}

func (ctxRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected call")
}

func (r ctxRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

// Fetching a github:// source while parsing stops with ctx.
func TestPlan_CanceledWhileParsing(t *testing.T) {
	dir := t.TempDir()
	doc := `apiVersion: gh-infra/v1
kind: File
metadata:
  owner: o
  name: r
spec:
  files:
    - path: remote.txt
      source: github://o/src/remote.txt
`
	if err := os.WriteFile(filepath.Join(dir, "files.yaml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Plan(ctx, PlanOptions{Runner: ctxRunner{}, Paths: []string{dir}, DryRun: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled from the github:// fetch", err)
	}
}

func TestImport_Canceled(t *testing.T) {
	ctx, runner := newCancelingRunner(t)
	err := Import(ctx, runner, []string{"o/r"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestImportInto_Canceled(t *testing.T) {
	ctx, runner := newCancelingRunner(t)
	_, err := ImportInto(ctx, runner, nil, writeCancelTestManifest(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestApply_Canceled(t *testing.T) {
	for _, stream := range []bool{false, true} {
		ctx, runner := newCancelingRunner(t)
		repo := &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
			Spec: manifest.RepositorySpec{Variables: []manifest.Variable{
				{Name: "A", Value: "1"}, {Name: "B", Value: "2"},
			}},
		}
		var changes []repository.Change
		for _, v := range repo.Spec.Variables {
			changes = append(changes, repository.Change{
				Type: repository.ChangeCreate, Resource: manifest.ResourceVariable, Name: "o/r", Field: v.Name, NewValue: v.Value,
			})
		}
		result := &PlanResult{
			RepoChanges: changes,
			TargetRepos: []*manifest.Repository{repo},
			Parsed:      &manifest.ParseResult{Repositories: []*manifest.Repository{repo}},
			engine:      newEngine(runner, nil, discardPrinter()),
		}

		_, err := Apply(ctx, result, ApplyOptions{Stream: stream})

		if !errors.Is(err, context.Canceled) {
			t.Errorf("stream=%v: err = %v, want context.Canceled", stream, err)
		}
		if len(runner.calls) != 1 {
			t.Errorf("stream=%v: gh calls = %q, want none after the cancel", stream, runner.calls)
		}
	}
}
