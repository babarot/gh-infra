package parallel

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func TestMap_BasicOrder(t *testing.T) {
	items := []int{10, 20, 30, 40, 50}
	results := Map(items, 2, func(i int, v int) string {
		return fmt.Sprintf("%d:%d", i, v)
	})

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for i, item := range items {
		want := fmt.Sprintf("%d:%d", i, item)
		if results[i] != want {
			t.Errorf("results[%d] = %q, want %q", i, results[i], want)
		}
	}
}

func TestMap_Empty(t *testing.T) {
	results := Map([]int{}, 5, func(i int, v int) int {
		t.Error("fn should not be called for empty input")
		return 0
	})
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMap_SingleItem(t *testing.T) {
	results := Map([]string{"hello"}, 1, func(i int, v string) int {
		return len(v)
	})
	if len(results) != 1 || results[0] != 5 {
		t.Errorf("expected [5], got %v", results)
	}
}

func TestMap_ZeroConcurrency(t *testing.T) {
	// concurrency <= 0 means unlimited (all items at once)
	items := []int{1, 2, 3, 4, 5}
	results := Map(items, 0, func(i int, v int) int {
		return v * 2
	})
	for i, want := range []int{2, 4, 6, 8, 10} {
		if results[i] != want {
			t.Errorf("results[%d] = %d, want %d", i, results[i], want)
		}
	}
}

func TestMap_ConcurrencyLimit(t *testing.T) {
	// Verify that at most N goroutines run concurrently
	var running atomic.Int32
	var maxRunning atomic.Int32

	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}

	Map(items, 3, func(i int, v int) int {
		cur := running.Add(1)
		for {
			old := maxRunning.Load()
			if cur <= old || maxRunning.CompareAndSwap(old, cur) {
				break
			}
		}
		running.Add(-1)
		return v
	})

	if max := maxRunning.Load(); max > 3 {
		t.Errorf("max concurrent = %d, want <= 3", max)
	}
}

func TestMap_PreservesOrderWithHighConcurrency(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	results := Map(items, 50, func(i int, v int) int {
		return v * v
	})

	for i, v := range items {
		want := v * v
		if results[i] != want {
			t.Errorf("results[%d] = %d, want %d", i, results[i], want)
		}
	}
}
