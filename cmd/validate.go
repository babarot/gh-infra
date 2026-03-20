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
	repos, err := manifest.ParsePath(path)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Valid: %d repositories defined\n", len(repos))
	for _, r := range repos {
		fmt.Printf("  - %s\n", r.Metadata.FullName())
	}
	return nil
}
