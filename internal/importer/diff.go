package importer

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

// Diff builds a change plan for all targets.
// manifestBytes is shared across targets so patches accumulate correctly.
// Targets are processed sequentially (same file may be patched by multiple targets).
func Diff(targets []TargetMatches, runner gh.Runner, printer DiagnosticPrinter, tracker RefreshTracker, allFileDocs []*manifest.FileDocument) (*Result, error) {
	if printer == nil {
		printer = noopDiagnosticPrinter{}
	}
	if tracker == nil {
		tracker = noopRefreshTracker{}
	}
	plan := &Result{
		ManifestEdits: make(map[string][]byte),
	}

	// Shared manifest bytes — lazily loaded, patched in-place across targets.
	manifestBytes := make(map[string][]byte)

	// Build source reference counts across ALL file documents (not just matched ones)
	// to detect shared templates that should not be overwritten.
	sourceRefCount := buildSourceRefCount(allFileDocs)

	// Determine resolver owner from first target.
	var resolverOwner string
	if len(targets) > 0 {
		resolverOwner = targets[0].Target.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)
	proc := repository.NewProcessor(runner, resolver, printer)

	ctx := context.Background()

	for _, tm := range targets {
		fullName := tm.Target.FullName()

		// Fetch current GitHub state.
		onStatus := func(s string) {
			tracker.UpdateStatus(fullName, s)
		}
		current, err := proc.FetchRepository(ctx, tm.Target.Owner, tm.Target.Name, onStatus)
		if err != nil {
			// Auth errors affect all targets — abort immediately.
			if errors.Is(err, gh.ErrUnauthorized) || errors.Is(err, gh.ErrForbidden) {
				tracker.Fail(fullName)
				return nil, fmt.Errorf("fetch %s: %w", fullName, err)
			}
			// Other errors (network, 404, etc.) — skip this target.
			printer.Warning(fullName, fmt.Sprintf("fetch failed: %v", err))
			tracker.Fail(fullName)
			continue
		}
		if current.IsNew {
			printer.Warning(fullName, "repository not found on GitHub")
			tracker.Fail(fullName)
			continue
		}

		// Convert to manifest.
		imported := repository.ToManifest(ctx, current, resolver)

		// Ensure manifest bytes are loaded for all relevant source paths.
		if err := ensureManifestBytes(manifestBytes, tm.Matches); err != nil {
			return nil, err
		}

		// Plan Repository matches.
		if len(tm.Matches.Repositories) > 0 {
			rp, err := DiffRepository(DiffInput{
				Repos:         tm.Matches.Repositories,
				Imported:      imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				return nil, fmt.Errorf("plan repository %s: %w", fullName, err)
			}
			plan.AddRepoResult(rp)
		}

		// Plan RepositorySet matches.
		if len(tm.Matches.RepositorySets) > 0 {
			rp, err := DiffRepositorySet(DiffInput{
				Repos:         tm.Matches.RepositorySets,
				Imported:      imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				return nil, fmt.Errorf("plan repositoryset %s: %w", fullName, err)
			}
			plan.AddRepoResult(rp)
		}

		// Plan FileSet matches.
		if len(tm.Matches.FileSets) > 0 {
			tracker.UpdateStatus(fullName, "comparing files...")
			fileChanges, err := DiffFiles(ctx, runner, tm.Matches.FileSets, fullName, sourceRefCount)
			if err != nil {
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
