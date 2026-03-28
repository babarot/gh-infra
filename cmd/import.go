package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/infra"
	"github.com/babarot/gh-infra/internal/ui"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <owner/repo> [owner/repo ...]",
		Short: "Export existing repository settings as YAML",
		Long:  "Fetch current GitHub repository settings and output them as gh-infra YAML.\nMultiple repositories can be specified to import them in parallel.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(args)
		},
	}
	return cmd
}

func runImport(args []string) error {
	targets, err := parseImportTargets(args)
	if err != nil {
		return err
	}

	result, err := infra.Import(targets)
	if err != nil {
		if err == context.Canceled {
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
