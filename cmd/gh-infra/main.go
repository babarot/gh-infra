package main

import (
	"os"

	"github.com/babarot/gh-infra/cmd"
	"github.com/babarot/gh-infra/internal/ui"
)

var (
	version  = "dev"
	revision = "HEAD"
)

func main() {
	root := cmd.NewRootCmd(version, revision)
	if err := root.Execute(); err != nil {
		ui.FatalError(err)
		os.Exit(1)
	}
}
