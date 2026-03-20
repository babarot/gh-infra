package cmd

import (
	"github.com/spf13/cobra"
)

var (
	verbose bool
)

func NewRootCmd(version, revision string) *cobra.Command {
	root := &cobra.Command{
		Use:   "gh-infra",
		Short: "Declarative GitHub infrastructure management",
		Long:  "Manage GitHub repository settings, branch protection, secrets, and more via YAML.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version + " (" + revision + ")",
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Show gh command execution details")

	root.AddCommand(
		newPlanCmd(),
		newApplyCmd(),
		newImportCmd(),
		newValidateCmd(),
	)

	return root
}
