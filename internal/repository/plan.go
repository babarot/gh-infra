package repository

import (
	"context"
	"fmt"

	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
)

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

// PlanTargetRepoNames returns the list of repo full names that would be fetched (after filtering).
func PlanTargetRepoNames(repos []*manifest.Repository, filterRepo string) []string {
	var names []string
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			continue
		}
		names = append(names, repo.Metadata.FullName())
	}
	return names
}

// Plan fetches current state for all repositories, computes diffs, and returns changes.
func (p *Processor) Plan(ctx context.Context, repos []*manifest.Repository, opts PlanOptions, tracker RefreshTracker) ([]Change, []*manifest.Repository, error) {
	if tracker == nil {
		tracker = noopRefreshTracker{}
	}
	var targets []*manifest.Repository
	for _, repo := range repos {
		if opts.FilterRepo != "" && repo.Metadata.FullName() != opts.FilterRepo {
			logger.Debug("skip repo (filter)", "repo", repo.Metadata.FullName())
			continue
		}
		targets = append(targets, repo)
	}

	logger.Info("fetching", "repos", len(targets), "parallel", parallel.DefaultConcurrency)

	if len(targets) == 0 {
		return nil, nil, nil
	}

	diffOpts := DiffOptions{ForceSecrets: opts.ForceSecrets, Resolver: p.resolver}

	results := parallel.Map(ctx, targets, parallel.DefaultConcurrency, func(ctx context.Context, idx int, r *manifest.Repository) repoResult {
		logger.Debug("fetch start", "repo", r.Metadata.FullName())
		fullName := r.Metadata.FullName()
		onStatus := func(status string) {
			tracker.UpdateStatus(fullName, status)
		}
		current, err := p.FetchRepository(ctx, r.Metadata.Owner, r.Metadata.Name, onStatus)
		if err != nil {
			logger.Error("fetch failed", "repo", fullName, "err", err)
			tracker.Error(fullName, err)
			return repoResult{index: idx, repo: r, err: err}
		}

		changes := Diff(ctx, r, current, diffOpts)
		logger.Debug("diff done", "repo", fullName, "changes", len(changes))
		tracker.Done(fullName)
		return repoResult{index: idx, repo: r, changes: changes}
	})

	// If canceled, return immediately without printing errors.
	if ctx.Err() != nil {
		return nil, nil, nil
	}

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
