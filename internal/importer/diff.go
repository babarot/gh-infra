package importer

import (
	"context"
	"errors"
	"fmt"
	"os"

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

	// Determine resolver owner from first target.
	var resolverOwner string
	if len(opts.Targets) > 0 {
		resolverOwner = opts.Targets[0].Target.Owner
	}
	resolver := manifest.NewResolver(opts.Runner, resolverOwner)
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

		current, err := proc.FetchRepository(ctx, tm.Target.Owner, tm.Target.Name, onStatus)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return fetchResult{tm: tm, fatal: context.Canceled}
			}
			if errors.Is(err, gh.ErrUnauthorized) || errors.Is(err, gh.ErrForbidden) {
				tracker.Fail(fullName)
				return fetchResult{tm: tm, fatal: fmt.Errorf("fetch %s: %w", fullName, err)}
			}
			tracker.Error(fullName, fmt.Errorf("fetch failed: %w", err))
			return fetchResult{tm: tm, skip: true}
		}
		if current.IsNew {
			tracker.Error(fullName, fmt.Errorf("repository not found on GitHub"))
			return fetchResult{tm: tm, skip: true}
		}

		imported := repository.ToManifest(ctx, current, resolver)
		return fetchResult{tm: tm, imported: imported}
	})

	// Abort on fatal errors (auth failure, context canceled).
	for _, f := range fetched {
		if f.fatal != nil {
			return nil, f.fatal
		}
	}
	if ctx.Err() != nil {
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

		// Ensure manifest bytes are loaded for all relevant source paths.
		if err := ensureManifestBytes(manifestBytes, f.tm.Matches); err != nil {
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
				return nil, fmt.Errorf("plan repository %s: %w", fullName, err)
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
				return nil, fmt.Errorf("plan repositoryset %s: %w", fullName, err)
			}
			plan.AddRepoResult(rp)
		}

		// Plan FileSet matches.
		if len(f.tm.Matches.FileSets) > 0 {
			fileChanges, err := DiffFiles(ctx, opts.Runner, f.tm.Matches.FileSets, DiffFilesOptions{
				FilterRepo:     fullName,
				SourceRefCount: sourceRefCount,
				OnStatus: func(status string) {
					tracker.UpdateStatus(fullName, status)
				},
			})
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, context.Canceled
				}
				return nil, fmt.Errorf("plan files %s: %w", fullName, err)
			}
			plan.FileChanges = append(plan.FileChanges, fileChanges...)
		}

		tracker.Done(fullName)
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
