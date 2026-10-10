package fileset

import (
	"context"
	"fmt"
	"time"

	"github.com/babarot/gh-infra/internal/parallel"
)

// ApplyOptions configures apply behavior from FileSet spec.
type ApplyOptions struct {
	CommitMessage string
	Via           string // "push" or "pull_request"
	Branch        string
	FileSetID     string
	PRTitle       string // custom PR title (pull_request only)
	PRBody        string // custom PR body (pull_request only)
	SourceURL     string // provenance URL for <% .Source.URL %> in templates
}

// Apply executes the planned file changes using Git Data API.
// Changes are grouped by target repo and applied in parallel across repos.
func (p *Processor) Apply(ctx context.Context, changes []Change, opts ApplyOptions, reporter ProgressReporter) []ApplyResult {
	if reporter == nil {
		reporter = noopProgressReporter{}
	}
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
	allResults := parallel.Map(ctx, repoList, parallel.DefaultConcurrency, func(ctx context.Context, _ int, entry repoEntry) []ApplyResult {
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
		statusFn := func(status string) {
			reporter.UpdateStatus(entry.name, status)
		}
		prURL, skipped, err := p.applyToRepo(ctx, entry.name, filesToApply, opts, statusFn)
		elapsed := time.Since(start)

		for _, c := range filesToApply {
			results = append(results, ApplyResult{
				Change:  c,
				Err:     err,
				Via:     opts.Via,
				PRURL:   prURL,
				Skipped: skipped,
			})
		}

		switch {
		case err != nil:
			reporter.Error(entry.name, elapsed, err)
		case skipped:
			reporter.Done(entry.name, elapsed, 0)
		default:
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
	Change  Change
	Err     error
	Via     string // "push" or "pull_request"
	PRURL   string // non-empty when via is pull_request
	Skipped bool   // true when GitHub already has the content and no commit was made
}

func groupChangesByTarget(changes []Change) map[string][]Change {
	grouped := make(map[string][]Change)
	for _, c := range changes {
		grouped[c.Target] = append(grouped[c.Target], c)
	}
	return grouped
}

// resolveCommitMessage returns the commit message from opts or a default,
// rendering any <% %> template expressions against the given repo.
func resolveCommitMessage(opts ApplyOptions, repo string) (string, error) {
	msg := opts.CommitMessage
	if msg == "" {
		msg = fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetID)
	}
	if !HasTemplate(msg, nil) {
		return msg, nil
	}
	return RenderCommitMessage(msg, repo, opts.SourceURL)
}
