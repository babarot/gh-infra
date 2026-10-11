package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"
)

// Usage is what a runner has used so far.
type Usage struct {
	Calls  int           // gh processes run, retries included
	Waited time.Duration // time spent waiting for rate limits (overlapping waits count once)
}

// usageCounter counts gh processes and rate limit waits.
type usageCounter struct {
	mu        sync.Mutex
	calls     int
	waited    time.Duration
	waitedTil time.Time // end of the latest counted wait, to count overlaps once
}

func (u *usageCounter) call() {
	u.mu.Lock()
	u.calls++
	u.mu.Unlock()
}

// wait counts a wait from start to until, minus what an earlier wait
// already covered.
func (u *usageCounter) wait(start, until time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if start.Before(u.waitedTil) {
		start = u.waitedTil
	}
	if until.After(start) {
		u.waited += until.Sub(start)
		u.waitedTil = until
	}
}

// Usage returns how many gh processes the runner ran and how long it waited
// for rate limits.
func (r *GHRunner) Usage() Usage {
	r.usage.mu.Lock()
	defer r.usage.mu.Unlock()
	return Usage{Calls: r.usage.calls, Waited: r.usage.waited}
}

// RateLimit is one resource of `gh api rate_limit`.
type RateLimit struct {
	Remaining int
	Limit     int
	Reset     time.Time
}

// RateLimits reads the REST ("core") and GraphQL limits. It runs outside the
// slots and the usage count, and `gh api rate_limit` does not count against
// the limit. It returns false when the endpoint is not available (e.g. GHES
// with rate limiting disabled).
func (r *GHRunner) RateLimits(ctx context.Context) (core, graphql RateLimit, ok bool) {
	if r.DryRun {
		return RateLimit{}, RateLimit{}, false
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "api", "rate_limit")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return RateLimit{}, RateLimit{}, false
	}
	var resp struct {
		Resources map[string]struct {
			Remaining int   `json:"remaining"`
			Limit     int   `json:"limit"`
			Reset     int64 `json:"reset"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return RateLimit{}, RateLimit{}, false
	}
	c, okCore := resp.Resources["core"]
	g, okGraphQL := resp.Resources["graphql"]
	if !okCore || !okGraphQL {
		return RateLimit{}, RateLimit{}, false
	}
	return RateLimit{c.Remaining, c.Limit, time.Unix(c.Reset, 0)},
		RateLimit{g.Remaining, g.Limit, time.Unix(g.Reset, 0)}, true
}
