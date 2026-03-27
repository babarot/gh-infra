package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

func newPullCmd() *cobra.Command {
	var (
		repo   string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "pull [path]",
		Short: "Pull current GitHub file content back to local sources",
		Long:  "Fetch file content from a GitHub repository and write it back to local template sources or inline content blocks, bringing local state in sync with what is on GitHub.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runPull(path, repo, dryRun)
		},
	}

	cmd.Flags().StringVarP(&repo, "repo", "r", "", "Target specific repository only (owner/repo)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be written without making changes")

	return cmd
}

func runPull(path, filterRepo string, dryRun bool) error {
	p := ui.NewStandardPrinter()

	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	if len(parsed.FileSets) == 0 {
		p.Message("No FileSet resources found in " + path)
		return nil
	}

	runner := gh.NewRunner(false)
	processor := fileset.NewProcessor(runner, p)

	// Determine target repos for display
	var targetRepos []string
	for _, fs := range parsed.FileSets {
		for _, r := range fs.Spec.Repositories {
			fullName := fs.RepoFullName(r.Name)
			if filterRepo != "" && fullName != filterRepo {
				continue
			}
			targetRepos = append(targetRepos, fullName)
		}
	}

	if len(targetRepos) == 0 {
		p.Message("No matching repositories found")
		return nil
	}

	p.Phase(fmt.Sprintf("Fetching file content from %s ...", strings.Join(targetRepos, ", ")))
	p.BlankLine()

	changes, err := fileset.PlanPull(processor, parsed.FileSets, filterRepo)
	if err != nil {
		return err
	}

	// Display plan
	written, unchanged, skipped := fileset.PullSummary(changes)

	for _, c := range changes {
		switch c.Type {
		case fileset.PullWriteSource:
			relTarget := relativePath(c.LocalTarget)
			fmt.Fprintf(p.OutWriter(), "  %-40s → %-40s %s\n",
				c.Path, relTarget, ui.Green.Render("written"))
		case fileset.PullWriteInline:
			relManifest := relativePath(c.ManifestPath)
			fmt.Fprintf(p.OutWriter(), "  %-40s → %-40s %s\n",
				c.Path, relManifest+" (inline)", ui.Green.Render("written"))
		case fileset.PullNoOp:
			fmt.Fprintf(p.OutWriter(), "  %-40s   %-40s %s\n",
				c.Path, "", ui.Dim.Render("unchanged"))
		case fileset.PullSkip:
			fmt.Fprintf(p.OutWriter(), "  %-40s   %-40s %s\n",
				c.Path, "", ui.Yellow.Render("skipped ("+c.Reason+")"))
		}
	}

	// Print warnings
	for _, c := range changes {
		for _, w := range c.Warnings {
			fmt.Fprintf(p.ErrWriter(), "\n  %s %s: %s", ui.Yellow.Render("⚠"), c.Path, w)
		}
	}

	fmt.Fprintln(p.OutWriter())

	if dryRun {
		p.Summary(fmt.Sprintf("Dry run: %s to write, %s unchanged, %s to skip.",
			ui.Bold.Render(fmt.Sprintf("%d", written)),
			ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
			ui.Bold.Render(fmt.Sprintf("%d", skipped)),
		))
		return nil
	}

	if written == 0 {
		p.Message("Nothing to pull. Local files are up-to-date.")
		return nil
	}

	// Read manifest bytes for inline edits
	manifestBytes := make(map[string][]byte)
	for _, fs := range parsed.FileSets {
		sp := fs.SourcePath()
		if _, ok := manifestBytes[sp]; !ok {
			data, err := os.ReadFile(sp)
			if err != nil {
				return fmt.Errorf("read manifest %s: %w", sp, err)
			}
			manifestBytes[sp] = data
		}
	}

	if err := fileset.ApplyPull(changes, manifestBytes); err != nil {
		return err
	}

	p.Summary(fmt.Sprintf("Pull complete! %s written, %s unchanged, %s skipped.",
		ui.Bold.Render(fmt.Sprintf("%d", written)),
		ui.Bold.Render(fmt.Sprintf("%d", unchanged)),
		ui.Bold.Render(fmt.Sprintf("%d", skipped)),
	))

	return nil
}

// relativePath attempts to make a path relative to the current working directory.
func relativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
