package cmd

import (
	"fmt"
	"os"

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
	repos, err := manifest.ParsePath(path)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found in", path)
		return nil
	}

	runner := gh.NewRunner(false, verbose)
	fetcher := state.NewFetcher(runner)

	fmt.Fprintf(os.Stderr, "Reading desired state from %s ...\n", path)
	fmt.Fprintf(os.Stderr, "Fetching current state from GitHub API ...\n\n")

	var allChanges []plan.Change
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			continue
		}

		if repo.Metadata.ManagedBy == "self" {
			fmt.Fprintf(os.Stderr, "  ⚠ %s: managed_by=self, skipping\n", repo.Metadata.FullName())
			continue
		}

		current, err := fetcher.FetchRepository(repo.Metadata.Owner, repo.Metadata.Name)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", repo.Metadata.FullName(), err)
		}

		changes := plan.Diff(repo, current)
		allChanges = append(allChanges, changes...)
	}

	output.PrintPlan(os.Stdout, allChanges)

	if ci && hasRealChanges(allChanges) {
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
