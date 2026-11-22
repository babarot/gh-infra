package cmd

import (
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestInterruptError_ExitCode(t *testing.T) {
	for _, tt := range []struct {
		sig  os.Signal
		want int
	}{
		{syscall.SIGINT, 130},
		{os.Interrupt, 130},
		{syscall.SIGTERM, 143},
	} {
		if got := (&InterruptError{Signal: tt.sig}).ExitCode(); got != tt.want {
			t.Errorf("%v: exit code = %d, want %d", tt.sig, got, tt.want)
		}
	}
}

func interruptCause(t *testing.T, ctx context.Context) os.Signal {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context was not canceled")
	}
	e, ok := errors.AsType[*InterruptError](context.Cause(ctx))
	if !ok {
		t.Fatalf("cause = %v, want *InterruptError", context.Cause(ctx))
	}
	return e.Signal
}

// Ctrl+C read as a key is a SIGINT.
func TestNotifyInterrupt_Key(t *testing.T) {
	ctx, interrupt, stop := NotifyInterrupt(context.Background())
	defer stop()
	interrupt()
	if got := interruptCause(t, ctx); got != os.Interrupt {
		t.Errorf("signal = %v, want %v", got, os.Interrupt)
	}
	// A later interrupt or stop does not change the cause.
	interrupt()
	stop()
	if got := interruptCause(t, ctx); got != os.Interrupt {
		t.Errorf("signal after stop = %v, want %v", got, os.Interrupt)
	}
}

func TestNotifyInterrupt_SIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot send itself SIGTERM on Windows")
	}
	ctx, _, stop := NotifyInterrupt(context.Background())
	defer stop()
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if got := interruptCause(t, ctx); got != syscall.SIGTERM {
		t.Errorf("signal = %v, want %v", got, syscall.SIGTERM)
	}
}

func TestNotifyInterrupt_StopIsNotAnInterrupt(t *testing.T) {
	ctx, _, stop := NotifyInterrupt(context.Background())
	stop()
	if _, ok := errors.AsType[*InterruptError](context.Cause(ctx)); ok {
		t.Errorf("cause = %v, want no interrupt", context.Cause(ctx))
	}
}
