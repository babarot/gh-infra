package importer

import (
	"context"
	"fmt"
	"os"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// PlanInto builds a change plan for all targets.
// manifestBytes is shared across targets so patches accumulate correctly.
// Targets are processed sequentially (same file may be patched by multiple targets).
func PlanInto(targets []TargetMatches, runner gh.Runner, printer ui.Printer, tracker *ui.RefreshTracker) (*IntoPlan, error) {
	plan := &IntoPlan{
		ManifestEdits: make(map[string][]byte),
	}

	// Shared manifest bytes — lazily loaded, patched in-place across targets.
	manifestBytes := make(map[string][]byte)

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
			if tracker != nil {
				tracker.UpdateStatus(fullName, s)
			}
		}
		current, err := proc.FetchRepository(ctx, tm.Target.Owner, tm.Target.Name, onStatus)
		if err != nil {
			printer.Warning(fullName, fmt.Sprintf("fetch failed: %v", err))
			if tracker != nil {
				tracker.Fail(fullName)
			}
			continue
		}
		if current.IsNew {
			printer.Warning(fullName, "repository not found on GitHub")
			if tracker != nil {
				tracker.Fail(fullName)
			}
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
			rp, err := PlanRepository(RepoPlanInput{
				Repos:         tm.Matches.Repositories,
				Imported:      imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				return nil, fmt.Errorf("plan repository %s: %w", fullName, err)
			}
			plan.AddRepoPlan(rp)
		}

		// Plan RepositorySet matches.
		if len(tm.Matches.RepositorySets) > 0 {
			rp, err := PlanRepositorySet(RepoPlanInput{
				Repos:         tm.Matches.RepositorySets,
				Imported:      imported,
				ManifestBytes: manifestBytes,
			})
			if err != nil {
				return nil, fmt.Errorf("plan repositoryset %s: %w", fullName, err)
			}
			plan.AddRepoPlan(rp)
		}

		// TODO(phase3): PlanFiles for FileSet matches

		if tracker != nil {
			tracker.Done(fullName)
		}
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
