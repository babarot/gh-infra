package gh

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/babarot/gh-infra/internal/logger"
)

// Runner abstracts gh command execution for testability.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// GHRunner executes gh commands as subprocesses.
type GHRunner struct {
	DryRun bool
}

func NewRunner(dryRun bool) *GHRunner {
	return &GHRunner{
		DryRun: dryRun,
	}
}

func (r *GHRunner) Run(args ...string) ([]byte, error) {
	cmdStr := "gh " + strings.Join(args, " ")

	if r.DryRun {
		logger.Info("dry-run", "cmd", cmdStr)
		return nil, nil
	}

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

	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, ErrNotInstalled
	}

	stderr := strings.TrimSpace(errBuf.String())
	exitCode := cmd.ProcessState.ExitCode()

	logger.Warn("command failed", "cmd", cmdStr, "exit", exitCode, "stderr", truncate(stderr, 500))

	if strings.Contains(stderr, "not logged in") ||
		strings.Contains(stderr, "gh auth login") {
		return nil, ErrNotAuthed
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
			return nil, fmt.Errorf("%w: %w", ErrNotFound, exitErr)
		case 401:
			return nil, fmt.Errorf("%w: %w", ErrUnauthorized, exitErr)
		case 403:
			return nil, fmt.Errorf("%w: %w", ErrForbidden, exitErr)
		}
	}

	return nil, exitErr
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
