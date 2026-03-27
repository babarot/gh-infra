package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
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
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(path, manifest.ParseOptions{FailOnUnknown: failOnUnknown})
	if err != nil {
		return err
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("deprecation", w)
	}

	if len(parsed.Repositories) == 0 && len(parsed.FileSets) == 0 {
		p.Message("No resources found in " + path)
		return nil
	}

	manifest.ResolveSecrets(parsed.Repositories)

	runner := gh.NewRunner(false)

	// Determine owner for resolver (use first repo's owner)
	var resolverOwner string
	if len(parsed.Repositories) > 0 {
		resolverOwner = parsed.Repositories[0].Metadata.Owner
	}
	resolver := manifest.NewResolver(runner, resolverOwner)

	p.Phase(fmt.Sprintf("Reading desired state from %s ...", path))
	p.Phase("Fetching current state from GitHub API ...")
	p.BlankLine()

	// Compute all changes in parallel
	var repoChanges []repository.Change
	var targetRepos []*manifest.Repository
	var fileChanges []fileset.FileApplyChange

	// Collect all target names and start a single spinner display
	var allTasks []ui.RefreshTask
	allTasks = append(allTasks, repository.FetchTargetNames(parsed.Repositories, filterRepo)...)
	allTasks = append(allTasks, fileset.PlanTargetNames(parsed.FileSetDocs, filterRepo)...)
	tracker := ui.RunRefresh(allTasks)

	g := new(errgroup.Group)

	if len(parsed.Repositories) > 0 {
		fetcher := repository.NewFetcher(runner)
		diffOpts := repository.DiffOptions{ForceSecrets: forceSecrets, Resolver: resolver}
		g.Go(func() error {
			var fetchErr error
			repoChanges, targetRepos, fetchErr = repository.FetchAllChanges(parsed.Repositories, filterRepo, fetcher, p, tracker, diffOpts)
			return fetchErr
		})
	}

	if len(parsed.FileSetDocs) > 0 {
		processor := fileset.NewProcessor(runner, p)
		g.Go(func() error {
			var planErr error
			fileChanges, planErr = processor.Plan(parsed.FileSetDocs, filterRepo, tracker)
			return planErr
		})
	}

	if err := g.Wait(); err != nil {
		tracker.Wait()
		return err
	}
	tracker.Wait()

	hasRepo := repository.HasRealChanges(repoChanges)
	hasFile := fileset.HasChanges(fileChanges)

	if !hasRepo && !hasFile {
		p.Message("\nNo changes. Infrastructure is up-to-date.")
		return nil
	}

	// Print unified plan
	repoCreates, repoUpdates, repoDeletes := repository.CountChanges(repoChanges)
	fileCreates, fileUpdates, fileDeletes := fileset.CountChanges(fileChanges)
	creates := repoCreates + fileCreates
	updates := repoUpdates + fileUpdates
	deletes := repoDeletes + fileDeletes

	p.Separator()
	p.Legend(creates > 0, updates > 0, deletes > 0)

	printUnifiedPlan(p, repoChanges, fileChanges)
	parts := []string{
		fmt.Sprintf("%s to create", ui.Bold.Render(fmt.Sprintf("%d", creates))),
		fmt.Sprintf("%s to update", ui.Bold.Render(fmt.Sprintf("%d", updates))),
		fmt.Sprintf("%s to destroy", ui.Bold.Render(fmt.Sprintf("%d", deletes))),
	}
	p.Summary(fmt.Sprintf("Plan: %s\nTo apply, run: %s", strings.Join(parts, ", "), ui.Bold.Render("gh infra apply")))

	// Confirm
	if !autoApprove {
		diffEntries := buildDiffEntries(fileChanges)
		confirmed, err := p.ConfirmWithDiff("Do you want to apply these changes?", diffEntries)
		if err != nil {
			return err
		}
		if !confirmed {
			p.Message("Apply canceled.")
			return nil
		}
		// Apply skip selections from the diff viewer back to fileChanges
		applySkipSelections(fileChanges, diffEntries)
	}

	totalSucceeded := 0
	totalFailed := 0

	stream := ui.OutputMode() == "stream"

	var allRepoResults []repository.ApplyResult
	var allFileResults []fileset.FileApplyResult

	// Apply repo changes
	if hasRepo {
		executor := repository.NewExecutor(runner, resolver)
		var reporter ui.ProgressReporter
		if stream {
			reporter = ui.NewStreamReporter(p, "Applying", "Applied")
		} else {
			names := make([]string, 0)
			for _, c := range repoChanges {
				if c.Type != repository.ChangeNoOp {
					names = append(names, c.Name)
				}
			}
			reporter = ui.NewSpinnerReporter(uniqueStrings(names), "Applying", "Applied", "(repo)")
		}
		allRepoResults = executor.Apply(repoChanges, targetRepos, reporter)
		s, f := repository.CountApplyResults(allRepoResults)
		totalSucceeded += s
		totalFailed += f
	}

	// Apply file changes (per FileSet for correct options)
	if hasFile {
		processor := fileset.NewProcessor(runner, p)
		for _, fs := range parsed.FileSetDocs {
			var fsChanges []fileset.FileApplyChange
			for _, c := range fileChanges {
				if c.FileSet == fs.Resource.Metadata.Owner {
					fsChanges = append(fsChanges, c)
				}
			}
			if !fileset.HasChanges(fsChanges) {
				continue
			}
			opts := fileset.ApplyOptions{
				CommitMessage: fs.Resource.Spec.CommitMessage,
				Via:           fs.Resource.Spec.Via,
				Branch:        fs.Resource.Spec.Branch,
				FileSetName:   fs.Resource.Metadata.Owner,
				PRTitle:       fs.Resource.Spec.PRTitle,
				PRBody:        fs.Resource.Spec.PRBody,
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
		printUnifiedApplyResults(p, allRepoResults, allFileResults)
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
// to fileChanges, setting skipped entries to FileNoOp so they are not applied.
func applySkipSelections(changes []fileset.FileApplyChange, entries []ui.DiffEntry) {
	// Build a set of target+path keys that were marked as skipped
	type key struct{ target, path string }
	skipped := make(map[key]bool, len(entries))
	for _, e := range entries {
		if e.Skip {
			skipped[key{e.Target, e.Path}] = true
		}
	}
	for i := range changes {
		if skipped[key{changes[i].Target, changes[i].Path}] {
			changes[i].Type = fileset.FileNoOp
		}
	}
}

func buildDiffEntries(changes []fileset.FileApplyChange) []ui.DiffEntry {
	var entries []ui.DiffEntry
	for _, c := range changes {
		var icon string
		switch c.Type {
		case fileset.FileCreate:
			icon = ui.IconAdd
		case fileset.FileUpdate:
			icon = ui.IconChange
		case fileset.FileDelete:
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
