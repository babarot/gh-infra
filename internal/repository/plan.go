package repository

import (
	"context"
	"fmt"

	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

const defaultParallel = 5

// PlanOptions configures the plan phase for repositories.
type PlanOptions struct {
	FilterRepo   string
	ForceSecrets bool
}

type repoResult struct {
	index   int
	repo    *manifest.Repository
	changes []Change
	err     error
}

// PlanTargetNames returns display tasks for repos that would be fetched (after filtering).
func PlanTargetNames(repos []*manifest.Repository, filterRepo string) []ui.RefreshTask {
	var names []string
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			continue
		}
		names = append(names, repo.Metadata.FullName())
	}
	return ui.BuildRefreshTasks(names, "repo")
}

// planTaskKey returns the tracker key for a given repo full name.
func planTaskKey(fullName string) string {
	return "Fetching " + fullName + " (repo)"
}

// Plan fetches current state for all repositories, computes diffs, and returns changes.
func (p *Processor) Plan(ctx context.Context, repos []*manifest.Repository, opts PlanOptions, tracker *ui.RefreshTracker) ([]Change, []*manifest.Repository, error) {
	var targets []*manifest.Repository
	for _, repo := range repos {
		if opts.FilterRepo != "" && repo.Metadata.FullName() != opts.FilterRepo {
			logger.Debug("skip repo (filter)", "repo", repo.Metadata.FullName())
			continue
		}
		targets = append(targets, repo)
	}

	logger.Info("fetching", "repos", len(targets), "parallel", defaultParallel)

	if len(targets) == 0 {
		return nil, nil, nil
	}

	diffOpts := DiffOptions{ForceSecrets: opts.ForceSecrets, Resolver: p.resolver}

	results := parallel.Map(ctx, targets, defaultParallel, func(ctx context.Context, idx int, r *manifest.Repository) repoResult {
		logger.Debug("fetch start", "repo", r.Metadata.FullName())
		current, err := p.FetchRepository(ctx, r.Metadata.Owner, r.Metadata.Name)
		if err != nil {
			logger.Error("fetch failed", "repo", r.Metadata.FullName(), "err", err)
			tracker.Error(planTaskKey(r.Metadata.FullName()), err)
			return repoResult{index: idx, repo: r, err: err}
		}

		changes := Diff(ctx, r, current, diffOpts)
		logger.Debug("diff done", "repo", r.Metadata.FullName(), "changes", len(changes))
		tracker.Done(planTaskKey(r.Metadata.FullName()))
		return repoResult{index: idx, repo: r, changes: changes}
	})

	var allChanges []Change
	var targetRepos []*manifest.Repository
	var errors int
	for _, res := range results {
		if res.err != nil {
			p.printer.Error(res.repo.Metadata.FullName(), res.err.Error())
			errors++
			continue
		}
		allChanges = append(allChanges, res.changes...)
		targetRepos = append(targetRepos, res.repo)
	}

	if errors > 0 {
		label := "error"
		if errors > 1 {
			label = "errors"
		}
		p.printer.Warning("", fmt.Sprintf("%d %s occurred during refresh. Affected repositories were skipped.", errors, label))
	}

	logger.Info("plan complete", "total_changes", len(allChanges), "errors", errors)
	return allChanges, targetRepos, nil
}
