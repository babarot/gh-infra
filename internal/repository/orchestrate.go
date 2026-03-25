package repository

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

const defaultParallel = 5

type repoResult struct {
	index   int
	repo    *manifest.Repository
	changes []Change
	err     error
}

// FetchTargetNames returns display tasks for repos that would be fetched (after filtering).
func FetchTargetNames(repos []*manifest.Repository, filterRepo string) []ui.RefreshTask {
	var tasks []ui.RefreshTask
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			continue
		}
		tasks = append(tasks, ui.RefreshTask{
			Name:      "Fetching " + repo.Metadata.FullName() + " (repo)",
			DoneLabel: "Fetched " + repo.Metadata.FullName() + " (repo)",
		})
	}
	return tasks
}

// fetchTaskKey returns the tracker key for a given repo full name.
// This must match the Name used in FetchTargetNames.
func fetchTaskKey(fullName string) string {
	return "Fetching " + fullName + " (repo)"
}

func FetchAllChanges(repos []*manifest.Repository, filterRepo string, fetcher *Fetcher, printer ui.Printer, tracker *ui.RefreshTracker, diffOpts ...DiffOptions) ([]Change, []*manifest.Repository, error) {
	var targets []*manifest.Repository
	for _, repo := range repos {
		if filterRepo != "" && repo.Metadata.FullName() != filterRepo {
			logger.Debug("skip repo (filter)", "repo", repo.Metadata.FullName())
			continue
		}
		targets = append(targets, repo)
	}

	logger.Info("fetching", "repos", len(targets), "parallel", defaultParallel)

	if len(targets) == 0 {
		return nil, nil, nil
	}

	results := parallel.Map(targets, defaultParallel, func(idx int, r *manifest.Repository) repoResult {
		logger.Debug("fetch start", "repo", r.Metadata.FullName())
		current, err := fetcher.FetchRepository(r.Metadata.Owner, r.Metadata.Name)
		if err != nil {
			logger.Error("fetch failed", "repo", r.Metadata.FullName(), "err", err)
			tracker.Error(fetchTaskKey(r.Metadata.FullName()), err)
			return repoResult{index: idx, repo: r, err: err}
		}

		changes := Diff(r, current, diffOpts...)
		logger.Debug("diff done", "repo", r.Metadata.FullName(), "changes", len(changes))
		tracker.Done(fetchTaskKey(r.Metadata.FullName()))
		return repoResult{index: idx, repo: r, changes: changes}
	})

	var allChanges []Change
	var targetRepos []*manifest.Repository
	var errors int
	for _, res := range results {
		if res.err != nil {
			printer.Error(res.repo.Metadata.FullName(), res.err.Error())
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
		printer.Warning("", fmt.Sprintf("%d %s occurred during refresh. Affected repositories were skipped.", errors, label))
	}

	logger.Info("plan complete", "total_changes", len(allChanges), "errors", errors)
	return allChanges, targetRepos, nil
}
