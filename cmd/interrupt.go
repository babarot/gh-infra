package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/babarot/gh-infra/internal/ui"
)

// InterruptError is the cause of the command's context when the run was
// interrupted: by a signal, or by Ctrl+C read as a key (SIGINT).
type InterruptError struct {
	Signal os.Signal
}

func (e *InterruptError) Error() string { return e.Signal.String() + " received" }

// ExitCode is 128 plus the signal number, as a shell reports a process the
// signal ended: 130 for SIGINT, 143 for SIGTERM.
func (e *InterruptError) ExitCode() int {
	if s, ok := e.Signal.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 128 + int(syscall.SIGINT)
}

// NotifyInterrupt returns a context that is canceled on SIGINT or SIGTERM,
// or when interrupt is called (Ctrl+C read as a key), with an
// *InterruptError as its cause. After the first, the default signal
// handling is back, so a second Ctrl+C ends the process at once. Call stop
// when the command is done.
func NotifyInterrupt(parent context.Context) (ctx context.Context, interrupt func(), stop func()) {
	ctx, cancel := context.WithCancelCause(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	var once sync.Once
	fire := func(s os.Signal) {
		once.Do(func() {
			signal.Stop(ch)
			cancel(&InterruptError{Signal: s})
		})
	}
	go func() {
		select {
		case s := <-ch:
			fire(s)
		case <-ctx.Done():
			signal.Stop(ch)
		}
	}()
	return ctx, func() { fire(os.Interrupt) }, func() {
		signal.Stop(ch)
		cancel(nil)
	}
}

// interrupted reports whether the command was interrupted by a signal or
// Ctrl+C. ctx is the command's context, which both cancel. It is checked
// first, since a gh process killed by the cancel can fail with an error of
// its own instead of context.Canceled.
func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}

// interruptedExit prints that the run was interrupted and returns the exit
// status for it: 128 plus the signal number (130 for SIGINT and Ctrl+C, 143
// for SIGTERM), so a shell, a script or CI sees that the work did not finish.
func interruptedExit(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "\n%s\n", ui.Dim.Render("Interrupted."))
	code := 128 + int(syscall.SIGINT)
	if e, ok := errors.AsType[*InterruptError](context.Cause(ctx)); ok {
		code = e.ExitCode()
	}
	return &ExitError{Code: code}
}
