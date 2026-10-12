package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/babarot/gh-infra/cmd"
	"github.com/babarot/gh-infra/internal/ui"
)

var (
	version  = "dev"
	revision = "HEAD"
)

func main() {
	// One cancel for every way to interrupt: a signal, or Ctrl+C read as a
	// key by the spinner. After the first, the default signal handling is
	// back, so a second Ctrl+C ends the process at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	ui.SetInterruptHandler(stop)

	root := cmd.NewRootCmd(version, revision)
	err := root.ExecuteContext(ctx)
	stop()
	if err != nil {
		if code, ok := cmd.ExitCode(err); ok {
			os.Exit(code)
		}
		ui.FatalError(err)
		os.Exit(1)
	}
}
