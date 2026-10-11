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
		output        string
		changedSince  string
		detailedExit  bool
	)

	cmd := &cobra.Command{
		Use:   "plan [path...]",
		Short: "Show changes between desired state and current GitHub state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(args, planCommandOptions{
				FilterRepo:       repo,
				CI:               ci,
				FailOnUnknown:    failOnUnknown,
				ShowDiff:         showDiff,
				Output:           output,
				ChangedSince:     changedSince,
				DetailedExitCode: detailedExit,
			})
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&ci, "ci", false, "Exit with code 1 if changes are detected")
	cmd.Flags().BoolVar(&detailedExit, "detailed-exitcode", false, "Exit 2 if changes are detected, 1 on error, 0 otherwise")
	_ = cmd.Flags().MarkDeprecated("ci", "use --detailed-exitcode instead (exits 2 on changes, 1 on errors)")
	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")
	cmd.Flags().BoolVar(&showDiff, "diff", false, "Show unified diff for FileSet file changes (prints full content)")
	cmd.Flags().StringVarP(&output, "output", "o", infra.OutputText, "Output format: text, json, or markdown")
	cmd.Flags().StringVar(&changedSince, "changed-since", "", "Only manifests changed since this git ref, with the local files they read (e.g. origin/main)")

	return cmd
}

type planCommandOptions struct {
	FilterRepo    string
	CI            bool
	FailOnUnknown bool
	ShowDiff      bool
	Output        string
	ChangedSince  string

	DetailedExitCode bool
}

func runPlan(paths []string, opts planCommandOptions) error {
	if err := infra.ValidateOutput(opts.Output, infra.OutputJSON, infra.OutputMarkdown); err != nil {
		return err
	}
	if opts.CI {
		ui.DisableStyles()
	}

	result, err := infra.Plan(infra.PlanOptions{
		Paths:         paths,
		FilterRepo:    opts.FilterRepo,
		FailOnUnknown: opts.FailOnUnknown,
		DryRun:        true,
		ShowDiff:      opts.ShowDiff,
		Output:        opts.Output,
		ChangedSince:  opts.ChangedSince,
	})
	return finishPlan(result, err, opts)
}

// finishPlan prints the machine-readable document and turns the outcome into
// the command's exit status. With --detailed-exitcode: 0 no changes, 1 error
// (including an interrupted plan and no manifests found), 2 changes. With the
// deprecated --ci: 1 for changes or errors. Otherwise 1 only for errors.
// An error always wins over changes.
func finishPlan(result *infra.PlanResult, err error, opts planCommandOptions) error {
	if err != nil {
		switch {
		case errors.Is(err, infra.ErrNoResources):
			// Already reported; an empty path is not "no changes".
			if opts.DetailedExitCode {
				return &ExitError{Code: 1}
			}
			return nil
		case errors.Is(err, context.Canceled):
			printCancelled()
			if opts.DetailedExitCode {
				return &ExitError{Code: 1}
			}
			return nil
		}
		return err
	}

	switch opts.Output {
	case infra.OutputJSON:
		if err := result.PlanDocument(opts.ShowDiff).Write(os.Stdout); err != nil {
			return err
		}
	case infra.OutputMarkdown:
		if err := result.PlanDocument(opts.ShowDiff).WriteMarkdown(os.Stdout); err != nil {
			return err
		}
	}

	// A plan that skipped repositories is incomplete, so fail even when the
	// remaining repositories have no changes.
	if err := result.FailedTargetsError(); err != nil {
		return err
	}

	if !result.HasChanges {
		return nil
	}
	if opts.Output == infra.OutputText || opts.Output == "" {
		result.Printer().Summary("To apply, run: " + ui.Bold.Render("gh infra apply"))
	}
	switch {
	case opts.DetailedExitCode:
		return &ExitError{Code: 2}
	case opts.CI:
		return &ExitError{Code: 1}
	}
	return nil
}
