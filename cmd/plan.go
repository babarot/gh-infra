package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"unicode"

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
			return runPlan(cmd.Context(), args, planCommandOptions{
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
	Paths         []string // as given on the command line, for the apply hint
	FilterRepo    string
	CI            bool
	FailOnUnknown bool
	ShowDiff      bool
	Output        string
	ChangedSince  string

	DetailedExitCode bool
}

func runPlan(ctx context.Context, paths []string, opts planCommandOptions) error {
	if err := infra.ValidateOutput(opts.Output, infra.OutputJSON, infra.OutputMarkdown); err != nil {
		return err
	}
	if opts.CI {
		ui.DisableStyles()
	}

	runner, err := newRunner()
	if err != nil {
		return err
	}
	result, err := infra.Plan(ctx, infra.PlanOptions{
		Runner:        runner,
		Paths:         paths,
		FilterRepo:    opts.FilterRepo,
		FailOnUnknown: opts.FailOnUnknown,
		DryRun:        true,
		ShowDiff:      opts.ShowDiff,
		Output:        opts.Output,
		ChangedSince:  opts.ChangedSince,
	})
	opts.Paths = paths
	return finishPlan(ctx, result, err, opts)
}

// applyCommand returns the apply command for what this plan covered: the
// same paths, --repo and --changed-since.
func applyCommand(opts planCommandOptions) string {
	args := []string{"gh", "infra", "apply"}
	for _, p := range opts.Paths {
		args = append(args, shellQuote(p))
	}
	if opts.FilterRepo != "" {
		args = append(args, "--repo", shellQuote(opts.FilterRepo))
	}
	if opts.ChangedSince != "" {
		args = append(args, "--changed-since", shellQuote(opts.ChangedSince))
	}
	return strings.Join(args, " ")
}

// shellQuote quotes s for a POSIX shell when it holds anything but letters,
// digits and the punctuation common in paths and git refs.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_./:@%+=,~^", r)
	}
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !safe(r) }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// finishPlan prints the machine-readable document and turns the outcome into
// the command's exit status. With --detailed-exitcode: 0 no changes, 1 error
// (including an interrupted plan and no manifests found), 2 changes. With the
// deprecated --ci: 1 for changes or errors. Otherwise 1 only for errors.
// An error always wins over changes.
func finishPlan(ctx context.Context, result *infra.PlanResult, err error, opts planCommandOptions) error {
	if err != nil {
		switch {
		case interrupted(ctx, err):
			printCancelled()
			if opts.DetailedExitCode {
				return &ExitError{Code: 1}
			}
			return nil
		case errors.Is(err, infra.ErrNoResources):
			// Already reported; an empty path is not "no changes".
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
		result.Printer().Summary("To apply, run: " + ui.Bold.Render(applyCommand(opts)))
	}
	switch {
	case opts.DetailedExitCode:
		return &ExitError{Code: 2}
	case opts.CI:
		return &ExitError{Code: 1}
	}
	return nil
}
