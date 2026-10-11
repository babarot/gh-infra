package gh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
