package infra

import (
	"fmt"
	"time"

	"github.com/babarot/gh-infra/internal/ui"
)

// waitNotifier is implemented by gh.GHRunner.
type waitNotifier interface {
	SetOnWait(func(until time.Time))
}

// showRateLimitWaits shows the runner's rate limit waits under the spinner
// of tracker until the returned function is called.
func showRateLimitWaits(runner any, tracker *ui.RefreshTracker) func() {
	n, ok := runner.(waitNotifier)
	if !ok {
		return func() {}
	}
	n.SetOnWait(func(until time.Time) {
		if until.IsZero() {
			tracker.SetNotice("")
			return
		}
		tracker.SetNotice(fmt.Sprintf("Waiting for the GitHub API rate limit until %s ...", until.Local().Format("15:04:05")))
	})
	return func() { n.SetOnWait(nil) }
}
