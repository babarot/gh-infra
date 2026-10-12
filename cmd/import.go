package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/infra"
	"github.com/babarot/gh-infra/internal/ui"
)

func newImportCmd() *cobra.Command {
	var (
		intoPath    string
		autoApprove bool
	)

	cmd := &cobra.Command{
		Use:   "import <owner/repo> [owner/repo ...]",
		Short: "Export existing repository settings as YAML",
		Long: "Fetch current GitHub repository settings and output them as gh-infra YAML.\n" +
			"With --into, pull GitHub state back into existing local manifests.\n" +
			"When --into is used without specifying repositories, all repositories\n" +
			"defined in the manifests are targeted.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if autoApprove && intoPath == "" {
				return errors.New("--auto-approve requires --into")
			}
			if intoPath != "" {
				return runImportInto(cmd.Context(), args, intoPath, autoApprove)
			}
			if len(args) == 0 {
				return fmt.Errorf("requires at least 1 arg when --into is not used")
			}
			return runImport(cmd.Context(), args)
		},
	}

	cmd.Flags().StringVar(&intoPath, "into", "",
		"Pull GitHub state into existing local manifests (dir or file path)")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false,
		"With --into, write the changes without asking, using the default action for each file")

	return cmd
}

func runImport(ctx context.Context, args []string) error {
	runner, err := newRunner()
	if err != nil {
		return err
	}
	err = infra.Import(ctx, runner, args)
	if err != nil {
		if interrupted(ctx, err) {
			printCancelled()
			return nil
		}
		return err
	}
	return nil
}

func runImportInto(ctx context.Context, args []string, intoPath string, autoApprove bool) error {
	runner, err := newRunner()
	if err != nil {
		return err
	}
	diff, err := infra.ImportInto(ctx, runner, args, intoPath)
	if err != nil {
		if interrupted(ctx, err) {
			printCancelled()
			return nil
		}
		return err
	}

	if !diff.Matched {
		return nil
	}

	if !diff.HasChanges() {
		if diff.HasWarnings() {
			diff.Printer().Message("\nNo changes to write. See the warnings above.")
			return nil
		}
		if diff.Skipped > 0 {
			diff.Printer().Message("\nNo changes detected. Some repositories were skipped due to errors above.")
		} else {
			diff.Printer().Message("\nNo changes detected")
		}
		return nil
	}

	p := diff.Printer()

	ok, err := confirmImport(p, diff, autoApprove)
	if err != nil {
		if interrupted(ctx, err) {
			printCancelled()
			return nil
		}
		return err
	}
	if !ok {
		return nil
	}

	// Writing local files is not stopped midway: an interrupt during it is
	// reported once everything is written.
	written, err := diff.Write()
	if err != nil {
		return err
	}
	if !written.Wrote() {
		p.Message("\n" + written.Summary())
	} else {
		p.Summary(written.Summary())
	}
	if ctx.Err() != nil {
		printCancelled()
		return &ExitError{Code: 1}
	}
	return nil
}

// confirmImport asks before writing, applying the file actions chosen in the
// diff viewer. With autoApprove it asks nothing and uses each file's default
// action, as confirming the viewer without changes would.
func confirmImport(p ui.Printer, diff importDiff, autoApprove bool) (bool, error) {
	fileEntries := diff.DiffEntries()
	if autoApprove {
		diff.ApplySelections(fileEntries)
		return true, nil
	}

	var ok bool
	var err error
	if len(fileEntries) > 0 {
		ok, err = p.ConfirmWithDiff("Apply import changes?", fileEntries)
		if err == nil {
			diff.ApplySelections(fileEntries)
		}
	} else {
		ok, err = p.Confirm("Apply import changes?")
	}
	if err != nil {
		return false, fmt.Errorf("import --into needs a terminal to confirm; use --auto-approve to write without asking: %w", err)
	}
	return ok, nil
}

// importDiff is the part of *infra.ImportDiff that confirmImport uses.
type importDiff interface {
	DiffEntries() []ui.DiffEntry
	ApplySelections([]ui.DiffEntry)
}
