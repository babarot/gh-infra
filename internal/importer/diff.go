package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/repository"
)

// DiffOptions configures import diff planning.
type DiffOptions struct {
	Targets     []TargetMatches
	Runner      gh.Runner
	Tracker     RefreshTracker
	AllFileDocs []*manifest.FileDocument
}

// fetchResult holds the result of a parallel repository fetch.
type fetchResult struct {
	tm       TargetMatches
	imported *manifest.Repository
	fatal    error // auth errors, context canceled — abort all
	skip     bool  // fetch failed non-fatally — skip this target
}

// Diff builds a change plan for all targets.
//
// Phase 1 fetches GitHub state for all targets in parallel.
// Phase 2 computes diffs and patches manifests sequentially, because
// manifestBytes is a shared write-back cache that accumulates patches
// across targets referencing the same file.
func Diff(ctx context.Context, opts DiffOptions) (*Result, error) {
	tracker := opts.Tracker
	if tracker == nil {
		tracker = noopRefreshTracker{}
	}
	var completedMu sync.Mutex
	completed := make(map[string]bool, len(opts.Targets))
	completeDone := func(name string) {
		completedMu.Lock()
		if completed[name] {
			completedMu.Unlock()
			return
		}
		completed[name] = true
		completedMu.Unlock()
		tracker.Done(name)
	}
	completeError := func(name string, err error) {
		completedMu.Lock()
		if completed[name] {
			completedMu.Unlock()
			return
		}
		completed[name] = true
		completedMu.Unlock()
		tracker.Error(name, err)
	}
	completeFail := func(name string) {
		completedMu.Lock()
		if completed[name] {
			completedMu.Unlock()
			return
		}
		completed[name] = true
		completedMu.Unlock()
		tracker.Fail(name)
	}
	completeRemaining := func() {
		for _, tm := range opts.Targets {
			fullName := tm.Target.FullName()
			completedMu.Lock()
			if completed[fullName] {
				completedMu.Unlock()
				continue
			}
			completedMu.Unlock()
			completeFail(fullName)
		}
	}

	// Bound to no owner: ToManifest resolves names against each repository's.
	resolver := manifest.NewResolver(opts.Runner)
	proc := repository.NewProcessor(opts.Runner, resolver)

	// Build source reference counts across ALL file documents (not just matched ones)
	// to detect shared templates that should not be overwritten.
	sourceRefCount := buildSourceRefCount(opts.AllFileDocs)

	// ── Phase 1: Fetch all targets in parallel ──────────────────────────
	fetched := parallel.Map(ctx, opts.Targets, parallel.DefaultConcurrency, func(ctx context.Context, _ int, tm TargetMatches) fetchResult {
		fullName := tm.Target.FullName()
		onStatus := func(s string) {
			tracker.UpdateStatus(fullName, s)
		}

		current, err := proc.FetchRepository(ctx, tm.Target.Owner, tm.Target.Name, nil, onStatus)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return fetchResult{tm: tm, fatal: context.Canceled}
			}
			if errors.Is(err, gh.ErrUnauthorized) || errors.Is(err, gh.ErrForbidden) {
				completeFail(fullName)
				return fetchResult{tm: tm, fatal: fmt.Errorf("fetch %s: %w", fullName, err)}
			}
			completeError(fullName, fmt.Errorf("fetch failed: %w", err))
			return fetchResult{tm: tm, skip: true}
		}
		if current.IsNew {
			completeError(fullName, fmt.Errorf("repository not found on GitHub"))
			return fetchResult{tm: tm, skip: true}
		}

		imported := repository.ToManifest(ctx, current, resolver)
		tracker.Checkpoint(fullName, "fetched repository state")
		return fetchResult{tm: tm, imported: imported}
	})

	// Abort on fatal errors (auth failure, context canceled).
	for _, f := range fetched {
		if f.fatal != nil {
			completeRemaining()
			return nil, f.fatal
		}
	}
	if ctx.Err() != nil {
		completeRemaining()
		return nil, context.Canceled
	}

	// ── Phase 2: Diff and patch sequentially ────────────────────────────
	// manifestBytes is a shared write-back cache; patches from one target
	// must be visible to the next target referencing the same file.
	plan := &Result{
		ManifestEdits: make(map[string][]byte),
	}
	manifestBytes := make(map[string][]byte)

	for _, f := range fetched {
		if f.skip {
			continue
		}

		fullName := f.tm.Target.FullName()
		if ctx.Err() != nil {
			completeRemaining()
			return nil, context.Canceled
		}

		// Plan FileSet matches first: a target whose files cannot be read is
		// skipped as a whole, before its repository edits reach the shared
		// manifestBytes. DiffFiles does not touch manifestBytes.
		var fileChanges []Change
		if len(f.tm.Matches.FileSets) > 0 {
			var err error
			fileChanges, err = DiffFiles(ctx, opts.Runner, f.tm.Matches.FileSets, DiffFilesOptions{
				FilterRepo:     fullName,
				SourceRefCount: sourceRefCount,
				OnStatus: func(status string) {
					tracker.UpdateStatus(fullName, status)
				},
			})
			switch {
			case err == nil:
			case errors.Is(err, context.Canceled) || ctx.Err() != nil:
				// A gh process killed by the cancel fails with an error of
				// its own, so ctx tells an interrupt.
				completeRemaining()
				return nil, context.Canceled
			case errors.Is(err, gh.ErrUnauthorized):
				// The token is not valid, so no other target would do better.
				completeFail(fullName)
				completeRemaining()
				return nil, fmt.Errorf("plan files %s: %w", fullName, err)
			default:
				// Like a target that could not be fetched: report it and
				// leave the whole target out. A 403 here is about this
				// repository only, since its state was read in phase 1.
				completeError(fullName, err)
				continue
			}
		}

		// Ensure manifest bytes are loaded for all relevant source paths.
		if err := ensureManifestBytes(manifestBytes, f.tm.Matches); err != nil {
			completeError(fullName, err)
			completeRemaining()
			return nil, err
		}

		// Plan Repository matches.
		if len(f.tm.Matches.Repositories) > 0 {
			rp, err := DiffRepository(DiffInput{
				Repos:         f.tm.Matches.Repositories,
				Imported:      f.imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				err := fmt.Errorf("plan repository %s: %w", fullName, err)
				completeError(fullName, err)
				completeRemaining()
				return nil, err
			}
			plan.AddRepoResult(rp)
		}

		// Plan RepositorySet matches.
		if len(f.tm.Matches.RepositorySets) > 0 {
			rp, err := DiffRepositorySet(DiffInput{
				Repos:         f.tm.Matches.RepositorySets,
				Imported:      f.imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				err := fmt.Errorf("plan repositoryset %s: %w", fullName, err)
				completeError(fullName, err)
				completeRemaining()
				return nil, err
			}
			plan.AddRepoResult(rp)
		}

		plan.FileChanges = append(plan.FileChanges, fileChanges...)
		completeDone(fullName)
	}

	return plan, nil
}

// ensureManifestBytes loads manifest files referenced by matches into the shared map.
func ensureManifestBytes(manifestBytes map[string][]byte, m Matches) error {
	paths := make(map[string]bool)
	for _, doc := range m.Repositories {
		paths[doc.SourcePath] = true
	}
	for _, doc := range m.RepositorySets {
		paths[doc.SourcePath] = true
	}
	for _, doc := range m.FileSets {
		paths[doc.SourcePath] = true
	}

	for p := range paths {
		if _, ok := manifestBytes[p]; ok {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read manifest %s: %w", p, err)
		}
		manifestBytes[p] = data
	}
	return nil
}
