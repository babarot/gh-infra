package infra

import (
	"context"

	"github.com/babarot/gh-infra/internal/ui"
)

// canceler is implemented by ui.RefreshTracker.
type canceler interface {
	Canceled() <-chan struct{}
}

// withTrackerCancel returns a context that is canceled with parent, or when
// the spinner is interrupted with Ctrl+C, so in-flight gh subprocesses are
// terminated via exec.CommandContext.
func withTrackerCancel(parent context.Context, c canceler) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	ch := c.Canceled()
	if ch == nil {
		return ctx, cancel
	}
	go func() {
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// startRefresh starts the spinner for tasks and shows the runner's rate
// limit waits under it. The returned context is canceled with parent or
// when the spinner is interrupted. When parent is canceled (a signal), the
// spinner stops as for Ctrl+C, and errors of the interrupted work are not
// shown. Call stop when the work is done; it does not wait for the spinner.
func startRefresh(parent context.Context, runner any, tasks []ui.RefreshTask) (tracker *ui.RefreshTracker, ctx context.Context, stop func()) {
	tracker = ui.RunRefresh(tasks)
	stopWaits := showRateLimitWaits(runner, tracker.SetNotice)
	stopInterrupt := context.AfterFunc(parent, tracker.Interrupt)
	ctx, cancel := withTrackerCancel(parent, tracker)
	return tracker, ctx, func() {
		cancel()
		stopInterrupt()
		stopWaits()
	}
}
