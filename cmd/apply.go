package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var (
		repo         string
		autoApprove  bool
		forceSecrets bool
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
			return runApply(path, repo, autoApprove, forceSecrets)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&forceSecrets, "force-secrets", false, "Always re-set all secrets (even if they already exist)")

	return cmd
}

func runApply(path, filterRepo string, autoApprove, forceSecrets bool) error {
	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		fmt.Println("No resources found in", path)
		return nil
	}

	manifest.ResolveSecrets(parsed.Repositories)

	runner := gh.NewRunner(false)

	fmt.Fprintf(os.Stderr, "Reading desired state from %s ...\n", path)
	fmt.Fprintf(os.Stderr, "Fetching current state from GitHub API ...\n\n")

	// Compute repo changes
	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		diffOpts := repository.DiffOptions{ForceSecrets: forceSecrets}
		repoChanges, targetRepos, err = repository.FetchAllChanges(parsed.Repositories, filterRepo, fetcher, diffOpts)
		if err != nil {
			return err
		}
	}

	// Compute file changes
	var fileChanges []fileset.FileChange
	if len(parsed.FileSets) > 0 {
		processor := fileset.NewProcessor(runner)
		fileChanges = processor.Plan(parsed.FileSets)
	}

	hasRepo := repository.HasRealChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	if !hasRepo && !hasFile {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return nil
	}

	// Print unified plan
	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, _ := fileset.CountChanges(fileChanges)
	fmt.Fprintf(os.Stdout, "\nPlan: %d to create, %d to update, %d to destroy\n\n",
		repoCreates+fileCreates, repoUpdates+fileUpdates, repoDeletes)

	if hasRepo {
		repository.PrintPlanChanges(os.Stdout, repoChanges)
	}
	if hasFile {
		fileset.PrintPlan(os.Stdout, fileChanges)
	}

	// Confirm
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

	var hasErrors bool

	// Apply repo changes
	if hasRepo {
		executor := repository.NewExecutor(runner)
		results := executor.Apply(repoChanges, targetRepos)
		repository.PrintApplyResults(os.Stdout, results)
		for _, r := range results {
			if r.Err != nil {
				hasErrors = true
			}
		}
	}

	// Apply file changes (per FileSet for correct options)
	if hasFile {
		processor := fileset.NewProcessor(runner)
		var allFileResults []fileset.FileApplyResult
		for _, fs := range parsed.FileSets {
			// Filter changes for this FileSet
			var fsChanges []fileset.FileChange
			for _, c := range fileChanges {
				if c.FileSet == fs.Metadata.Name {
					fsChanges = append(fsChanges, c)
				}
			}
			if !fileset.HasChanges(fsChanges) {
				continue
			}
			opts := fileset.ApplyOptions{
				CommitMessage: fs.Spec.CommitMessage,
				Strategy:      fs.Spec.Strategy,
				Branch:        fs.Spec.Branch,
				FileSetName:   fs.Metadata.Name,
			}
			results := processor.Apply(fsChanges, opts)
			allFileResults = append(allFileResults, results...)
		}
		fileset.PrintApplyResults(os.Stdout, allFileResults)
		fileset.PrintSummary(os.Stdout, allFileResults)
		for _, r := range allFileResults {
			if r.Err != nil {
				hasErrors = true
			}
		}
	}

	if hasErrors {
		return fmt.Errorf("apply had errors")
	}

	return nil
}
