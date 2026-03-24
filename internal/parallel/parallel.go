package parallel

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// Map processes items concurrently and returns results in input order.
// concurrency controls the maximum number of goroutines running at once.
// If concurrency <= 0, all items are processed without a limit.
func Map[T, R any](items []T, concurrency int, fn func(int, T) R) []R {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results
	}

	var sem *semaphore.Weighted
	if concurrency > 0 {
		sem = semaphore.NewWeighted(int64(concurrency))
	}

	var wg sync.WaitGroup
	for i, item := range items {
		wg.Add(1)
		go func(idx int, item T) {
			defer wg.Done()
			if sem != nil {
				_ = sem.Acquire(context.Background(), 1)
				defer sem.Release(1)
			}
			results[idx] = fn(idx, item)
		}(i, item)
	}

	wg.Wait()
	return results
}
