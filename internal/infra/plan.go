package infra

import (
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// PlanOptions configures the plan phase.
type PlanOptions struct {
	Path          string
	FilterRepo    string
	FailOnUnknown bool
	ForceSecrets  bool // only meaningful when followed by Apply
	DryRun        bool // true = plan only (skip secret resolution)
}

// PlanResult holds the outcome of the plan phase (pure data, no dependencies).
type PlanResult struct {
	RepoChanges []repository.Change
	FileChanges []fileset.Change
	TargetRepos []*manifest.Repository
	Parsed      *manifest.ParseResult

	Creates int
	Updates int
	Deletes int

	HasChanges bool
}

// Plan parses manifests, fetches current state, computes diffs, and prints the plan.
// It returns a PlanResult and an Engine that can be used for Apply.
func Plan(opts PlanOptions) (*Engine, *PlanResult, error) {
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(opts.Path, manifest.ParseOptions{FailOnUnknown: opts.FailOnUnknown})
	if err != nil {
		return nil, nil, err
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("deprecation", w)
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + opts.Path)
		return nil, &PlanResult{}, nil
	}

	if !opts.DryRun {
		manifest.ResolveSecrets(parsed.Repositories)
	}

	runner := gh.NewRunner(false)

	var resolverOwner string
	if len(parsed.Repositories) > 0 {
		resolverOwner = parsed.Repositories[0].Metadata.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)

	engine := New(parsed, runner, resolver, p)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", opts.Path))
	p.Phase("Fetching current state from GitHub API ...")
	p.BlankLine()

	// Collect all target names and start a single spinner display
	var allTasks []ui.RefreshTask
	allTasks = append(allTasks, repository.PlanTargetNames(parsed.Repositories, opts.FilterRepo)...)
	allTasks = append(allTasks, fileset.PlanTargetNames(parsed.FileSets, opts.FilterRepo)...)
	tracker := ui.RunRefresh(allTasks)

	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	var fileChanges []fileset.Change

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		g.Go(func() error {
			var fetchErr error
			repoChanges, targetRepos, fetchErr = engine.repo.Plan(parsed.Repositories, repository.PlanOptions{
				FilterRepo:   opts.FilterRepo,
				ForceSecrets: opts.ForceSecrets,
			}, tracker)
			return fetchErr
		})
	}

	if len(parsed.FileSets) > 0 {
		g.Go(func() error {
			var planErr error
			fileChanges, planErr = engine.file.Plan(parsed.FileSets, opts.FilterRepo, tracker)
			return planErr
		})
	}

	if err := g.Wait(); err != nil {
		tracker.Wait()
		return nil, nil, err
	}
	tracker.Wait()

	hasRepo := repository.HasChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	result := &PlanResult{
		RepoChanges: repoChanges,
		FileChanges: fileChanges,
		TargetRepos: targetRepos,
		Parsed:      parsed,
		HasChanges:  hasRepo || hasFile,
	}

	if !result.HasChanges {
		p.Message("\nNo changes. Infrastructure is up-to-date.")
		return engine, result, nil
	}

	// Count and print unified plan
	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, fileDeletes := fileset.CountChanges(fileChanges)
	result.Creates = repoCreates + fileCreates
	result.Updates = repoUpdates + fileUpdates
	result.Deletes = repoDeletes + fileDeletes

	p.Separator()
	p.Legend(result.Creates > 0, result.Updates > 0, result.Deletes > 0)

	PrintPlan(p, repoChanges, fileChanges)

	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", result.Creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", result.Updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", result.Deletes))),
	}
	p.Summary(fmt.Sprintf("Plan: %s", strings.Join(parts, ", ")))

	return engine, result, nil
}
