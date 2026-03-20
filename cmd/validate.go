package cmd

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate YAML syntax and schema",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runValidate(path)
		},
	}
	return cmd
}

func runValidate(path string) error {
	parsed, err := manifest.ParseAll(path)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Valid: %d repositories, %d filesets defined\n",
		len(parsed.Repositories), len(parsed.FileSets))
	for _, r := range parsed.Repositories {
		fmt.Printf("  - Repository: %s\n", r.Metadata.FullName())
	}
	for _, fs := range parsed.FileSets {
		fmt.Printf("  - FileSet: %s (%d files → %d targets)\n",
			fs.Metadata.Name, len(fs.Spec.Files), len(fs.Spec.Targets))
	}
	return nil
}
