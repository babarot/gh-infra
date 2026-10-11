package gh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	retry "github.com/avast/retry-go/v4"

	"github.com/babarot/gh-infra/internal/logger"
)

const (
	// defaultMaxRetries is how many times a call is tried for transient
	// network errors; the delay doubles from defaultRetryDelay (1s, 2s, 4s,
	// 8s). Rate limits are waited for separately.
	defaultMaxRetries = 5
	defaultRetryDelay = 1 * time.Second

	// DefaultConcurrency is how many gh processes run at once by default.
	DefaultConcurrency = 8
)

// Runner abstracts gh command execution for testability.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
	// RunWithStdin executes a gh command with body as stdin (for --input - usage).
	RunWithStdin(ctx context.Context, body []byte, args ...string) ([]byte, error)
}

// GHRunner executes gh commands as subprocesses with automatic retry.
// Use one runner for a whole command run: it limits how many gh processes
// run at once across everything that shares it.
type GHRunner struct {
	DryRun     bool
	MaxRetries uint

	slots chan struct{} // one token per running gh process
	gate  rateGate      // shared wait while a rate limit is in effect

	waitMu sync.Mutex
	onWait func(until time.Time)

	// Replaceable in tests.
	clock      func() time.Time
	sleep      func(ctx context.Context, d time.Duration) error
	fetchReset func(ctx context.Context) (time.Time, bool)
}

// NewRunner returns a runner that runs at most DefaultConcurrency gh
// processes at once.
func NewRunner(dryRun bool) *GHRunner {
	return NewRunnerWithConcurrency(dryRun, DefaultConcurrency)
}

// NewRunnerWithConcurrency returns a runner that runs at most concurrency gh
// processes at once (DefaultConcurrency when concurrency < 1).
func NewRunnerWithConcurrency(dryRun bool, concurrency int) *GHRunner {
	if concurrency < 1 {
		concurrency = DefaultConcurrency
	}
	return &GHRunner{
		DryRun:     dryRun,
		MaxRetries: defaultMaxRetries,
		slots:      make(chan struct{}, concurrency),
		clock:      time.Now,
		sleep:      sleepContext,
		fetchReset: fetchRateLimitReset,
	}
}

// Concurrency returns how many gh processes the runner runs at once.
func (r *GHRunner) Concurrency() int { return cap(r.slots) }

// acquire waits for a free slot, or for ctx to end. A slot is held only
// while a gh process runs, never across retry delays.
func (r *GHRunner) acquire(ctx context.Context) error {
	if r.slots == nil {
		return nil
	}
	select {
	case r.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *GHRunner) release() {
	if r.slots != nil {
		<-r.slots
	}
}

func (r *GHRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return r.runInternal(ctx, nil, args)
}

func (r *GHRunner) RunWithStdin(ctx context.Context, body []byte, args ...string) ([]byte, error) {
	return r.runInternal(ctx, body, args)
}

func (r *GHRunner) runInternal(ctx context.Context, body []byte, args []string) ([]byte, error) {
	cmdStr := "gh " + strings.Join(args, " ")

	if r.DryRun {
		logger.Info("dry-run", "cmd", cmdStr)
		return nil, nil
	}

	var waited time.Duration // rate limit waits of this call
	for attempt := 1; ; attempt++ {
		// Wait while another call (or this one) hit a rate limit. No slot is
		// held while waiting.
		if err := r.waitGate(ctx, &waited); err != nil {
			return nil, err
		}
		if err := r.acquire(ctx); err != nil {
			return nil, err
		}
		out, err := r.exec(ctx, body, args, cmdStr)
		r.release()
		if err == nil {
			r.gate.succeeded(r.clock())
			return out, nil
		}
		if ctx.Err() != nil {
			return nil, unwrapUnrecoverable(err)
		}

		switch classifyRateLimit(err) {
		case secondaryRateLimit:
			until := r.gate.secondary(r.clock())
			logger.Warn("secondary rate limit, waiting", "cmd", cmdStr, "until", until.Format(time.TimeOnly))
			continue
		case primaryRateLimit:
			until := r.primaryReset(ctx)
			logger.Warn("rate limit, waiting", "cmd", cmdStr, "until", until.Format(time.TimeOnly))
			continue
		}

		if !retry.IsRecoverable(err) || !isRetryable(err) || attempt >= int(r.MaxRetries) {
			return nil, unwrapUnrecoverable(err)
		}
		delay := defaultRetryDelay << (attempt - 1)
		logger.Warn("retrying", "attempt", attempt+1, "cmd", cmdStr, "err", err, "delay", delay)
		if err := r.sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

// unwrapUnrecoverable removes the retry.Unrecoverable marker that
// buildCommandError puts on errors that must not be retried.
func unwrapUnrecoverable(err error) error {
	if !retry.IsRecoverable(err) {
		if inner := errors.Unwrap(err); inner != nil {
			return inner
		}
	}
	return err
}

func (r *GHRunner) exec(ctx context.Context, body []byte, args []string, cmdStr string) ([]byte, error) {
	logger.Debug("exec", "cmd", cmdStr)

	cmd := exec.CommandContext(ctx, "gh", args...)
	if body != nil {
		cmd.Stdin = bytes.NewReader(body)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	// Trace: log full request/response
	if logger.IsTrace() {
		stdout := strings.TrimSpace(outBuf.String())
		stderr := strings.TrimSpace(errBuf.String())
		if stdout != "" {
			logger.Trace("stdout", "cmd", cmdStr, "output", truncate(stdout, 2000))
		}
		if stderr != "" {
			logger.Trace("stderr", "cmd", cmdStr, "output", truncate(stderr, 1000))
		}
	}

	if runErr == nil {
		logger.Debug("ok", "cmd", cmdStr, "bytes", outBuf.Len())
		return outBuf.Bytes(), nil
	}

	// Non-retryable: gh not installed
	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, retry.Unrecoverable(ErrNotInstalled)
	}

	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	exitCode := cmd.ProcessState.ExitCode()

	logger.Warn("command failed", "cmd", cmdStr, "exit", exitCode, "stderr", truncate(stderr, 500))

	return nil, buildCommandError(cmdStr, exitCode, stdout, stderr)
}

func buildCommandError(cmdStr string, exitCode int, stdout, stderr string) error {
	// Auth failures are emitted as plain stderr text, so detect them before
	// attempting API error normalization.
	if strings.Contains(stderr, "not logged in") ||
		strings.Contains(stderr, "gh auth login") {
		return retry.Unrecoverable(ErrNotAuthed)
	}

	apiErr := parseAPIErrorFromStreams(stdout, stderr)

	exitErr := &ExitError{
		Cmd:      cmdStr,
		ExitCode: exitCode,
		Stderr:   stderr,
		Stdout:   truncate(stdout, 2000),
		APIError: apiErr,
	}

	if apiErr == nil {
		// Other errors (network timeout, TLS handshake, etc.) remain retryable.
		return exitErr
	}

	logger.Debug("api error", "status", apiErr.Status, "message", apiErr.Message)
	switch apiErr.Status {
	case 404:
		return retry.Unrecoverable(fmt.Errorf("%w: %w", ErrNotFound, exitErr))
	case 401:
		return retry.Unrecoverable(fmt.Errorf("%w: %w", ErrUnauthorized, exitErr))
	case 403:
		// Rate limits are waited for and retried; other 403s are not. Judge
		// by the whole output: a --paginate failure leaves only "HTTP 403"
		// in the parsed message.
		if rateLimitKindOf(stderr+"\n"+stdout+"\n"+apiErr.Message) != notRateLimited {
			return exitErr
		}
		return retry.Unrecoverable(fmt.Errorf("%w: %w", ErrForbidden, exitErr))
	case 409:
		return retry.Unrecoverable(fmt.Errorf("%w: %w", ErrConflict, exitErr))
	case 422:
		return retry.Unrecoverable(fmt.Errorf("%w: %w", ErrValidation, exitErr))
	default:
		return exitErr
	}
}

func parseAPIErrorFromStreams(stdout, stderr string) *APIError {
	// `gh api` emits the raw JSON response body on stdout and a derived
	// human-readable summary on stderr.  Prefer stdout because it carries
	// structured fields (message, status, errors) that can be parsed
	// without heuristics, then fall back to stderr for plain `gh` commands
	// that only report errors there.
	if apiErr := tryParseAPIError(stdout); apiErr != nil {
		return apiErr
	}
	return tryParseAPIError(stderr)
}

// isRetryable determines whether an error should trigger a retry.
func isRetryable(err error) bool {
	// Unrecoverable errors are never retried (handled by retry-go internally)
	// All other errors (network, timeout, etc.) are retried
	if exitErr, ok := errors.AsType[*ExitError](err); ok {
		stderr := strings.ToLower(exitErr.Stderr)
		return strings.Contains(stderr, "timeout") ||
			strings.Contains(stderr, "connection reset") ||
			strings.Contains(stderr, "connection refused") ||
			strings.Contains(stderr, "tls handshake") ||
			strings.Contains(stderr, "no such host") ||
			strings.Contains(stderr, "eof") ||
			strings.Contains(stderr, "broken pipe")
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
