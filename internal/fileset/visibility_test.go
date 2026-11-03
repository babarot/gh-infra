package fileset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

// refusingRefsRunner refuses the first refusals ref creations with the 403
// GitHub returns while a visibility change is in progress, as the GHRunner
// returns it.
type refusingRefsRunner struct {
	*gh.MockRunner
	mu       sync.Mutex
	refusals int
	refused  int
}

func (r *refusingRefsRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if strings.HasPrefix(strings.Join(args, " "), "api repos/owner/repo/git/refs --method POST") {
		r.mu.Lock()
		refuse := r.refused < r.refusals
		if refuse {
			r.refused++
		}
		r.mu.Unlock()
		if refuse {
			_, _ = r.MockRunner.Run(ctx, args...) // recorded as a call
			return nil, fmt.Errorf("%w: Resource not accessible by personal access token (HTTP 403)", gh.ErrForbidden)
		}
	}
	return r.MockRunner.Run(ctx, args...)
}

func shortVisibilitySettle(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldTimeout, oldInterval := visibilitySettleTimeout, visibilitySettleInterval
	visibilitySettleTimeout, visibilitySettleInterval = timeout, time.Millisecond
	t.Cleanup(func() { visibilitySettleTimeout, visibilitySettleInterval = oldTimeout, oldInterval })
}

func applyWithRefusals(refusals int, visibilityChanged bool) (*refusingRefsRunner, []ApplyResult) {
	runner := &refusingRefsRunner{MockRunner: newNoopGuardMock("new-tree"), refusals: refusals}
	opts := ApplyOptions{FileSetID: "test", Via: manifest.ViaPullRequest}
	if visibilityChanged {
		opts.VisibilityChanged = map[string]bool{"owner/repo": true}
	}
	changes := []Change{{Target: "owner/repo", Path: "Justfile", Type: ChangeUpdate, Desired: "build:\n"}}
	p := NewProcessor(runner, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	return runner, p.Apply(context.Background(), changes, opts, noopProgressReporter{})
}

func TestApply_PRBranchAfterVisibilityChange_RetriesForbidden(t *testing.T) {
	shortVisibilitySettle(t, time.Minute)
	runner, results := applyWithRefusals(2, true)
	if err := results[0].Err; err != nil {
		t.Fatalf("expected the branch to be created once the visibility change settles, got %v", err)
	}
	if n := countCalls(runner.Called, "api repos/owner/repo/git/refs --method POST"); n != 3 {
		t.Errorf("expected 3 ref creations (2 refused), got %d", n)
	}
	if n := countCalls(runner.Called, "pr create"); n != 1 {
		t.Errorf("expected the PR to be opened, got %d pr create calls", n)
	}
}

func TestApply_PRBranchAfterVisibilityChange_GivesUp(t *testing.T) {
	shortVisibilitySettle(t, 20*time.Millisecond)
	_, results := applyWithRefusals(1<<30, true)
	err := results[0].Err
	if !errors.Is(err, gh.ErrForbidden) || !strings.Contains(err.Error(), "after the visibility change") {
		t.Fatalf("expected a 403 that says it was retried, got %v", err)
	}
}

func TestApply_PRBranchWithoutVisibilityChange_DoesNotRetry(t *testing.T) {
	shortVisibilitySettle(t, time.Minute)
	runner, results := applyWithRefusals(1, false)
	if !errors.Is(results[0].Err, gh.ErrForbidden) {
		t.Fatalf("expected the 403 to fail the target, got %v", results[0].Err)
	}
	if n := countCalls(runner.Called, "api repos/owner/repo/git/refs --method POST"); n != 1 {
		t.Errorf("expected 1 ref creation, got %d", n)
	}
}
