package infra

import (
	"fmt"
	"time"
)

// waitNotifier is implemented by gh.GHRunner.
type waitNotifier interface {
	SetOnWait(func(until time.Time))
}

// showRateLimitWaits shows the runner's rate limit waits with notice until
// the returned function is called. notice gets "" when a wait ends.
func showRateLimitWaits(runner any, notice func(text string)) func() {
	n, ok := runner.(waitNotifier)
	if !ok {
		return func() {}
	}
	n.SetOnWait(func(until time.Time) {
		if until.IsZero() {
			notice("")
			return
		}
		notice(fmt.Sprintf("Waiting for the GitHub API rate limit until %s ...", until.Local().Format("15:04:05")))
	})
	return func() { n.SetOnWait(nil) }
}
