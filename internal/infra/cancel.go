package infra

import (
	"context"

	"github.com/babarot/gh-infra/internal/ui"
)

// loader is implemented by gh.GHRunner.
type loader interface {
	Load() (running, waiting int)
	Concurrency() int
}

// withLoad returns heading with a gauge of the runner's gh processes, when
// the runner can report them and has a limit.
func withLoad(heading ui.RefreshHeading, runner any) ui.RefreshHeading {
	if l, ok := runner.(loader); ok && l.Concurrency() > 0 {
		heading.Load = func() (running, waiting, limit int) {
			running, waiting = l.Load()
			return running, waiting, l.Concurrency()
		}
	}
	return heading
}

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
//
// The heading shows how busy the runner's gh processes are when the runner
// can say (gh.GHRunner).
func startRefresh(parent context.Context, runner any, heading ui.RefreshHeading, tasks []ui.RefreshTask) (tracker *ui.RefreshTracker, ctx context.Context, stop func()) {
	tracker = ui.RunRefresh(withLoad(heading, runner), tasks)
	stopWaits := showRateLimitWaits(runner, tracker.SetNotice)
	stopInterrupt := context.AfterFunc(parent, tracker.Interrupt)
	ctx, cancel := withTrackerCancel(parent, tracker)
	return tracker, ctx, func() {
		cancel()
		stopInterrupt()
		stopWaits()
	}
}
