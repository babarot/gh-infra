package infra

import (
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/ui"
)

// ImportIntoResult holds the outcome of the import-into plan phase.
type ImportIntoResult struct {
	Plan    *importer.IntoPlan
	printer ui.Printer
}

// Printer returns the printer used during this session.
func (r *ImportIntoResult) Printer() ui.Printer { return r.printer }

// HasChanges reports whether any changes were detected.
func (r *ImportIntoResult) HasChanges() bool { return r.Plan.HasChanges() }

// ImportInto plans changes for import --into by fetching GitHub state and
// comparing it against local manifests.
func ImportInto(targets []importer.TargetMatches) (*ImportIntoResult, error) {
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

	if err != nil {
		return nil, err
	}

	return &ImportIntoResult{Plan: plan, printer: printer}, nil
}

// PrintImportPlan prints the import plan to the terminal,
// grouped by target repo name (matching the plan command's output pattern).
func PrintImportPlan(p ui.Printer, plan *importer.IntoPlan) {
	// Collect all target names in order.
	seen := make(map[string]bool)
	var targets []string
	for _, d := range plan.RepoDiffs {
		if !seen[d.Target] {
			seen[d.Target] = true
			targets = append(targets, d.Target)
		}
	}
	for _, c := range plan.FileChanges {
		if c.Type == fileset.ChangeNoOp && c.WriteMode != importer.WriteSkip {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			targets = append(targets, c.Target)
		}
	}

	// Index by target.
	repoDiffsByTarget := make(map[string][]importer.FieldDiff)
	for _, d := range plan.RepoDiffs {
		repoDiffsByTarget[d.Target] = append(repoDiffsByTarget[d.Target], d)
	}
	fileChangesByTarget := make(map[string][]importer.Change)
	for _, c := range plan.FileChanges {
		if c.Type == fileset.ChangeNoOp && c.WriteMode != importer.WriteSkip {
			continue
		}
		fileChangesByTarget[c.Target] = append(fileChangesByTarget[c.Target], c)
	}

	for _, target := range targets {
		rDiffs := repoDiffsByTarget[target]
		fChanges := fileChangesByTarget[target]

		p.ActionHeader(target, "will be updated")
		p.GroupHeader(ui.IconChange, target)

		// Print repo-level field diffs.
		if len(rDiffs) > 0 {
			w := 0
			for _, d := range rDiffs {
				if len(d.Field) > w {
					w = len(d.Field)
				}
			}
			p.SetColumnWidth(w)

			for _, d := range rDiffs {
				p.PrintChange(ui.ChangeItem{
					Icon:  ui.IconChange,
					Field: d.Field,
					Old:   formatImportValue(d.Old),
					New:   formatImportValue(d.New),
				})
			}
		}

		// Print file-level change summary.
		if len(fChanges) > 0 {
			w := 0
			for _, c := range fChanges {
				if len(ImportLocalPath(c)) > w {
					w = len(ImportLocalPath(c))
				}
			}
			p.SetColumnWidth(w)

			count := len(fChanges)
			label := fmt.Sprintf("%d file", count)
			if count != 1 {
				label += "s"
			}
			p.SubGroupHeader(ui.IconChange, fmt.Sprintf("FileSet: %s", ui.Bold.Render(label)))

			for _, c := range fChanges {
				item := ui.FileItem{
					Path: ImportLocalPath(c),
				}
				if c.WriteMode == importer.WriteSkip {
					item.Icon = ui.IconWarning
					item.Reason = c.Reason
				} else {
					item.Icon = ui.IconChange
					item.Added, item.Removed = fileset.DiffStat(c.Current, c.Desired)
				}
				p.PrintFileChange(item)
			}
		}

		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

// BuildImportFileDiffEntries converts file-level changes into DiffEntry items for the diff viewer.
func BuildImportFileDiffEntries(plan *importer.IntoPlan) []ui.DiffEntry {
	var entries []ui.DiffEntry

	for _, c := range plan.FileChanges {
		entry := ui.DiffEntry{
			Path:   ImportLocalPath(c),
			Target: c.Target,
		}
		// WriteSkip entries cannot be applied — shown in console plan only.
		if c.WriteMode == importer.WriteSkip {
			continue
		}
		switch c.Type {
		case "update":
			entry.Icon = ui.IconChange
			entry.Current = c.Current
			entry.Desired = c.Desired
		default:
			continue // noop, etc.
		}

		for _, w := range c.Warnings {
			entry.Icon = ui.IconWarning
			if entry.Current != "" {
				entry.Current += "\n# " + w
			}
		}

		entries = append(entries, entry)
	}

	return entries
}

// ApplyImportSkipSelections writes skip selections from the diff viewer back
// to plan.FileChanges, setting skipped entries to NoOp so they are not applied.
func ApplyImportSkipSelections(plan *importer.IntoPlan, entries []ui.DiffEntry) {
	type key struct{ target, path string }
	skipped := make(map[key]bool, len(entries))
	for _, e := range entries {
		if e.Skip {
			skipped[key{e.Target, e.Path}] = true
		}
	}
	for i := range plan.FileChanges {
		c := &plan.FileChanges[i]
		if skipped[key{c.Target, c.Path}] {
			c.Type = fileset.ChangeNoOp
		}
	}
}

// ImportLocalPath returns the local write-back path for display.
func ImportLocalPath(c importer.Change) string {
	if c.LocalTarget != "" {
		return c.LocalTarget
	}
	if c.ManifestPath != "" {
		return c.ManifestPath + ":" + c.Path
	}
	return c.Path
}

// ApplyImportInto writes the planned changes to disk.
func ApplyImportInto(result *ImportIntoResult) error {
	return importer.ApplyInto(result.Plan)
}

// formatImportValue formats a FieldDiff value as YAML text for display.
func formatImportValue(v any) string {
	if v == nil {
		return "(none)"
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	}
	data, err := goyaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return strings.TrimRight(string(data), "\n")
}
