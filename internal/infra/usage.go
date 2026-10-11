package infra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/ui"
)

// APIUsage is the GitHub API usage of a command run, in the JSON document.
type APIUsage struct {
	Calls         int            `json:"calls"`
	WaitedSeconds int            `json:"waited_seconds"`
	Remaining     *APIRateLimits `json:"remaining,omitempty"`
}

// APIRateLimits is what is left of the REST and GraphQL rate limits.
type APIRateLimits struct {
	Core    APIRateLimit `json:"core"`
	GraphQL APIRateLimit `json:"graphql"`
}

// APIRateLimit is one rate limit; Reset is RFC 3339.
type APIRateLimit struct {
	Remaining int    `json:"remaining"`
	Limit     int    `json:"limit"`
	Reset     string `json:"reset"`
}

// usageSource is implemented by gh.GHRunner.
type usageSource interface {
	Usage() gh.Usage
	RateLimits(ctx context.Context) (core, graphql gh.RateLimit, ok bool)
}

// apiUsage reads the usage of runner, or returns nil for a runner that does
// not count (tests).
func apiUsage(runner gh.Runner) *APIUsage {
	src, ok := runner.(usageSource)
	if !ok {
		return nil
	}
	u := src.Usage()
	usage := &APIUsage{Calls: u.Calls, WaitedSeconds: int(u.Waited.Round(time.Second) / time.Second)}
	if core, graphql, ok := src.RateLimits(context.Background()); ok {
		usage.Remaining = &APIRateLimits{
			Core:    APIRateLimit{core.Remaining, core.Limit, core.Reset.UTC().Format(time.RFC3339)},
			GraphQL: APIRateLimit{graphql.Remaining, graphql.Limit, graphql.Reset.UTC().Format(time.RFC3339)},
		}
	}
	return usage
}

// printAPIUsage prints one dim line with the usage on stderr, after the
// spinner. Nothing is printed when no API call was made.
func printAPIUsage(p ui.Printer, usage *APIUsage) {
	if usage == nil || usage.Calls == 0 {
		return
	}
	p.Phase(ui.Dim.Render(formatAPIUsage(usage)))
}

func formatAPIUsage(u *APIUsage) string {
	parts := []string{fmt.Sprintf("GitHub API: %s calls", thousands(u.Calls))}
	if r := u.Remaining; r != nil {
		parts = append(parts, fmt.Sprintf("remaining core %s/%s, graphql %s/%s",
			thousands(r.Core.Remaining), thousands(r.Core.Limit),
			thousands(r.GraphQL.Remaining), thousands(r.GraphQL.Limit)))
	}
	if u.WaitedSeconds > 0 {
		parts = append(parts, fmt.Sprintf("waited %s for rate limits", time.Duration(u.WaitedSeconds)*time.Second))
	}
	return strings.Join(parts, "; ")
}

func thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
