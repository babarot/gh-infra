package cmd

import (
	"context"
	"errors"
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
		showDiff      bool
	)

	cmd := &cobra.Command{
		Use:   "plan [path...]",
		Short: "Show changes between desired state and current GitHub state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(args, planCommandOptions{
				FilterRepo:    repo,
				CI:            ci,
				FailOnUnknown: failOnUnknown,
				ShowDiff:      showDiff,
			})
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&ci, "ci", false, "Exit with code 1 if changes are detected")
	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "Show unified diff for each file change")

	return cmd
}

type planCommandOptions struct {
	FilterRepo    string
	CI            bool
	FailOnUnknown bool
	ShowDiff      bool
}

func runPlan(paths []string, opts planCommandOptions) error {
	if opts.CI {
		ui.DisableStyles()
	}

	result, err := infra.Plan(infra.PlanOptions{
		Paths:         paths,
		FilterRepo:    opts.FilterRepo,
		FailOnUnknown: opts.FailOnUnknown,
		DryRun:        true,
		ShowDiff:      opts.ShowDiff,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			printCancelled()
			return nil
		}
		return err
	}

	if result.HasChanges {
		result.Printer().Summary("To apply, run: " + ui.Bold.Render("gh infra apply"))
		if opts.CI {
			os.Exit(1)
		}
	}

	return nil
}
