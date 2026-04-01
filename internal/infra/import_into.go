package infra

import (
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/ui"
)

// ImportInto plans changes for import --into by fetching GitHub state and
// comparing it against local manifests.
func ImportInto(targets []importer.TargetMatches) (*importer.IntoPlan, ui.Printer, error) {
	runner := gh.NewRunner(false)
	printer := ui.NewStandardPrinter()

	// Build spinner tasks.
	var tasks []ui.RefreshTask
	for _, tm := range targets {
		tasks = append(tasks, ui.RefreshTask{
			Name:      tm.Target.FullName(),
			FailLabel: tm.Target.FullName(),
		})
	}

	printer.Phase("Fetching current state from GitHub API ...")
	printer.BlankLine()

	tracker := ui.RunRefresh(tasks)

	plan, err := importer.PlanInto(targets, runner, printer, tracker)

	tracker.Wait()

	return plan, printer, err
}
