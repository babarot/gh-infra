package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/plan"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func newApplyCmd() *cobra.Command {
	var (
		repo          string
		autoApprove   bool
		forceSecrets  bool
		failOnUnknown bool
	)

	cmd := &cobra.Command{
		Use:   "apply [path]",
		Short: "Apply desired state to GitHub",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runApply(path, repo, autoApprove, forceSecrets, failOnUnknown)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&forceSecrets, "force-secrets", false, "Always re-set all secrets (even if they already exist)")
	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")

	return cmd
}

func runApply(path, filterRepo string, autoApprove, forceSecrets, failOnUnknown bool) error {
	result, err := plan.Run(plan.Options{
		Path:          path,
		FilterRepo:    filterRepo,
		FailOnUnknown: failOnUnknown,
		ForceSecrets:  forceSecrets,
		DryRun:        false,
	})
	if err != nil {
		return err
	}

	if !result.HasChanges {
		return nil
	}

	p := result.Printer

	// Confirm
	if !autoApprove {
		diffEntries := buildDiffEntries(result.FileChanges)
		confirmed, err := p.ConfirmWithDiff("Do you want to apply these changes?", diffEntries)
		if err != nil {
			return err
		}
		if !confirmed {
			p.Message("Apply canceled.")
			return nil
		}
		applySkipSelections(result.FileChanges, diffEntries)
	}

	runner := result.Runner
	resolver := result.Resolver

	totalSucceeded := 0
	totalFailed := 0

	stream := ui.OutputMode() == "stream"

	var allRepoResults []repository.ApplyResult
	var allFileResults []fileset.FileApplyResult

	// Apply repo changes
	if repository.HasChanges(result.RepoChanges) {
		executor := repository.NewExecutor(runner, resolver)
		var reporter ui.ProgressReporter
		if stream {
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
		allRepoResults = executor.Apply(result.RepoChanges, result.TargetRepos, reporter)
		s, f := repository.CountApplyResults(allRepoResults)
		totalSucceeded += s
		totalFailed += f
	}

	// Apply file changes (per FileSet for correct options)
	if fileset.HasChanges(result.FileChanges) {
		processor := fileset.NewProcessor(runner, p)
		for _, fs := range result.Parsed.FileSets {
			var fsChanges []fileset.FileChange
			for _, c := range result.FileChanges {
				if c.FileSetOwner == fs.Metadata.Owner {
					fsChanges = append(fsChanges, c)
				}
			}
			if !fileset.HasChanges(fsChanges) {
				continue
			}
			opts := fileset.ApplyOptions{
				CommitMessage: fs.Spec.CommitMessage,
				Via:           fs.Spec.Via,
				Branch:        fs.Spec.Branch,
				FileSetName:   fs.Metadata.Owner,
				PRTitle:       fs.Spec.PRTitle,
				PRBody:        fs.Spec.PRBody,
			}
			var fileReporter ui.ProgressReporter
			if stream {
				fileReporter = ui.NewStreamReporter(p, "Applying", "Applied")
			} else {
				var targets []string
				for _, c := range fsChanges {
					targets = append(targets, c.Target)
				}
				fileReporter = ui.NewSpinnerReporter(uniqueStrings(targets), "Applying", "Applied", "(files)")
			}
			results := processor.Apply(fsChanges, opts, fileReporter)
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
	if !stream {
		p.Separator()
		plan.PrintApplyResults(p, allRepoResults, allFileResults)
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

// applySkipSelections writes skip selections from the diff viewer back
// to fileChanges, setting skipped entries to ChangeNoOp so they are not applied.
func applySkipSelections(changes []fileset.FileChange, entries []ui.DiffEntry) {
	type key struct{ target, path string }
	skipped := make(map[key]bool, len(entries))
	for _, e := range entries {
		if e.Skip {
			skipped[key{e.Target, e.Path}] = true
		}
	}
	for i := range changes {
		if skipped[key{changes[i].Target, changes[i].Path}] {
			changes[i].Type = fileset.ChangeNoOp
		}
	}
}

func buildDiffEntries(changes []fileset.FileChange) []ui.DiffEntry {
	var entries []ui.DiffEntry
	for _, c := range changes {
		var icon string
		switch c.Type {
		case fileset.ChangeCreate:
			icon = ui.IconAdd
		case fileset.ChangeUpdate:
			icon = ui.IconChange
		case fileset.ChangeDelete:
			icon = ui.IconRemove
		default:
			continue
		}
		entries = append(entries, ui.DiffEntry{
			Path:    c.Path,
			Target:  c.Target,
			Icon:    icon,
			Current: c.Current,
			Desired: c.Desired,
		})
	}
	return entries
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
