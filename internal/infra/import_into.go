package infra

import (
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

// ImportOptions configures the Import function.
type ImportOptions struct {
	Args []string // owner/repo arguments
	Into string   // non-empty = import into local manifests
}

// ImportResult holds the outcome of the Import function.
type ImportResult struct {
	// stdout mode fields
	YAMLDocs  [][]byte
	Errors    map[string]error
	Succeeded int
	Failed    int

	// --into mode: diff result for local manifest updates.
	Plan *importer.Result

	printer ui.Printer
}

// Printer returns the printer used during this session.
func (r *ImportResult) Printer() ui.Printer { return r.printer }

// HasChanges reports whether any changes were detected (--into mode).
func (r *ImportResult) HasChanges() bool {
	if r.Plan == nil {
		return false
	}
	return r.Plan.HasChanges()
}

// DiffEntries returns file-level diff entries for the interactive diff viewer.
// WriteSkip entries are excluded (shown in the console plan only).
func (r *ImportResult) DiffEntries() []ui.DiffEntry {
	if r.Plan == nil {
		return nil
	}

	var entries []ui.DiffEntry
	for _, c := range r.Plan.FileChanges {
		if c.WriteMode == importer.WriteSkip {
			continue
		}
		switch c.Type {
		case "update":
			entry := ui.DiffEntry{
				Path:    localPath(c),
				Target:  c.Target,
				Icon:    ui.IconChange,
				Current: c.Current,
				Desired: c.Desired,
			}
			for _, w := range c.Warnings {
				entry.Icon = ui.IconWarning
				if entry.Current != "" {
					entry.Current += "\n# " + w
				}
			}
			entries = append(entries, entry)
		}
	}
	return entries
}

// MarkSkips writes skip selections from the diff viewer back to the plan,
// setting skipped entries to NoOp so they are not imported.
func (r *ImportResult) MarkSkips(entries []ui.DiffEntry) {
	type key struct{ target, path string }
	skipped := make(map[key]bool, len(entries))
	for _, e := range entries {
		if e.Skip {
			skipped[key{e.Target, e.Path}] = true
		}
	}
	for i := range r.Plan.FileChanges {
		c := &r.Plan.FileChanges[i]
		if skipped[key{c.Target, c.Path}] {
			c.Type = fileset.ChangeNoOp
		}
	}
}

// Write writes the planned changes to local files (--into mode).
func (r *ImportResult) Write() error {
	return importer.Write(r.Plan)
}

// importInto parses manifests, matches targets, fetches GitHub state,
// compares it against local manifests, and prints the diff to the terminal.
func importInto(opts ImportOptions) (*ImportResult, error) {
	printer := ui.NewStandardPrinter()

	// Parse manifests and match targets.
	parsed, err := manifest.ParseAll(opts.Into)
	if err != nil {
		return nil, err
	}

	targets, err := parseArgs(opts.Args)
	if err != nil {
		return nil, err
	}

	var matched []importer.TargetMatches
	for _, tm := range targets {
		matches := importer.FindMatches(parsed, tm.Target.FullName())
		if matches.IsEmpty() {
			printer.Warning(tm.Target.FullName(), "not found in manifests, skipping")
			continue
		}
		matched = append(matched, importer.TargetMatches{Target: tm.Target, Matches: matches})
	}

	if len(matched) == 0 {
		return &ImportResult{printer: printer}, nil
	}

	runner := gh.NewRunner(false)

	// Build spinner tasks.
	var tasks []ui.RefreshTask
	for _, tm := range matched {
		tasks = append(tasks, ui.RefreshTask{
			Name:      tm.Target.FullName(),
			FailLabel: tm.Target.FullName(),
		})
	}

	printer.Phase("Fetching current state from GitHub API ...")
	printer.BlankLine()

	tracker := ui.RunRefresh(tasks)

	plan, err := importer.Diff(matched, runner, printer, tracker)

	tracker.Wait()

	if err != nil {
		return nil, err
	}

	result := &ImportResult{Plan: plan, printer: printer}

	if result.HasChanges() {
		printer.Separator()
		printImportPlan(printer, plan)
	}

	return result, nil
}

// printImportPlan prints the import plan to the terminal,
// grouped by target repo name (matching the plan command's output pattern).
func printImportPlan(p ui.Printer, plan *importer.Result) {
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
				if len(localPath(c)) > w {
					w = len(localPath(c))
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
					Path: localPath(c),
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

// localPath returns the local write-back path for display.
func localPath(c importer.Change) string {
	if c.LocalTarget != "" {
		return c.LocalTarget
	}
	if c.ManifestPath != "" {
		return c.ManifestPath + ":" + c.Path
	}
	return c.Path
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
