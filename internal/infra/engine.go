package infra

import (
	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// Engine orchestrates the plan/apply workflow across all resource types.
type Engine struct {
	repo    *repository.Processor
	file    *fileset.Processor
	printer ui.Printer
}

// Printer returns the Engine's printer for caller-side output (e.g., summary messages).
func (e *Engine) Printer() ui.Printer {
	return e.printer
}

// New creates an Engine. It initializes the sub-processors needed for Plan and Apply.
func New(parsed *manifest.ParseResult, runner gh.Runner, resolver *manifest.Resolver, printer ui.Printer) *Engine {
	return &Engine{
		repo:    repository.NewProcessor(runner, resolver, printer),
		file:    fileset.NewProcessor(runner, printer),
		printer: printer,
	}
}
