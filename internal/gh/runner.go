package gh

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Runner abstracts gh command execution for testability.
type Runner interface {
	Run(args ...string) ([]byte, error)
}

// GHRunner executes gh commands as subprocesses.
type GHRunner struct {
	DryRun  bool
	Verbose bool
}

func NewRunner(dryRun, verbose bool) *GHRunner {
	return &GHRunner{
		DryRun:  dryRun,
		Verbose: verbose,
	}
}

func (r *GHRunner) Run(args ...string) ([]byte, error) {
	if r.DryRun {
		log.Printf("[dry-run] gh %s", strings.Join(args, " "))
		return nil, nil
	}

	if r.Verbose {
		log.Printf("+ gh %s", strings.Join(args, " "))
	}

	cmd := exec.Command("gh", args...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if runErr == nil {
		return outBuf.Bytes(), nil
	}

	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, ErrNotInstalled
	}

	stderr := strings.TrimSpace(errBuf.String())
	exitCode := cmd.ProcessState.ExitCode()

	if strings.Contains(stderr, "not logged in") ||
		strings.Contains(stderr, "gh auth login") {
		return nil, ErrNotAuthed
	}

	apiErr := tryParseAPIError(stderr)

	exitErr := &ExitError{
		Cmd:      "gh " + strings.Join(args, " "),
		ExitCode: exitCode,
		Stderr:   stderr,
		APIError: apiErr,
	}

	if apiErr != nil {
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
