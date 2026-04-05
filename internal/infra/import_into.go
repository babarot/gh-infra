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

// ImportDiff holds the diff result of ImportInto.
type ImportDiff struct {
	Plan    *importer.Result
	Matched bool // false when no targets matched any manifest resource
	printer ui.Printer
}

// Printer returns the printer used during this session.
func (d *ImportDiff) Printer() ui.Printer { return d.printer }

// HasChanges reports whether any changes were detected.
func (d *ImportDiff) HasChanges() bool {
	if d.Plan == nil {
		return false
	}
	return d.Plan.HasChanges()
}

// DiffEntries returns file-level diff entries for the interactive diff viewer.
// Skip-only entries are excluded (shown in the console plan only).
func (d *ImportDiff) DiffEntries() []ui.DiffEntry {
	if d.Plan == nil {
		return nil
	}

	var entries []ui.DiffEntry
	for _, c := range d.Plan.FileChanges {
		if isSkipOnlyChange(c) {
			continue
		}
		switch c.Type {
		case "update":
			entry := ui.DiffEntry{
				Path:           c.DisplayPath(c.SelectedAction),
				RepoPath:       c.Path,
				Target:         c.Target,
				Icon:           ui.IconChange,
				Current:        c.CurrentForAction(c.SelectedAction),
				WriteCurrent:   c.CurrentForAction(importer.ActionWrite),
				PatchCurrent:   c.CurrentForAction(importer.ActionPatch),
				Desired:        c.Desired,
				Action:         string(c.SelectedAction),
				DefaultAction:  string(importer.DefaultAction(c.SuggestedWriteMode)),
				AllowedActions: importActionsToStrings(c.AllowedActions),
				WriteTarget:    c.DisplayPath(importer.ActionWrite),
				PatchTarget:    c.DisplayPath(importer.ActionPatch),
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

// ApplySelections writes action selections from the diff viewer back to the plan.
func (d *ImportDiff) ApplySelections(entries []ui.DiffEntry) {
	type key struct{ target, path string }
	selected := make(map[key]importer.ImportAction, len(entries))
	for _, e := range entries {
		repoPath := e.RepoPath
		if repoPath == "" {
			repoPath = e.Path
		}
		action := e.Action
		if action == "" {
			if e.Skip {
				action = string(importer.ActionSkip)
			} else if e.DefaultAction != "" {
				action = e.DefaultAction
			} else {
				action = string(importer.ActionWrite)
			}
		}
		selected[key{e.Target, repoPath}] = importer.ImportAction(action)
	}
	for i := range d.Plan.FileChanges {
		c := &d.Plan.FileChanges[i]
		action, ok := selected[key{c.Target, c.Path}]
		if !ok {
			action, ok = selected[key{c.Target, localPath(*c)}]
		}
		if !ok {
			continue
		}
		c.SelectedAction = action
		c.Current = c.CurrentForAction(action)
		c.UpdateTypeForAction()
	}
}

// MarkSkips is a compatibility wrapper for the previous skip/apply viewer model.
func (d *ImportDiff) MarkSkips(entries []ui.DiffEntry) {
	for i := range entries {
		if entries[i].Skip {
			entries[i].Action = string(importer.ActionSkip)
		} else if entries[i].Action == "" {
			entries[i].Action = string(importer.ActionWrite)
		}
		if entries[i].DefaultAction == "" {
			entries[i].DefaultAction = string(importer.ActionWrite)
		}
		if entries[i].AllowedActions == nil {
			entries[i].AllowedActions = []string{
				string(importer.ActionWrite),
				string(importer.ActionSkip),
			}
		}
	}
	d.ApplySelections(entries)
}

// Write writes the planned changes to local files.
func (d *ImportDiff) Write() error {
	return importer.Write(d.Plan)
}

// ImportInto parses manifests, matches targets, fetches GitHub state,
// compares it against local manifests, and prints the diff to the terminal.
// The returned ImportDiff provides methods for diff viewing, skip selection, and writing.
func ImportInto(args []string, into string) (*ImportDiff, error) {
	printer := ui.NewStandardPrinter()

	// Parse manifests and match targets.
	parsed, err := manifest.ParseAll(into)
	if err != nil {
		return nil, err
	}

	targets, err := parseArgs(args)
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
		printer.Message("\nNo matching resources found in manifests")
		return &ImportDiff{Matched: false, printer: printer}, nil
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

	plan, err := importer.Diff(matched, runner, printer, tracker, parsed.FileDocs)

	tracker.Wait()

	if err != nil {
		return nil, err
	}

	result := &ImportDiff{Plan: plan, Matched: true, printer: printer}

	if result.HasChanges() {
		printer.Separator()
		printImportPlan(printer, plan)
	}

	return result, nil
}

// printImportPlan prints the import plan to the terminal,
// grouped by target repo name (matching the plan command's output pattern).
func printImportPlan(p ui.Printer, plan *importer.Result) {
	seen := make(map[string]bool)
	var targets []string
	for _, d := range plan.RepoDiffs {
		if !seen[d.Target] {
			seen[d.Target] = true
			targets = append(targets, d.Target)
		}
	}
	for _, c := range plan.FileChanges {
		if c.Type == fileset.ChangeNoOp && !isSkipOnlyChange(c) {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			targets = append(targets, c.Target)
		}
	}

	repoDiffsByTarget := make(map[string][]importer.FieldDiff)
	for _, d := range plan.RepoDiffs {
		repoDiffsByTarget[d.Target] = append(repoDiffsByTarget[d.Target], d)
	}
	fileChangesByTarget := make(map[string][]importer.Change)
	for _, c := range plan.FileChanges {
		if c.Type == fileset.ChangeNoOp && !isSkipOnlyChange(c) {
			continue
		}
		fileChangesByTarget[c.Target] = append(fileChangesByTarget[c.Target], c)
	}

	for _, target := range targets {
		rDiffs := repoDiffsByTarget[target]
		fChanges := fileChangesByTarget[target]

		p.ActionHeader(target, "will be updated")
		p.GroupHeader(ui.IconChange, target)

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
					Path: c.DisplayPath(c.SelectedAction),
				}
				if isSkipOnlyChange(c) {
					item.Icon = ui.IconWarning
					item.Reason = c.Reason
				} else {
					item.Icon = ui.IconChange
					item.Added, item.Removed = fileset.DiffStat(c.CurrentForAction(c.SelectedAction), c.Desired)
				}
				p.PrintFileChange(item)
			}
		}

		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

// localPath returns the local write-back path for display.
func localPath(c importer.Change) string { return c.DisplayPath(c.SelectedAction) }

func isSkipOnlyChange(c importer.Change) bool {
	if len(c.AllowedActions) == 1 && c.AllowedActions[0] == importer.ActionSkip {
		return true
	}
	return len(c.AllowedActions) == 0 && c.WriteMode == importer.WriteSkip
}

func importActionsToStrings(actions []importer.ImportAction) []string {
	out := make([]string, len(actions))
	for i, action := range actions {
		out[i] = string(action)
	}
	return out
}

// formatImportValue formats a FieldDiff value as YAML text for display.
func formatImportValue(v any) string {
	if v == nil {
		return "(none)"
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return "(none)"
		}
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
