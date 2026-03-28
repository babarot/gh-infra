package parallel

import (
	"context"
	"sync"
)

// Map processes items concurrently and returns results in input order.
// concurrency controls the maximum number of goroutines running at once.
// If concurrency <= 0, all items are processed without a limit.
// When ctx is canceled, remaining unstarted items are skipped.
func Map[T, R any](ctx context.Context, items []T, concurrency int, fn func(context.Context, int, T) R) []R {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results
	}
	if concurrency <= 0 {
		concurrency = len(items)
	}

	ch := make(chan int, len(items))

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Go(func() {
			for i := range ch {
				if ctx.Err() != nil {
					continue // drain channel without processing
				}
				results[i] = fn(ctx, i, items[i])
			}
		})
	}

	for i := range items {
		ch <- i
	}
	close(ch)

	wg.Wait()
	return results
}
