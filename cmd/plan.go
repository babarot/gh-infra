package cmd

import (
	"fmt"
	"os"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/output"
	"github.com/babarot/gh-infra/internal/plan"
	"github.com/babarot/gh-infra/internal/state"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	var (
		repo string
		ci   bool
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
			return runPlan(path, repo, ci)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&ci, "ci", false, "Exit with code 1 if changes are detected")

	return cmd
}

func runPlan(path, filterRepo string, ci bool) error {
	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		fmt.Println("No resources found in", path)
		return nil
	}

	runner := gh.NewRunner(false)

	fmt.Fprintf(os.Stderr, "Reading desired state from %s ...\n", path)
	fmt.Fprintf(os.Stderr, "Fetching current state from GitHub API ...\n\n")

	hasAnyChanges := false

	// Repository changes
	if len(parsed.Repositories) > 0 {
		fetcher := state.NewFetcher(runner)
		allChanges, _, err := fetchAllChanges(parsed.Repositories, filterRepo, fetcher)
		if err != nil {
			return err
		}
		output.PrintPlan(os.Stdout, allChanges)
		if hasRealChanges(allChanges) {
			hasAnyChanges = true
		}
	}

	// FileSet changes
	if len(parsed.FileSets) > 0 {
		processor := fileset.NewProcessor(runner)
		fileChanges := processor.Plan(parsed.FileSets)
		fileset.PrintPlan(os.Stdout, fileChanges)
		if fileset.HasChanges(fileChanges) {
			hasAnyChanges = true
		}
	}

	if !hasAnyChanges {
		fmt.Println("No changes. Infrastructure is up-to-date.")
	}

	if ci && hasAnyChanges {
		os.Exit(1)
	}

	return nil
}

func hasRealChanges(changes []plan.Change) bool {
	for _, c := range changes {
		if c.Type != plan.ChangeNoOp {
			return true
		}
	}
	return false
}
