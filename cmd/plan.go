package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/infra"
	"github.com/babarot/gh-infra/internal/ui"
)

func newPlanCmd() *cobra.Command {
	var (
		repo          string
		ci            bool
		failOnUnknown bool
	)

	cmd := &cobra.Command{
		Use:   "plan [path]",
		Short: "Show changes between desired state and current GitHub state",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runPlan(path, repo, ci, failOnUnknown)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&ci, "ci", false, "Exit with code 1 if changes are detected")
	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")

	return cmd
}

func runPlan(path, filterRepo string, ci, failOnUnknown bool) error {
	result, err := infra.Plan(infra.PlanOptions{
		Path:          path,
		FilterRepo:    filterRepo,
		FailOnUnknown: failOnUnknown,
		DryRun:        true,
	})
	if err != nil {
		return err
	}

	if result.HasChanges {
		result.Printer.Summary("To apply, run: " + ui.Bold.Render("gh infra apply"))
		if ci {
			os.Exit(1)
		}
	}

	return nil
}
