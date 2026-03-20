package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/plan"
	"github.com/babarot/gh-infra/internal/state"
	"golang.org/x/sync/semaphore"
)

const defaultParallel = 5

type repoResult struct {
	index   int
	repo    *manifest.Repository
	changes []plan.Change
	err     error
}

// fetchAllChanges fetches current state and computes diffs for all repos in parallel.
func fetchAllChanges(repos []*manifest.Repository, filterRepo string, fetcher *state.Fetcher, diffOpts ...plan.DiffOptions) ([]plan.Change, []*manifest.Repository, error) {
	// Filter repos first
	var targets []*manifest.Repository
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			logger.Debug("skip repo (filter)", "repo", repo.Metadata.FullName())
			continue
		}
		if repo.Metadata.ManagedBy == manifest.ManagedBySelf {
			fmt.Fprintf(os.Stderr, "  ⚠ %s: managed_by=self, skipping\n", repo.Metadata.FullName())
			continue
		}
		targets = append(targets, repo)
	}

	logger.Info("fetching", "repos", len(targets), "parallel", defaultParallel)

	if len(targets) == 0 {
		return nil, nil, nil
	}

	results := make([]repoResult, len(targets))
	sem := semaphore.NewWeighted(defaultParallel)
	var wg sync.WaitGroup

	for i, repo := range targets {
		wg.Add(1)
		go func(idx int, r *manifest.Repository) {
			defer wg.Done()

			_ = sem.Acquire(context.Background(), 1)
			defer sem.Release(1)

			logger.Debug("fetch start", "repo", r.Metadata.FullName())
			current, err := fetcher.FetchRepository(r.Metadata.Owner, r.Metadata.Name)
			if err != nil {
				logger.Error("fetch failed", "repo", r.Metadata.FullName(), "err", err)
				results[idx] = repoResult{index: idx, repo: r, err: err}
				return
			}

			changes := plan.Diff(r, current, diffOpts...)
			logger.Debug("diff done", "repo", r.Metadata.FullName(), "changes", len(changes))
			results[idx] = repoResult{index: idx, repo: r, changes: changes}
		}(i, repo)
	}

	wg.Wait()

	var allChanges []plan.Change
	var targetRepos []*manifest.Repository
	for _, res := range results {
		if res.err != nil {
			return nil, nil, fmt.Errorf("fetch %s: %w", res.repo.Metadata.FullName(), res.err)
		}
		allChanges = append(allChanges, res.changes...)
		targetRepos = append(targetRepos, res.repo)
	}

	logger.Info("plan complete", "total_changes", len(allChanges))
	return allChanges, targetRepos, nil
}
