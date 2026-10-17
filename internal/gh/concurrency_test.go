package gh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGH puts a gh script first on PATH. Each call appends "+" to log,
// sleeps, then appends "-", so the log shows how many calls overlapped.
func fakeGH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho + >> " + log + "\nsleep 0.2\necho - >> " + log + "\necho '{}'\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func maxOverlap(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	cur, peak := 0, 0
	for line := range strings.FieldsSeq(string(data)) {
		if line == "+" {
			cur++
			peak = max(peak, cur)
		} else {
			cur--
		}
	}
	return peak
}

func TestRunner_LimitsConcurrentProcesses(t *testing.T) {
	for _, limit := range []int{1, 3} {
		log := fakeGH(t)
		r := NewRunnerWithConcurrency(false, limit)
		var wg sync.WaitGroup
		for range 9 {
			wg.Go(func() {
				if _, err := r.Run(context.Background(), "api", "x"); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if got := maxOverlap(t, log); got > limit || got == 0 {
			t.Errorf("limit %d: %d gh processes ran at once", limit, got)
		}
	}
}

func TestRunner_AcquireHonorsContext(t *testing.T) {
	r := NewRunnerWithConcurrency(false, 1)
	if err := r.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.acquire(ctx); err == nil {
		t.Error("acquire should give up when the context ends")
	}
	r.release()
}

func TestRunner_Load(t *testing.T) {
	r := NewRunnerWithConcurrency(false, 1)
	if running, waiting := r.Load(); running != 0 || waiting != 0 {
		t.Fatalf("idle: Load = (%d, %d), want (0, 0)", running, waiting)
	}
	if err := r.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Two more calls wait for the one slot.
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _ = r.acquire(ctx) })
	}
	waitFor(t, func() bool { _, w := r.Load(); return w == 2 })
	if running, waiting := r.Load(); running != 1 || waiting != 2 {
		t.Errorf("busy: Load = (%d, %d), want (1, 2)", running, waiting)
	}

	// A call that gives up waiting is no longer counted.
	cancel()
	wg.Wait()
	if running, waiting := r.Load(); running != 1 || waiting != 0 {
		t.Errorf("after the waits ended: Load = (%d, %d), want (1, 0)", running, waiting)
	}
	r.release()
	if running, _ := r.Load(); running != 0 {
		t.Errorf("after release: running = %d, want 0", running)
	}

	var zero GHRunner // no slots: no limit to report
	if running, waiting := zero.Load(); running != 0 || waiting != 0 {
		t.Errorf("zero runner: Load = (%d, %d), want (0, 0)", running, waiting)
	}
}

// waitFor polls cond for up to two seconds.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
