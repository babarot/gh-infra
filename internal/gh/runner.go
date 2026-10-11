package gh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	retry "github.com/avast/retry-go/v4"

	"github.com/babarot/gh-infra/internal/logger"
)

const (
	defaultMaxRetries = 3
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

	out, err := retry.DoWithData(
		func() ([]byte, error) {
			if err := r.acquire(ctx); err != nil {
				return nil, retry.Unrecoverable(err)
			}
			defer r.release()
			return r.exec(ctx, body, args, cmdStr)
		},
		retry.Attempts(r.MaxRetries),
		retry.Delay(defaultRetryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(isRetryable),
		retry.Context(ctx),
		retry.OnRetry(func(n uint, err error) {
			logger.Warn("retrying", "attempt", n+1, "cmd", cmdStr, "err", err)
		}),
	)

	// Unwrap retry-go's "All attempts fail: #1: ..." wrapper
	// to surface the underlying error directly.
	if err != nil {
		if unwrapped := errors.Unwrap(err); unwrapped != nil {
			err = unwrapped
		}
	}

	return out, err
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
		// Rate limit is retryable; other 403s are not.
		if strings.Contains(apiErr.Message, "rate limit") ||
			strings.Contains(apiErr.Message, "abuse detection") {
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
			strings.Contains(stderr, "rate limit") ||
			strings.Contains(stderr, "abuse detection") ||
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
