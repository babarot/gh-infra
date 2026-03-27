package infra

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/fileset"
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

	totalSucceeded := 0
	totalFailed := 0

	var allRepoResults []repository.ApplyResult
	var allFileResults []fileset.ApplyResult

	// Apply repo changes
	if repository.HasChanges(result.RepoChanges) {
		var reporter ui.ProgressReporter
		if opts.Stream {
			reporter = ui.NewStreamReporter(p, "Applying", "Applied")
		} else {
			names := make([]string, 0)
			for _, c := range result.RepoChanges {
				if c.Type != repository.ChangeNoOp {
					names = append(names, c.Name)
				}
			}
			reporter = ui.NewSpinnerReporter(uniqueStrings(names), "Applying", "Applied", "(repo)")
		}
		allRepoResults = eng.repo.Apply(result.RepoChanges, result.TargetRepos, reporter)
		s, f := repository.CountApplyResults(allRepoResults)
		totalSucceeded += s
		totalFailed += f
	}

	// Apply file changes (per FileSet for correct options)
	if fileset.HasChanges(result.FileChanges) {
		for _, fs := range result.Parsed.FileSets {
			var fsChanges []fileset.Change
			for _, c := range result.FileChanges {
				if c.FileSetOwner == fs.Metadata.Owner {
					fsChanges = append(fsChanges, c)
				}
			}
			if !fileset.HasChanges(fsChanges) {
				continue
			}
			applyOpts := fileset.ApplyOptions{
				CommitMessage: fs.Spec.CommitMessage,
				Via:           fs.Spec.Via,
				Branch:        fs.Spec.Branch,
				FileSetOwner:  fs.Metadata.Owner,
				PRTitle:       fs.Spec.PRTitle,
				PRBody:        fs.Spec.PRBody,
			}
			var fileReporter ui.ProgressReporter
			if opts.Stream {
				fileReporter = ui.NewStreamReporter(p, "Applying", "Applied")
			} else {
				var targets []string
				for _, c := range fsChanges {
					targets = append(targets, c.Target)
				}
				fileReporter = ui.NewSpinnerReporter(uniqueStrings(targets), "Applying", "Applied", "(files)")
			}
			results := eng.file.Apply(fsChanges, applyOpts, fileReporter)
			allFileResults = append(allFileResults, results...)
			for _, r := range results {
				if r.Err != nil {
					totalFailed++
				} else {
					totalSucceeded++
				}
			}
		}
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
