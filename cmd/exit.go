package cmd

import (
	"errors"
	"fmt"

	"github.com/babarot/gh-infra/internal/infra"
)

// ExitError ends the process with Code without printing an error: the
// command has already reported its outcome (e.g. plan --detailed-exitcode
// exits 2 when there are changes).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// ExitCode returns the exit code carried by an ExitError in err, and 1 for
// an error that was already shown (infra.ErrAlreadyReported).
func ExitCode(err error) (int, bool) {
	if e, ok := errors.AsType[*ExitError](err); ok {
		return e.Code, true
	}
	if errors.Is(err, infra.ErrAlreadyReported) {
		return 1, true
	}
	return 0, false
}
