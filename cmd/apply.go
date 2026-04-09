package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/infra"
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
		Use:   "apply [path...]",
		Short: "Apply desired state to GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(args, applyCommandOptions{
				FilterRepo:    repo,
				AutoApprove:   autoApprove,
				ForceSecrets:  forceSecrets,
				FailOnUnknown: failOnUnknown,
			})
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&forceSecrets, "force-secrets", false, "Always re-set all secrets (even if they already exist)")
	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")

	return cmd
}

type applyCommandOptions struct {
	FilterRepo    string
	AutoApprove   bool
	ForceSecrets  bool
	FailOnUnknown bool
}

func runApply(paths []string, opts applyCommandOptions) error {
	result, err := infra.Plan(infra.PlanOptions{
		Paths:         paths,
		FilterRepo:    opts.FilterRepo,
		FailOnUnknown: opts.FailOnUnknown,
		ForceSecrets:  opts.ForceSecrets,
		DryRun:        false,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			printCancelled()
			return nil
		}
		return err
	}

	if !result.HasChanges {
		return nil
	}

	p := result.Printer()

	// Confirm
	if !opts.AutoApprove {
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

	return infra.Apply(result, infra.ApplyOptions{
		Stream: ui.OutputMode() == "stream",
	})
}

// applySkipSelections writes skip selections from the diff viewer back
// to fileChanges, setting skipped entries to ChangeNoOp so they are not applied.
func applySkipSelections(changes []fileset.Change, entries []ui.DiffEntry) {
	type key struct{ target, path string }
	skipped := make(map[key]bool, len(entries))
	for _, e := range entries {
		if e.Action == "skip" {
			skipped[key{e.Target, e.Path}] = true
		}
	}
	for i := range changes {
		if skipped[key{changes[i].Target, changes[i].Path}] {
			changes[i].Type = fileset.ChangeNoOp
		}
	}
}

func buildDiffEntries(changes []fileset.Change) []ui.DiffEntry {
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
			Path:           c.Path,
			Target:         c.Target,
			Icon:           icon,
			Current:        c.Current,
			Desired:        c.Desired,
			Action:         "apply",
			DefaultAction:  "apply",
			AllowedActions: []string{"apply", "skip"},
		})
	}
	return entries
}
