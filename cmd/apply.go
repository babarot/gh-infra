package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/apply"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/output"
	"github.com/babarot/gh-infra/internal/plan"
	"github.com/babarot/gh-infra/internal/state"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var (
		repo        string
		autoApprove bool
	)

	cmd := &cobra.Command{
		Use:   "apply [path]",
		Short: "Apply desired state to GitHub",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runApply(path, repo, autoApprove)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip confirmation prompt")

	return cmd
}

func runApply(path, filterRepo string, autoApprove bool) error {
	repos, err := manifest.ParsePath(path)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found in", path)
		return nil
	}

	manifest.ResolveSecrets(repos)

	runner := gh.NewRunner(false, verbose)
	fetcher := state.NewFetcher(runner)

	fmt.Fprintf(os.Stderr, "Reading desired state from %s ...\n", path)
	fmt.Fprintf(os.Stderr, "Fetching current state from GitHub API ...\n\n")

	var allChanges []plan.Change
	var targetRepos []*manifest.Repository

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
		targetRepos = append(targetRepos, repo)
	}

	if !hasRealChanges(allChanges) {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return nil
	}

	output.PrintPlan(os.Stdout, allChanges)

	if !autoApprove {
		fmt.Print("\nDo you want to apply these changes? (yes/no): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Println("Apply cancelled.")
			return nil
		}
		fmt.Println()
	}

	executor := apply.NewExecutor(runner)
	results := executor.Apply(allChanges, targetRepos)

	output.PrintApplyResults(os.Stdout, results)

	for _, r := range results {
		if r.Err != nil {
			return fmt.Errorf("apply had errors")
		}
	}

	return nil
}
