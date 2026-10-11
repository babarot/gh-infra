package infra

import (
	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// engine holds the runtime context shared between Plan and Apply.
type engine struct {
	repo    *repository.Processor
	file    *fileset.Processor
	printer ui.Printer
	quiet   bool      // machine-readable output: print nothing
	runner  gh.Runner // shared by every call of the command
}

func newEngine(runner gh.Runner, resolver *manifest.Resolver, printer ui.Printer) *engine {
	return &engine{
		runner:  runner,
		repo:    repository.NewProcessor(runner, resolver),
		file:    fileset.NewProcessor(runner, printer),
		printer: printer,
	}
}
