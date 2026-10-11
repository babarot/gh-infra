package gh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// failingGH puts a gh script first on PATH that fails the first n calls with
// stderr msg, then prints {}.
func failingGH(t *testing.T, n int, msg string) {
	t.Helper()
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	script := `#!/bin/sh
c=$(cat ` + count + ` 2>/dev/null || echo 0)
echo $((c+1)) > ` + count + `
if [ "$c" -lt ` + strconv.Itoa(n) + ` ]; then echo "` + msg + `" >&2; exit 1; fi
echo '{}'
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeTime makes the runner's clock advance only when it sleeps, and records
// the sleeps.
type fakeTime struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

func (f *fakeTime) install(r *GHRunner) {
	f.now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	r.clock = func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
	r.sleep = func(ctx context.Context, d time.Duration) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sleeps = append(f.sleeps, d)
		f.now = f.now.Add(d)
		return ctx.Err()
	}
}

func TestRunner_SecondaryRateLimitWaitsAndDoubles(t *testing.T) {
	failingGH(t, 2, "gh: You have exceeded a secondary rate limit. Please wait a few minutes before you try again. (HTTP 403)")
	r := NewRunnerWithConcurrency(false, 1)
	ft := &fakeTime{}
	ft.install(r)
	r.fetchReset = func(context.Context) (time.Time, bool) {
		t.Error("secondary limits need no rate_limit lookup")
		return time.Time{}, false
	}

	if _, err := r.Run(context.Background(), "api", "x"); err != nil {
		t.Fatal(err)
	}
	if len(ft.sleeps) != 2 || ft.sleeps[0] != time.Minute || ft.sleeps[1] != 2*time.Minute {
		t.Errorf("sleeps = %v, want [1m 2m]", ft.sleeps)
	}
}

func TestRunner_PrimaryRateLimitWaitsForReset(t *testing.T) {
	for _, msg := range []string{
		"gh: API rate limit exceeded for user ID 1. (HTTP 403)",
		// gh label / gh secret style, whose URL suffix hides the status
		"HTTP 403: API rate limit exceeded for user ID 1. (https://api.github.com/repos/o/r/labels)",
		// GraphQL (gh repo view) reports it without a status
		"GraphQL: API rate limit exceeded for user ID 1.",
	} {
		t.Run(msg[:12], func(t *testing.T) {
			failingGH(t, 1, msg)
			r := NewRunnerWithConcurrency(false, 1)
			ft := &fakeTime{}
			ft.install(r)
			r.fetchReset = func(context.Context) (time.Time, bool) { return ft.now.Add(5 * time.Minute), true }

			if _, err := r.Run(context.Background(), "api", "x"); err != nil {
				t.Fatal(err)
			}
			if len(ft.sleeps) != 1 || ft.sleeps[0] != 5*time.Minute+time.Second {
				t.Errorf("sleeps = %v, want [5m1s]", ft.sleeps)
			}
		})
	}
}

func TestRunner_PrimaryRateLimitTooFarFailsAtOnce(t *testing.T) {
	failingGH(t, 5, "gh: API rate limit exceeded for user ID 1. (HTTP 403)")
	r := NewRunnerWithConcurrency(false, 1)
	ft := &fakeTime{}
	ft.install(r)
	r.fetchReset = func(context.Context) (time.Time, bool) { return ft.now.Add(40 * time.Minute), true }

	_, err := r.Run(context.Background(), "api", "x")
	if !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "resets at") {
		t.Fatalf("err = %v, want ErrRateLimited with the reset time", err)
	}
	if len(ft.sleeps) != 0 {
		t.Errorf("should not wait past the cap, slept %v", ft.sleeps)
	}
}

func TestRunner_GateHoldsOtherCalls(t *testing.T) {
	failingGH(t, 0, "")
	r := NewRunnerWithConcurrency(false, 4)
	ft := &fakeTime{}
	ft.install(r)
	r.gate.extend(ft.now.Add(2 * time.Minute)) // another call hit a limit

	var notified []time.Time
	r.SetOnWait(func(until time.Time) { notified = append(notified, until) })
	if _, err := r.Run(context.Background(), "api", "x"); err != nil {
		t.Fatal(err)
	}
	if len(ft.sleeps) != 1 || ft.sleeps[0] != 2*time.Minute {
		t.Errorf("sleeps = %v, want [2m]", ft.sleeps)
	}
	if len(notified) != 2 || notified[0].IsZero() || !notified[1].IsZero() {
		t.Errorf("OnWait calls = %v, want [until, zero]", notified)
	}

	// A gate beyond the cap fails without waiting.
	r.gate.extend(ft.now.Add(30 * time.Minute))
	if _, err := r.Run(context.Background(), "api", "x"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}

func TestRunner_NetworkErrorsBackOff(t *testing.T) {
	failingGH(t, 2, "dial tcp 140.82.112.6:443: i/o timeout")
	r := NewRunnerWithConcurrency(false, 1)
	ft := &fakeTime{}
	ft.install(r)
	if _, err := r.Run(context.Background(), "api", "x"); err != nil {
		t.Fatal(err)
	}
	if len(ft.sleeps) != 2 || ft.sleeps[0] != time.Second || ft.sleeps[1] != 2*time.Second {
		t.Errorf("sleeps = %v, want [1s 2s]", ft.sleeps)
	}

	failingGH(t, 10, "dial tcp 140.82.112.6:443: i/o timeout")
	r = NewRunnerWithConcurrency(false, 1)
	ft = &fakeTime{}
	ft.install(r)
	if _, err := r.Run(context.Background(), "api", "x"); err == nil {
		t.Fatal("expected the error after the last attempt")
	}
	if len(ft.sleeps) != defaultMaxRetries-1 {
		t.Errorf("sleeps = %v, want %d", ft.sleeps, defaultMaxRetries-1)
	}
}

func TestRunner_RateLimitWaitStopsOnCancel(t *testing.T) {
	failingGH(t, 0, "")
	r := NewRunnerWithConcurrency(false, 1)
	r.gate.extend(time.Now().Add(10 * time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := r.Run(ctx, "api", "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Error("an interrupted wait should return at once")
	}
}

func TestBuildCommandError_PaginatedRateLimitIsRetried(t *testing.T) {
	// --paginate: the first page is on stdout, the 403 only on stderr.
	err := buildCommandError("gh api --paginate x", 1, `[{"id":1}]`, "gh: API rate limit exceeded for user ID 1. (HTTP 403)")
	if errors.Is(err, ErrForbidden) {
		t.Fatalf("a rate limit must not be ErrForbidden: %v", err)
	}
	if classifyRateLimit(err) != primaryRateLimit {
		t.Errorf("classify = %v, want primary", classifyRateLimit(err))
	}
}
