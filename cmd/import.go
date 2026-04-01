package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/infra"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

func newImportCmd() *cobra.Command {
	var intoPath string

	cmd := &cobra.Command{
		Use:   "import <owner/repo> [owner/repo ...]",
		Short: "Export existing repository settings as YAML",
		Long: "Fetch current GitHub repository settings and output them as gh-infra YAML.\n" +
			"With --into, pull GitHub state back into existing local manifests.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if intoPath != "" {
				return runImportInto(args, intoPath)
			}
			return runImport(args)
		},
	}

	cmd.Flags().StringVar(&intoPath, "into", "",
		"Pull GitHub state into existing local manifests (dir or file path)")

	return cmd
}

func runImport(args []string) error {
	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}

	result, err := infra.Import(targets)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			printCancelled()
			return nil
		}
		return err
	}

	p := result.Printer()

	p.Separator()

	// Output YAML in order
	out := p.OutWriter()
	for i, doc := range result.YAMLDocs {
		if i > 0 {
			fmt.Fprintln(out, "---")
		}
		fmt.Fprint(out, string(doc))
	}

	// Print errors to stderr so they remain visible when stdout is redirected
	for name, err := range result.Errors {
		p.Warning(name, fmt.Sprintf("skipping: %v", err))
	}

	// Summary
	summaryMsg := fmt.Sprintf("Import complete! %s exported", ui.Bold.Render(fmt.Sprintf("%d", result.Succeeded)))
	if result.Failed > 0 {
		summaryMsg += fmt.Sprintf(", %s failed", ui.Bold.Render(fmt.Sprintf("%d", result.Failed)))
	}
	summaryMsg += "."
	p.Summary(summaryMsg)
	return nil
}

func runImportInto(args []string, intoPath string) error {
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(intoPath)
	if err != nil {
		return err
	}

	var targets []importer.TargetMatches
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid target: %q (expected owner/repo)", arg)
		}
		target := importer.Target{Owner: parts[0], Name: parts[1]}
		matches := importer.FindMatches(parsed, target.FullName())
		if matches.IsEmpty() {
			p.Warning(target.FullName(), "not found in manifests, skipping")
			continue
		}
		targets = append(targets, importer.TargetMatches{Target: target, Matches: matches})
	}

	if len(targets) == 0 {
		p.Message("No matching resources found in manifests")
		return nil
	}

	plan, planPrinter, err := infra.ImportInto(targets)
	if err != nil {
		return err
	}

	if !plan.HasChanges() {
		planPrinter.Message("\nNo changes detected")
		return nil
	}

	planPrinter.Separator()

	// Print repo-level field diffs to terminal (same pattern as plan command).
	printImportRepoDiffs(planPrinter, plan)

	// File-level changes go to the diff viewer for interactive confirmation.
	fileEntries := buildImportFileDiffEntries(plan)

	var ok bool
	if len(fileEntries) > 0 {
		ok, err = planPrinter.ConfirmWithDiff("Apply import changes?", fileEntries)
		if err != nil {
			return err
		}
		// Write skip selections back to plan.FileChanges.
		applyImportSkipSelections(plan, fileEntries)
	} else {
		ok, err = planPrinter.Confirm("Apply import changes?")
	}
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := importer.ApplyInto(plan); err != nil {
		return err
	}

	planPrinter.Summary(fmt.Sprintf("Import complete! %d documents updated.", plan.UpdatedDocs))
	return nil
}

// applyImportSkipSelections writes skip selections from the diff viewer back
// to plan.FileChanges, setting skipped entries to NoOp so they are not applied.
func applyImportSkipSelections(plan *importer.IntoPlan, entries []ui.DiffEntry) {
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

// printImportRepoDiffs prints repo-level field diffs to the terminal,
// grouped by target repo name (matching the plan command's output pattern).
func printImportRepoDiffs(p ui.Printer, plan *importer.IntoPlan) {
	if len(plan.RepoDiffs) == 0 {
		return
	}

	// Group diffs by target.
	type group struct {
		name  string
		diffs []importer.FieldDiff
	}
	seen := make(map[string]int) // target → index in groups
	var groups []group
	for _, d := range plan.RepoDiffs {
		idx, ok := seen[d.Target]
		if !ok {
			idx = len(groups)
			seen[d.Target] = idx
			groups = append(groups, group{name: d.Target})
		}
		groups[idx].diffs = append(groups[idx].diffs, d)
	}

	for _, g := range groups {
		p.ActionHeader(g.name, "will be updated")
		p.GroupHeader(ui.IconChange, g.name)

		// Compute column width for alignment.
		w := 0
		for _, d := range g.diffs {
			if len(d.Field) > w {
				w = len(d.Field)
			}
		}
		p.SetColumnWidth(w)

		for _, d := range g.diffs {
			p.PrintChange(ui.ChangeItem{
				Icon: ui.IconChange,
				Old:  formatDiffValue(d.Old),
				New:  formatDiffValue(d.New),
				Field: d.Field,
			})
		}

		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

// buildImportFileDiffEntries converts file-level changes into DiffEntry items for the diff viewer.
func buildImportFileDiffEntries(plan *importer.IntoPlan) []ui.DiffEntry {
	var entries []ui.DiffEntry

	for _, c := range plan.FileChanges {
		entry := ui.DiffEntry{
			Path:   c.Path,
			Target: c.Target,
		}
		switch c.WriteMode {
		case importer.WriteSkip:
			entry.Icon = ui.IconWarning
			entry.Skip = true
			entry.Current = c.Reason
			entry.Desired = c.Reason
		default:
			switch c.Type {
			case "update":
				entry.Icon = ui.IconChange
				entry.Current = c.Current
				entry.Desired = c.Desired
			case "noop":
				continue
			}
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

// formatDiffValue formats a FieldDiff value as YAML text for the diff viewer.
// Scalar types (string, bool, nil) are rendered inline; complex types (structs,
// slices, maps) are marshaled to multi-line YAML so the unified diff is readable.
func formatDiffValue(v any) string {
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
	// Complex types: marshal to YAML.
	data, err := goyaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return strings.TrimRight(string(data), "\n")
}

func parseImportTargets(args []string) ([]infra.ImportTarget, error) {
	var targets []infra.ImportTarget
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target: %q (expected owner/repo)", arg)
		}
		targets = append(targets, infra.ImportTarget{Owner: parts[0], Name: parts[1]})
	}
	return targets, nil
}
