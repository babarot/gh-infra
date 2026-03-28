package fileset

import (
	"context"
	"fmt"
	"time"

	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

// ApplyOptions configures apply behavior from FileSet spec.
type ApplyOptions struct {
	CommitMessage string
	Via           string // "push" or "pull_request"
	Branch        string
	FileSetOwner  string
	PRTitle       string // custom PR title (pull_request only)
	PRBody        string // custom PR body (pull_request only)
}

const defaultApplyParallel = 5

// Apply executes the planned file changes using Git Data API.
// Changes are grouped by target repo and applied in parallel across repos.
func (p *Processor) Apply(ctx context.Context, changes []Change, opts ApplyOptions, reporter ui.ProgressReporter) []ApplyResult {
	grouped := groupChangesByTarget(changes)

	// Build ordered repo list for deterministic output
	type repoEntry struct {
		name    string
		changes []Change
	}
	var repoList []repoEntry
	for repo, repoChanges := range grouped {
		repoList = append(repoList, repoEntry{name: repo, changes: repoChanges})
	}

	if len(repoList) == 0 {
		return nil
	}

	// Apply repos in parallel
	allResults := parallel.Map(ctx, repoList, defaultApplyParallel, func(ctx context.Context, _ int, entry repoEntry) []ApplyResult {
		var results []ApplyResult
		var filesToApply []Change
		for _, c := range entry.changes {
			switch c.Type {
			case ChangeCreate, ChangeUpdate, ChangeDelete:
				filesToApply = append(filesToApply, c)
			case ChangeNoOp:
				// do nothing
			}
		}

		if len(filesToApply) == 0 {
			reporter.Done(entry.name, 0, 0)
			return results
		}

		paths := make([]string, len(filesToApply))
		for j, c := range filesToApply {
			paths[j] = c.Path
		}
		reporter.Start(entry.name, paths)

		start := time.Now()
		prURL, err := p.applyToRepo(ctx, entry.name, filesToApply, opts)
		elapsed := time.Since(start)

		for _, c := range filesToApply {
			results = append(results, ApplyResult{
				Change: c,
				Err:    err,
				Via:    opts.Via,
				PRURL:  prURL,
			})
		}

		if err != nil {
			reporter.Error(entry.name, elapsed, err)
		} else {
			reporter.Done(entry.name, elapsed, len(filesToApply))
		}
		return results
	})
	reporter.Wait()

	// Flatten in order
	var results []ApplyResult
	for _, r := range allResults {
		results = append(results, r...)
	}
	return results
}

type ApplyResult struct {
	Change Change
	Err    error
	Via    string // "push" or "pull_request"
	PRURL  string // non-empty when via is pull_request
}

func groupChangesByTarget(changes []Change) map[string][]Change {
	grouped := make(map[string][]Change)
	for _, c := range changes {
		grouped[c.Target] = append(grouped[c.Target], c)
	}
	return grouped
}

// resolveCommitMessage returns the commit message from opts or a default.
func resolveCommitMessage(opts ApplyOptions) string {
	if opts.CommitMessage != "" {
		return opts.CommitMessage
	}
	return fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetOwner)
}
