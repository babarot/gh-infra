package cmd

import (
	"fmt"
	"os"
	"strings"

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
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + path)
		return nil
	}

	runner := gh.NewRunner(false)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", path))
	p.Phase("Fetching current state from GitHub API ...")
	fmt.Fprintln(p.ErrWriter())

	// Phase 1: Refresh all resources in parallel
	var repoChanges []repository.Change
	var fileChanges []fileset.FileChange

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		g.Go(func() error {
			var fetchErr error
			repoChanges, _, fetchErr = repository.FetchAllChanges(parsed.Repositories, filterRepo, fetcher, p)
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
		p.Message("\nNo changes. Infrastructure is up-to-date.")
		if ci {
			return nil
		}
		return nil
	}

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

	if ci {
		os.Exit(1)
	}

	return nil
}
