package infra

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// ApplyOptions configures the apply phase.
type ApplyOptions struct {
	Stream bool // true = stream output mode instead of spinner
}

// Apply executes planned changes against GitHub.
func Apply(result *PlanResult, opts ApplyOptions) error {
	eng := result.engine
	p := eng.printer

	ctx := context.Background()

	totalSucceeded := 0
	totalFailed := 0

	var allRepoResults []repository.ApplyResult
	var allFileResults []fileset.ApplyResult

	hasRepo := repository.HasChanges(result.RepoChanges)
	hasFile := fileset.HasChanges(result.FileChanges)

	if opts.Stream {
		// Stream mode: run sequentially with stream reporters (no spinner)
		if hasRepo {
			reporter := ui.NewStreamReporter(p, "Applying", "Applied")
			allRepoResults = eng.repo.Apply(ctx, result.RepoChanges, result.TargetRepos, reporter)
			s, f := repository.CountApplyResults(allRepoResults)
			totalSucceeded += s
			totalFailed += f
		}
		if hasFile {
			for _, fs := range result.Parsed.FileSets {
				fsChanges, applyOpts := fileSetApplyArgs(fs, result.FileChanges)
				if !fileset.HasChanges(fsChanges) {
					continue
				}
				reporter := ui.NewStreamReporter(p, "Applying", "Applied")
				results := eng.file.Apply(ctx, fsChanges, applyOpts, reporter)
				allFileResults = append(allFileResults, results...)
				s, f := countFileResults(results)
				totalSucceeded += s
				totalFailed += f
			}
		}
	} else {
		// Spinner mode: unified tracker, parallel execution
		// Collect repo names and build deduplicated tasks
		var repoNames, fileNames []string
		if hasRepo {
			for _, c := range result.RepoChanges {
				if c.Type != repository.ChangeNoOp {
					repoNames = append(repoNames, c.Name)
				}
			}
			repoNames = uniqueStrings(repoNames)
		}
		if hasFile {
			for _, c := range result.FileChanges {
				if c.Type != fileset.ChangeNoOp {
					fileNames = append(fileNames, c.Target)
				}
			}
			fileNames = uniqueStrings(fileNames)
		}

		taskMap := make(map[string]int)
		for _, n := range repoNames {
			taskMap[n]++
		}
		for _, n := range fileNames {
			taskMap[n]++
		}
		var allTasks []ui.RefreshTask
		seen := make(map[string]bool)
		for _, n := range append(repoNames, fileNames...) {
			if seen[n] {
				continue
			}
			seen[n] = true
			allTasks = append(allTasks, ui.RefreshTask{
				Name:    n,
				Pending: taskMap[n],
			})
		}
		tracker := ui.RunRefresh(allTasks)

		g := new(errgroup.Group)

		if hasRepo {
			g.Go(func() error {
				reporter := ui.NewSpinnerReporterWith(tracker, repoNames)
				allRepoResults = eng.repo.Apply(ctx, result.RepoChanges, result.TargetRepos, reporter)
				return nil
			})
		}

		if hasFile {
			g.Go(func() error {
				for _, fs := range result.Parsed.FileSets {
					fsChanges, applyOpts := fileSetApplyArgs(fs, result.FileChanges)
					if !fileset.HasChanges(fsChanges) {
						continue
					}
					var targets []string
					for _, c := range fsChanges {
						targets = append(targets, c.Target)
					}
					reporter := ui.NewSpinnerReporterWith(tracker, uniqueStrings(targets))
					results := eng.file.Apply(ctx, fsChanges, applyOpts, reporter)
					allFileResults = append(allFileResults, results...)
				}
				return nil
			})
		}

		_ = g.Wait()
		tracker.Wait()

		s, f := repository.CountApplyResults(allRepoResults)
		totalSucceeded += s
		totalFailed += f
		cs, cf := countFileResults(allFileResults)
		totalSucceeded += cs
		totalFailed += cf
	}

	// Print unified apply results (skip in stream mode — stream output is the result)
	if !opts.Stream {
		p.Separator()
		printApplyResults(p, allRepoResults, allFileResults)
	}

	// Unified summary
	summaryMsg := fmt.Sprintf("Apply complete! %d changes applied", totalSucceeded)
	if totalFailed > 0 {
		summaryMsg += fmt.Sprintf(", %d failed", totalFailed)
	}
	summaryMsg += "."
	p.Summary(summaryMsg)

	if totalFailed > 0 {
		return fmt.Errorf("apply had errors")
	}

	return nil
}

func fileSetApplyArgs(fs *manifest.FileSet, allChanges []fileset.Change) ([]fileset.Change, fileset.ApplyOptions) {
	var fsChanges []fileset.Change
	for _, c := range allChanges {
		if c.FileSetOwner == fs.Metadata.Owner {
			fsChanges = append(fsChanges, c)
		}
	}
	opts := fileset.ApplyOptions{
		CommitMessage: fs.Spec.CommitMessage,
		Via:           fs.Spec.Via,
		Branch:        fs.Spec.Branch,
		FileSetOwner:  fs.Metadata.Owner,
		PRTitle:       fs.Spec.PRTitle,
		PRBody:        fs.Spec.PRBody,
	}
	return fsChanges, opts
}

func countFileResults(results []fileset.ApplyResult) (succeeded, failed int) {
	for _, r := range results {
		if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	return
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
