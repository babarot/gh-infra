package fileset

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-infra/internal/manifest"
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
	slices.SortFunc(repoList, func(a, b repoEntry) int { return strings.Compare(a.name, b.name) })

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
		// Deliver as the plan showed for this repository, which can differ
		// from the FileSet's via (PushToEmpty).
		repoOpts := opts
		if v := filesToApply[0].Via; v != "" {
			repoOpts.Via = v
		}
		prURL, via, skipped, err := p.applyToRepo(ctx, entry.name, filesToApply, repoOpts, statusFn)
		elapsed := time.Since(start)

		for _, c := range filesToApply {
			results = append(results, ApplyResult{
				Change:  c,
				Err:     err,
				Via:     via,
				PRURL:   prURL,
				Skipped: skipped,
			})
		}

		outcome := StepApplied
		switch {
		case err != nil:
			outcome = StepFailed
		case skipped:
			outcome = StepSkipped
		}
		for range filesToApply {
			reporter.Step(entry.name, outcome)
		}
		if prURL != "" && err == nil {
			reporter.PullRequest(entry.name, prURL)
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

// Delivery is how apply delivers the files of a repository, besides the
// files themselves: the method, the commit message and, for a pull
// request, its branch, title and body.
type Delivery struct {
	Via           string
	CommitMessage string
	Branch        string
	PRTitle       string
	PRBody        string
}

// DeliveryFor returns how apply delivers files to repo with opts, rendering
// templates as apply does, so a plan can tell whether two repositories get
// their files the same way. The pull request fields are set only for
// pull_request.
func DeliveryFor(opts ApplyOptions, repo string) (Delivery, error) {
	msg, err := resolveCommitMessage(opts, repo)
	if err != nil {
		return Delivery{}, fmt.Errorf("render commit message: %w", err)
	}
	d := Delivery{Via: opts.Via, CommitMessage: msg}
	if opts.Via != manifest.ViaPullRequest {
		return d, nil
	}
	d.Branch = pullRequestBranch(opts)
	if d.PRTitle, err = resolvePRTitle(opts, repo); err != nil {
		return Delivery{}, err
	}
	if d.PRBody, err = resolvePRBody(opts, repo); err != nil {
		return Delivery{}, err
	}
	return d, nil
}

// pullRequestBranch returns the branch a pull request is opened from.
func pullRequestBranch(opts ApplyOptions) string {
	if opts.Branch != "" {
		return opts.Branch
	}
	return manifest.DefaultPRBranch(opts.FileSetID)
}

// resolvePRTitle returns the pull request title from opts, or the first
// line of the commit message, rendered against repo.
func resolvePRTitle(opts ApplyOptions, repo string) (string, error) {
	if opts.PRTitle == "" {
		msg, err := resolveCommitMessage(opts, repo)
		if err != nil {
			return "", fmt.Errorf("render PR title: %w", err)
		}
		title, _ := splitCommitMessage(msg)
		return title, nil
	}
	if !HasTemplate(opts.PRTitle, nil) {
		return opts.PRTitle, nil
	}
	title, err := RenderCommitMessage(opts.PRTitle, repo, opts.SourceURL)
	if err != nil {
		return "", fmt.Errorf("render PR title: %w", err)
	}
	return title, nil
}

// resolvePRBody returns the pull request body from opts or a default,
// rendered against repo.
func resolvePRBody(opts ApplyOptions, repo string) (string, error) {
	if opts.PRBody == "" {
		return fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetID), nil
	}
	if !HasTemplate(opts.PRBody, nil) {
		return opts.PRBody, nil
	}
	body, err := RenderCommitMessage(opts.PRBody, repo, opts.SourceURL)
	if err != nil {
		return "", fmt.Errorf("render PR body: %w", err)
	}
	return body, nil
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
