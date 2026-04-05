package infra

import (
	"context"

	"github.com/babarot/gh-infra/internal/ui"
)

// withTrackerCancelContext mirrors the plan flow: if the refresh tracker is
// interrupted with Ctrl+C, cancel the returned context so in-flight gh
// subprocesses are terminated via exec.CommandContext.
func withTrackerCancelContext(tracker *ui.RefreshTracker) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := tracker.Canceled()
		if ch == nil {
			return
		}
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
