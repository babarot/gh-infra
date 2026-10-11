package gh

import (
	"context"
	"testing"
	"time"
)

func TestRunner_UsageCountsCallsAndWaits(t *testing.T) {
	failingGH(t, 1, "dial tcp 140.82.112.6:443: i/o timeout") // one retry
	r := NewRunnerWithConcurrency(false, 1)
	ft := &fakeTime{}
	ft.install(r)
	r.gate.extend(ft.now.Add(time.Minute))

	if _, err := r.Run(context.Background(), "api", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), "api", "y"); err != nil {
		t.Fatal(err)
	}
	u := r.Usage()
	if u.Calls != 3 {
		t.Errorf("calls = %d, want 3 (a retry counts)", u.Calls)
	}
	if u.Waited != time.Minute {
		t.Errorf("waited = %s, want 1m", u.Waited)
	}
}

func TestUsageCounter_OverlappingWaitsCountOnce(t *testing.T) {
	var u usageCounter
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	u.wait(base, base.Add(time.Minute))                     // call A
	u.wait(base.Add(10*time.Second), base.Add(time.Minute)) // call B, same gate
	u.wait(base.Add(2*time.Minute), base.Add(3*time.Minute))
	if u.waited != 2*time.Minute {
		t.Errorf("waited = %s, want 2m", u.waited)
	}
}
