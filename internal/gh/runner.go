package gh

import (
	"bytes"
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
)

// Runner abstracts gh command execution for testability.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// GHRunner executes gh commands as subprocesses with automatic retry.
type GHRunner struct {
	DryRun     bool
	MaxRetries uint
}

func NewRunner(dryRun bool) *GHRunner {
	return &GHRunner{
		DryRun:     dryRun,
		MaxRetries: defaultMaxRetries,
	}
}

func (r *GHRunner) Run(args ...string) ([]byte, error) {
	cmdStr := "gh " + strings.Join(args, " ")

	if r.DryRun {
		logger.Info("dry-run", "cmd", cmdStr)
		return nil, nil
	}

	out, err := retry.DoWithData(
		func() ([]byte, error) {
			return r.exec(args, cmdStr)
		},
		retry.Attempts(r.MaxRetries),
		retry.Delay(defaultRetryDelay),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(isRetryable),
		retry.OnRetry(func(n uint, err error) {
			logger.Warn("retrying", "attempt", n+1, "cmd", cmdStr, "err", err)
		}),
	)

	return out, err
}

func (r *GHRunner) exec(args []string, cmdStr string) ([]byte, error) {
	logger.Debug("exec", "cmd", cmdStr)

	cmd := exec.Command("gh", args...)

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

	stderr := strings.TrimSpace(errBuf.String())
	exitCode := cmd.ProcessState.ExitCode()

	logger.Warn("command failed", "cmd", cmdStr, "exit", exitCode, "stderr", truncate(stderr, 500))

	// Non-retryable: auth issue
	if strings.Contains(stderr, "not logged in") ||
		strings.Contains(stderr, "gh auth login") {
		return nil, retry.Unrecoverable(ErrNotAuthed)
	}

	apiErr := tryParseAPIError(stderr)

	exitErr := &ExitError{
		Cmd:      cmdStr,
		ExitCode: exitCode,
		Stderr:   stderr,
		APIError: apiErr,
	}

	if apiErr != nil {
		logger.Debug("api error", "status", apiErr.Status, "message", apiErr.Message)
		switch apiErr.Status {
		case 404:
			return nil, retry.Unrecoverable(fmt.Errorf("%w: %w", ErrNotFound, exitErr))
		case 401:
			return nil, retry.Unrecoverable(fmt.Errorf("%w: %w", ErrUnauthorized, exitErr))
		case 403:
			// Rate limit is retryable; other 403s are not
			if strings.Contains(apiErr.Message, "rate limit") ||
				strings.Contains(apiErr.Message, "abuse detection") {
				return nil, exitErr // retryable
			}
			return nil, retry.Unrecoverable(fmt.Errorf("%w: %w", ErrForbidden, exitErr))
		case 422:
			return nil, retry.Unrecoverable(exitErr)
		}
	}

	// Other errors (network timeout, TLS handshake, etc.) are retryable
	return nil, exitErr
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
