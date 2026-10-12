package repository

import (
	"context"
	"fmt"
	"strings"

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
	fatal   bool
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

	results := parallel.Map(ctx, targets, parallel.DefaultConcurrency, func(ctx context.Context, idx int, r *manifest.Repository) repoResult {
		// Names in rulesets belong to this repository's organization.
		diffOpts := DiffOptions{ForceSecrets: opts.ForceSecrets, Resolver: p.resolver.For(r.Metadata.Owner)}
		logger.Debug("fetch start", "repo", r.Metadata.FullName())
		fullName := r.Metadata.FullName()
		onStatus := func(status string) {
			tracker.UpdateStatus(fullName, status)
		}
		current, err := p.FetchRepository(ctx, r.Metadata.Owner, r.Metadata.Name, &r.Spec, onStatus)
		if err != nil {
			logger.Error("fetch failed", "repo", fullName, "err", err)
			tracker.Error(fullName, err)
			return repoResult{index: idx, repo: r, err: err}
		}
		tracker.Checkpoint(fullName, "fetched repository state")

		// Cross-field dependencies that need current state to evaluate.
		if err := ValidateDependencies(r, current); err != nil {
			logger.Error("dependency validation failed", "repo", fullName, "err", err)
			tracker.Error(fullName, err)
			return repoResult{index: idx, repo: r, err: err, fatal: true}
		}

		changes := Diff(ctx, r, current, diffOpts)
		// gh-infra does not change an archived repository except to unarchive
		// it: report the repository instead of planning changes to it.
		if err := archivedChangesError(current, changes); err != nil {
			logger.Error("changes to an archived repository", "repo", fullName, "err", err)
			tracker.Error(fullName, err)
			return repoResult{index: idx, repo: r, err: err}
		}
		if hasLabelDeleteChanges(changes) {
			tracker.UpdateStatus(fullName, "checking label usage...")
			p.enrichLabelDeleteInfo(ctx, changes)
		}
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
	var skipped int
	var firstFatal error
	for _, res := range results {
		if res.err != nil {
			// Fetch/validate errors are already surfaced via the tracker
			// (live inline on the spinner, then collected for post-spinner
			// reporting in RefreshTracker.PrintErrors).
			if res.fatal && firstFatal == nil {
				firstFatal = fmt.Errorf("%s: %w", res.repo.Metadata.FullName(), res.err)
			}
			skipped++
			continue
		}
		allChanges = append(allChanges, res.changes...)
		targetRepos = append(targetRepos, res.repo)
	}
	if firstFatal != nil {
		return nil, nil, firstFatal
	}

	logger.Info("plan complete", "total_changes", len(allChanges), "skipped", skipped)
	return allChanges, targetRepos, nil
}

func hasLabelDeleteChanges(changes []Change) bool {
	for _, c := range changes {
		if c.Resource == manifest.ResourceLabel && c.Type == ChangeDelete {
			return true
		}
	}
	return false
}

// archivedChangesError returns an error when current is archived and changes
// would write to it without unarchiving it first (Diff puts an unarchive
// first). GitHub makes an archived repository read-only; some of its APIs
// still accept writes (secrets, variables, topics, Actions permissions), but
// gh-infra does not rely on that and leaves an archived repository alone.
func archivedChangesError(current *CurrentState, changes []Change) error {
	if !current.Archived || !HasChanges(changes) {
		return nil
	}
	if c := changes[0]; c.Resource == manifest.ResourceRepository && c.Field == "archived" && !IsArchive(c) {
		return nil
	}
	const shown = 3
	var names []string
	for _, c := range changes {
		if c.Type == ChangeNoOp {
			continue
		}
		names = append(names, applyStatusTarget(c))
	}
	list := strings.Join(names[:min(shown, len(names))], ", ")
	if len(names) > shown {
		list += fmt.Sprintf(" and %d more", len(names)-shown)
	}
	return fmt.Errorf("repository is archived (read-only): gh-infra does not change an archived repository except to unarchive it, so these changes are not applied: %s; set archived: false to change it, or remove the changes from the manifest", list)
}
