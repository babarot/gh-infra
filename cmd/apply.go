package cmd

import (
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + path)
		return nil
	}

	manifest.ResolveSecrets(parsed.Repositories)

	runner := gh.NewRunner(false)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", path))
	p.Phase("Fetching current state from GitHub API ...")
	fmt.Fprintln(p.ErrWriter())

	// Compute all changes in parallel
	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	var fileChanges []fileset.FileChange

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		diffOpts := repository.DiffOptions{ForceSecrets: forceSecrets}
		g.Go(func() error {
			var fetchErr error
			repoChanges, targetRepos, fetchErr = repository.FetchAllChanges(parsed.Repositories, filterRepo, fetcher, p, diffOpts)
			return fetchErr
		})
	}

	if len(parsed.FileSets) > 0 {
		processor := fileset.NewProcessor(runner)
		g.Go(func() error {
			fileChanges = processor.Plan(parsed.FileSets)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	hasRepo := repository.HasRealChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	if !hasRepo && !hasFile {
		p.Message("\nNo changes. Infrastructure is up-to-date.")
		return nil
	}

	// Print unified plan
	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, fileDrifts := fileset.CountChanges(fileChanges)

	p.Separator()

	if hasRepo {
		repository.PrintPlanChanges(p, repoChanges)
	}
	if hasFile {
		fileset.PrintPlan(p, fileChanges)
	}

	creates := repoCreates + fileCreates
	updates := repoUpdates + fileUpdates
	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", repoDeletes))),
	}
	if fileDrifts > 0 {
		parts = append(parts, fmt.Sprintf("%s drifted", ui.Bold.Render(fmt.Sprintf("%d", fileDrifts))))
	}
	p.Summary(fmt.Sprintf("Plan: %s\nTo apply, run: %s", strings.Join(parts, ", "), ui.Bold.Render("gh infra apply")))

	// Confirm
	if !autoApprove {
		confirmed, err := p.Confirm("Do you want to apply these changes?")
		if err != nil {
			return err
		}
		if !confirmed {
			p.Message("Apply cancelled.")
			return nil
		}
	}

	totalSucceeded := 0
	totalFailed := 0

	// Apply repo changes
	if hasRepo {
		executor := repository.NewExecutor(runner)
		results := executor.Apply(repoChanges, targetRepos)
		repository.PrintApplyResults(p, results)
		s, f := repository.CountApplyResults(results)
		totalSucceeded += s
		totalFailed += f
	}

	// Apply file changes (per FileSet for correct options)
	if hasFile {
		processor := fileset.NewProcessor(runner)
		for _, fs := range parsed.FileSets {
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
			fileset.PrintApplyResults(p, results)
			for _, r := range results {
				if r.Skipped {
					continue
				}
				if r.Err != nil {
					totalFailed++
				} else {
					totalSucceeded++
				}
			}
		}
	}

	// Unified summary
	summaryMsg := fmt.Sprintf("Apply complete! %d changes applied", totalSucceeded)
	if totalFailed > 0 {
		summaryMsg += fmt.Sprintf(", %d failed", totalFailed)
	}
	summaryMsg += "."
	p.Summary(summaryMsg)

	if totalFailed > 0 {
		return fmt.Errorf("apply had errors")
	}

	return nil
}
