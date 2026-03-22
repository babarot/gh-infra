package cmd

import (
	"os"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
		ui.NoResources(path)
		return nil
	}

	runner := gh.NewRunner(false)

	ui.StartPhase(path)

	// Phase 1: Refresh all resources in parallel
	var repoChanges []repository.Change
	var fileChanges []fileset.FileChange

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		g.Go(func() error {
			var fetchErr error
			repoChanges, _, fetchErr = repository.FetchAllChanges(parsed.Repositories, filterRepo, fetcher)
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

	// Phase 2: Print unified plan
	hasRepo := repository.HasRealChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	if !hasRepo && !hasFile {
		ui.NoChanges()
		if ci {
			return nil
		}
		return nil
	}

	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, _ := fileset.CountChanges(fileChanges)

	ui.PlanHeader(0, 0, 0) // top separator

	if hasRepo {
		repository.PrintPlanChanges(repoChanges)
	}
	if hasFile {
		fileset.PrintPlan(fileChanges)
	}

	ui.PlanFooter(repoCreates+fileCreates, repoUpdates+fileUpdates, repoDeletes)

	if ci {
		os.Exit(1)
	}

	return nil
}
