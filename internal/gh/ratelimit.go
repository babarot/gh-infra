package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// maxRateLimitWait caps how long one call waits for rate limits in
	// total. A primary limit that resets later fails at once instead.
	maxRateLimitWait = 15 * time.Minute

	// secondaryRateLimitWait is the first wait after a secondary rate
	// limit; GitHub asks to wait at least a minute. It doubles while the
	// limit keeps being hit.
	secondaryRateLimitWait = time.Minute
)

// ErrRateLimited is returned when a rate limit would need a longer wait than
// maxRateLimitWait allows.
var ErrRateLimited = errors.New("GitHub API rate limit exceeded")

type rateLimitKind int

const (
	notRateLimited rateLimitKind = iota
	primaryRateLimit
	secondaryRateLimit
)

// rateLimitKindOf classifies a gh error message. The text is the only signal
// that works for every command: GraphQL reports rate limits with HTTP 200,
// `gh label` and friends print "HTTP 403: API rate limit exceeded ... (url)",
// and a --paginate failure leaves only "HTTP 403" in the parsed message.
func rateLimitKindOf(text string) rateLimitKind {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "secondary rate limit"),
		strings.Contains(text, "abuse detection"):
		return secondaryRateLimit
	case strings.Contains(text, "rate limit exceeded"),
		strings.Contains(text, "api rate limit"),
		strings.Contains(text, "rate_limited"):
		return primaryRateLimit
	}
	return notRateLimited
}

func classifyRateLimit(err error) rateLimitKind {
	exitErr, ok := errors.AsType[*ExitError](err)
	if !ok {
		return notRateLimited
	}
	text := exitErr.Stderr + "\n" + exitErr.Stdout
	if exitErr.APIError != nil {
		text += "\n" + exitErr.APIError.Message
	}
	return rateLimitKindOf(text)
}

// rateGate makes every call of a runner wait while a rate limit is in
// effect, so concurrent calls do not keep hitting it.
type rateGate struct {
	mu     sync.Mutex
	until  time.Time
	streak int // consecutive secondary rate limits, for the doubling wait
}

func (g *rateGate) deadline() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.until
}

// extend moves the gate to t if that is later.
func (g *rateGate) extend(t time.Time) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	if t.After(g.until) {
		g.until = t
	}
	return g.until
}

// secondary records a secondary rate limit and closes the gate for 1, 2,
// 4, ... minutes.
func (g *rateGate) secondary(now time.Time) time.Time {
	g.mu.Lock()
	g.streak++
	wait := secondaryRateLimitWait << (g.streak - 1)
	g.mu.Unlock()
	return g.extend(now.Add(wait))
}

// succeeded resets the doubling once a call gets through after the gate.
func (g *rateGate) succeeded(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.streak > 0 && !now.Before(g.until) {
		g.streak = 0
	}
}

// SetOnWait sets a function called with the time the runner waits until for
// a rate limit, and with the zero time when the wait is over. The caller
// uses it to show the wait, since the runner must not write to the terminal.
func (r *GHRunner) SetOnWait(fn func(until time.Time)) {
	r.waitMu.Lock()
	defer r.waitMu.Unlock()
	r.onWait = fn
}

func (r *GHRunner) notifyWait(until time.Time) {
	r.waitMu.Lock()
	fn := r.onWait
	r.waitMu.Unlock()
	if fn != nil {
		fn(until)
	}
}

// waitGate waits while the gate is closed, adding to *waited. It fails at
// once when the wait would take this call past maxRateLimitWait.
func (r *GHRunner) waitGate(ctx context.Context, waited *time.Duration) error {
	until := r.gate.deadline()
	now := r.clock()
	if !until.After(now) {
		return nil
	}
	d := until.Sub(now)
	if *waited+d > maxRateLimitWait {
		return rateLimitedError(until, now)
	}
	r.notifyWait(until)
	err := r.sleep(ctx, d)
	r.notifyWait(time.Time{})
	*waited += d
	return err
}

func rateLimitedError(until, now time.Time) error {
	return fmt.Errorf("%w; resets at %s (in %s)", ErrRateLimited,
		until.Local().Format("15:04"), until.Sub(now).Round(time.Minute))
}

// primaryReset returns when the exhausted rate limit resets. It reads
// `gh api rate_limit` outside the slots and the gate (so a call holding no
// slot cannot deadlock on it), and only when the gate is open: calls that
// hit the limit together share one lookup. Without an exhausted resource,
// it waits like a secondary rate limit.
func (r *GHRunner) primaryReset(ctx context.Context) time.Time {
	now := r.clock()
	if until := r.gate.deadline(); until.After(now) {
		return until
	}
	if reset, ok := r.fetchReset(ctx); ok && reset.After(now) {
		return r.gate.extend(reset.Add(time.Second))
	}
	return r.gate.secondary(now)
}

// fetchRateLimitReset reads the latest reset time among the resources with
// no requests left. `gh api rate_limit` does not count against the limit.
func fetchRateLimitReset(ctx context.Context) (time.Time, bool) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "api", "rate_limit")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return time.Time{}, false
	}
	var resp struct {
		Resources map[string]struct {
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return time.Time{}, false
	}
	var latest time.Time
	for _, res := range resp.Resources {
		if res.Remaining == 0 && res.Reset > 0 {
			if t := time.Unix(res.Reset, 0); t.After(latest) {
				latest = t
			}
		}
	}
	return latest, !latest.IsZero()
}

// sleepContext waits for d or until ctx ends.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
