package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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

	result, err := infra.ImportInto(targets)
	if err != nil {
		return err
	}

	if !result.HasChanges() {
		result.Printer().Message("\nNo changes detected")
		return nil
	}

	planPrinter := result.Printer()
	plan := result.Plan

	planPrinter.Separator()

	// Print plan to terminal (repo field diffs + file change summary).
	infra.PrintImportPlan(planPrinter, plan)

	// File-level changes go to the diff viewer for interactive confirmation.
	fileEntries := infra.BuildImportFileDiffEntries(plan)

	var ok bool
	if len(fileEntries) > 0 {
		ok, err = planPrinter.ConfirmWithDiff("Apply import changes?", fileEntries)
		if err != nil {
			return err
		}
		// Write skip selections back to plan.FileChanges.
		infra.ApplyImportSkipSelections(plan, fileEntries)
	} else {
		ok, err = planPrinter.Confirm("Apply import changes?")
	}
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if err := infra.ApplyImportInto(result); err != nil {
		return err
	}

	planPrinter.Summary(fmt.Sprintf("Import complete! %d documents updated.", plan.UpdatedDocs))
	return nil
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
