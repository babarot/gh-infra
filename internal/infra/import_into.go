package infra

import (
	"context"
	"fmt"
	"strings"

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
	Skipped int  // number of targets skipped due to fetch/validation errors
	printer ui.Printer
}

const (
	importActionWrite = "write"
	importActionPatch = "patch"
	importActionSkip  = "skip"
)

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
			defaultAction := defaultActionForChange(c)
			defaultMode := writeModeForAction(c, defaultAction)
			entry := ui.DiffEntry{
				Path:           c.DisplayPathForMode(defaultMode),
				RepoPath:       c.Path,
				Target:         c.Target,
				Icon:           ui.IconChange,
				Current:        c.CurrentForMode(defaultMode),
				SkipCurrent:    c.CurrentForMode(defaultMode),
				WriteCurrent:   c.CurrentForMode(writeModeForAction(c, importActionWrite)),
				PatchCurrent:   c.CurrentForMode(importer.WritePatch),
				Desired:        c.Desired,
				Action:         defaultAction,
				DefaultAction:  defaultAction,
				AllowedActions: allowedActionsForChange(c),
				WriteTarget:    c.DisplayPathForMode(writeModeForAction(c, importActionWrite)),
				PatchTarget:    c.DisplayPathForMode(importer.WritePatch),
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
	selected := make(map[key]string, len(entries))
	for _, e := range entries {
		repoPath := e.RepoPath
		if repoPath == "" {
			repoPath = e.Path
		}
		action := e.Action
		if action == "" {
			if e.Skip {
				action = importActionSkip
			} else if e.DefaultAction != "" {
				action = e.DefaultAction
			} else {
				action = importActionWrite
			}
		}
		selected[key{e.Target, repoPath}] = action
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
		c.UpdateTypeForMode(writeModeForAction(*c, action))
	}
}

// MarkSkips is a compatibility wrapper for the previous skip/apply viewer model.
func (d *ImportDiff) MarkSkips(entries []ui.DiffEntry) {
	for i := range entries {
		if entries[i].Skip {
			entries[i].Action = importActionSkip
		} else if entries[i].Action == "" {
			entries[i].Action = importActionWrite
		}
		if entries[i].DefaultAction == "" {
			entries[i].DefaultAction = importActionWrite
		}
		if entries[i].AllowedActions == nil {
			entries[i].AllowedActions = []string{
				importActionWrite,
				importActionSkip,
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

	runner := gh.NewRunner(false)
	sourceResolver := manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
		return runner.Run(ctx, args...)
	})

	// Parse manifests and match targets.
	parsed, err := manifest.ParseAll(into, manifest.ParseOptions{Resolver: sourceResolver})
	if err != nil {
		return nil, err
	}

	// When no args are given, target all repositories defined in manifests.
	if len(args) == 0 {
		args = allRepoFullNames(parsed)
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

	// Build spinner tasks.
	var tasks []ui.RefreshTask
	for _, tm := range matched {
		tasks = append(tasks, ui.RefreshTask{
			Name:      tm.Target.FullName(),
			DoneLabel: "fetched repository state",
			FailLabel: tm.Target.FullName(),
		})
	}

	printer.Phase(fmt.Sprintf("Reading desired state from %s ...", tildePath(into)))
	printer.Phase("Fetching current state from GitHub API ...")
	printer.BlankLine()

	tracker := ui.RunRefresh(tasks)
	ctx, cancel := withTrackerCancelContext(tracker)
	defer cancel()

	plan, err := importer.Diff(ctx, importer.DiffOptions{
		Targets:     matched,
		Runner:      runner,
		Tracker:     tracker,
		AllFileDocs: parsed.FileDocs,
	})

	tracker.Wait()
	tracker.PrintErrors()

	if ctx.Err() != nil {
		return nil, context.Canceled
	}
	if err != nil {
		return nil, err
	}

	result := &ImportDiff{
		Plan:    plan,
		Matched: true,
		Skipped: len(tracker.Errors()),
		printer: printer,
	}

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

		// Build header: include source file path when available
		header := target
		if len(rDiffs) > 0 && rDiffs[0].SourcePath != "" {
			header = fmt.Sprintf("%s (%s)", target, rDiffs[0].SourcePath)
		}
		p.ActionHeader(header, "will be updated")
		p.GroupHeader(ui.IconChange, target)

		if len(rDiffs) > 0 {
			groups := fieldDiffsToDiffGroups(rDiffs)
			p.SetColumnWidth(ui.DiffGroupFieldWidth(groups))
			ui.RenderDiffGroups(p, groups)
		}

		if len(fChanges) > 0 {
			w := 0
			for _, c := range fChanges {
				if len(displayPathForPlan(c)) > w {
					w = len(displayPathForPlan(c))
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
				defaultMode := defaultModeForChange(c)
				item := ui.FileItem{
					Path: c.DisplayPathForMode(defaultMode),
				}
				if isSkipOnlyChange(c) {
					item.Icon = ui.IconWarning
					item.Reason = planSkipReason(c)
				} else if defaultMode == importer.WriteSkip {
					item.Icon = ui.IconWarning
					item.Reason = planSkipReason(c)
				} else {
					item.Icon = ui.IconChange
					item.Added, item.Removed = fileset.DiffStat(c.CurrentForMode(defaultMode), c.Desired)
				}
				p.PrintFileChange(item)
			}
		}

		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

// localPath returns the local write-back path for display.
func localPath(c importer.Change) string { return c.DisplayPathForMode(c.EffectiveWriteMode()) }

func displayPathForPlan(c importer.Change) string {
	return c.DisplayPathForMode(defaultModeForChange(c))
}

func isSkipOnlyChange(c importer.Change) bool {
	return len(allowedActionsForChange(c)) == 0
}

func allowedActionsForChange(c importer.Change) []string {
	actions := []string{}
	if c.SupportsMode(importer.WriteSource) || c.SupportsMode(importer.WriteInline) {
		actions = append(actions, importActionWrite)
	}
	if c.SupportsMode(importer.WritePatch) {
		actions = append(actions, importActionPatch)
	}
	if len(actions) == 0 {
		return nil
	}
	actions = append(actions, importActionSkip)
	return actions
}

func defaultActionForChange(c importer.Change) string {
	if c.CreateOnly {
		if c.HasExistingPatches && c.SupportsMode(importer.WritePatch) {
			return importActionPatch
		}
		return importActionSkip
	}

	switch c.SuggestedWriteMode {
	case importer.WritePatch:
		return importActionPatch
	case importer.WriteSkip:
		return importActionSkip
	default:
		return importActionWrite
	}
}

func defaultModeForChange(c importer.Change) importer.WriteMode {
	return writeModeForAction(c, defaultActionForChange(c))
}

func planSkipReason(c importer.Change) string {
	if c.Reason != "" {
		return "skip: " + c.Reason
	}
	if c.CreateOnly && !c.HasExistingPatches {
		return "skip: reconcile:create_only (Tab to change)"
	}
	return "skip"
}

// fieldDiffsToDiffGroups converts flat FieldDiff slices into the unified
// DiffGroup model by parsing dot-notation field names and grouping by prefix.
func fieldDiffsToDiffGroups(diffs []importer.FieldDiff) []ui.DiffGroup {
	var result []ui.DiffGroup

	// Prefix classification
	nestedObject := map[string]bool{
		"features": true, "merge_strategy": true, "actions": true, "security": true,
	}
	keyedCollection := map[string]bool{
		"branch_protection": true, "rulesets": true,
	}
	flatCollection := map[string]bool{
		"labels": true, "variables": true, "milestones": true,
	}

	i := 0
	for i < len(diffs) {
		d := diffs[i]
		prefix, rest, hasDot := splitField(d.Field)

		if !hasDot {
			// Bare field (description, visibility, topics, etc.)
			result = append(result, ui.DiffGroup{
				Items: []ui.DiffItem{fieldDiffToItem(d, d.Field)},
			})
			i++
			continue
		}

		if nestedObject[prefix] {
			// Collect consecutive diffs with same prefix
			dg := ui.DiffGroup{Header: prefix, Icon: ui.IconChange}
			for i < len(diffs) {
				p, r, ok := splitField(diffs[i].Field)
				if !ok || p != prefix {
					break
				}
				dg.Items = append(dg.Items, fieldDiffToItem(diffs[i], r))
				i++
			}
			dg.Icon = aggregateIcon(dg.Items)
			result = append(result, dg)
			continue
		}

		if keyedCollection[prefix] {
			// Each unique key becomes its own group: prefix[key]
			key := rest
			header := fmt.Sprintf("%s[%s]", prefix, key)
			dg := ui.DiffGroup{Header: header}
			dg.Items = append(dg.Items, fieldDiffToItem(d, key))
			dg.Icon = aggregateIcon(dg.Items)
			result = append(result, dg)
			i++
			continue
		}

		if flatCollection[prefix] {
			// Collect consecutive diffs with same prefix under one header
			dg := ui.DiffGroup{Header: prefix}
			for i < len(diffs) {
				p, r, ok := splitField(diffs[i].Field)
				if !ok || p != prefix {
					break
				}
				dg.Items = append(dg.Items, fieldDiffToItem(diffs[i], r))
				i++
			}
			dg.Icon = aggregateIcon(dg.Items)
			result = append(result, dg)
			continue
		}

		// Unknown prefix (fallback): group consecutive same-prefix diffs
		dg := ui.DiffGroup{Header: prefix}
		for i < len(diffs) {
			p, r, ok := splitField(diffs[i].Field)
			if !ok || p != prefix {
				break
			}
			dg.Items = append(dg.Items, fieldDiffToItem(diffs[i], r))
			i++
		}
		dg.Icon = aggregateIcon(dg.Items)
		result = append(result, dg)
	}

	return result
}

func splitField(field string) (prefix, rest string, hasDot bool) {
	prefix, rest, hasDot = strings.Cut(field, ".")
	return
}

func fieldDiffToItem(d importer.FieldDiff, field string) ui.DiffItem {
	icon := ui.IconChange
	if d.Old == nil && d.New != nil {
		icon = ui.IconAdd
	} else if d.Old != nil && d.New == nil {
		icon = ui.IconRemove
	}
	return ui.DiffItem{
		Icon:  icon,
		Field: field,
		Old:   ui.FormatValue(d.Old),
		New:   ui.FormatValue(d.New),
	}
}

func aggregateIcon(items []ui.DiffItem) string {
	allAdd, allRemove := true, true
	for _, item := range items {
		if item.Icon != ui.IconAdd {
			allAdd = false
		}
		if item.Icon != ui.IconRemove {
			allRemove = false
		}
	}
	if allAdd {
		return ui.IconAdd
	}
	if allRemove {
		return ui.IconRemove
	}
	return ui.IconChange
}

// allRepoFullNames returns deduplicated "owner/repo" names from all parsed manifests.
func allRepoFullNames(parsed *manifest.ParseResult) []string {
	seen := make(map[string]bool)
	var names []string

	for _, doc := range parsed.RepositoryDocs {
		name := doc.Resource.Metadata.FullName()
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, doc := range parsed.FileDocs {
		for _, repo := range doc.Resource.Spec.Repositories {
			name := doc.Resource.RepoFullName(repo.Name)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func writeModeForAction(c importer.Change, action string) importer.WriteMode {
	switch action {
	case importActionPatch:
		return importer.WritePatch
	case importActionSkip:
		return importer.WriteSkip
	default:
		if c.SupportsMode(importer.WriteSource) {
			return importer.WriteSource
		}
		return importer.WriteInline
	}
}
