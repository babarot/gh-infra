package main

import (
	"fmt"
	"os"

	"github.com/babarot/gh-infra/cmd"
)

var (
	version  = "dev"
	revision = "HEAD"
)

func main() {
	root := cmd.NewRootCmd(version, revision)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
