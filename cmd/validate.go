package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

func newValidateCmd() *cobra.Command {
	var failOnUnknown bool

	cmd := &cobra.Command{
		Use:   "validate [path...]",
		Short: "Validate YAML syntax and schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(args, failOnUnknown)
		},
	}

	cmd.Flags().BoolVar(&failOnUnknown, "fail-on-unknown", false, "Error on YAML files with unknown Kind")

	return cmd
}

func runValidate(args []string, failOnUnknown bool) error {
	paths, err := manifest.ResolvePaths(args)
	if err != nil {
		return err
	}

	p := ui.NewStandardPrinter()

	runner := gh.NewRunner(false)
	sourceResolver := manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
		return runner.Run(ctx, args...)
	})

	parsed := &manifest.ParseResult{}
	for _, path := range paths {
		result, err := manifest.ParseAll(path, manifest.ParseOptions{
			FailOnUnknown: failOnUnknown,
			Resolver:      sourceResolver,
		})
		if err != nil {
			return err
		}
		parsed.Merge(result)
	}

	// Print deprecation warnings
	for _, w := range parsed.Warnings {
		p.Warning("deprecation", w)
	}

	p.Success("Valid", fmt.Sprintf("%d repositories, %d filesets defined", len(parsed.Repositories), len(parsed.FileSets)))
	for _, r := range parsed.Repositories {
		p.Message("  - Repository: " + r.Metadata.FullName())
	}
	for _, fs := range parsed.FileSets {
		p.Message(fmt.Sprintf("  - FileSet: %s (%d files → %d repositories)", fs.Metadata.Owner, len(fs.Spec.Files), len(fs.Spec.Repositories)))
	}
	return nil
}
