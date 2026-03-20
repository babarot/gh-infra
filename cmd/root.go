package cmd

import (
	"os"

	"github.com/babarot/gh-infra/internal/logger"
	"github.com/spf13/cobra"
)

var (
	verbose  bool
	logLevel string
)

func NewRootCmd(version, revision string) *cobra.Command {
	root := &cobra.Command{
		Use:           "gh-infra",
		Short:         "Declarative GitHub infrastructure management",
		Long:          "Manage GitHub repository settings, branch protection, secrets, and more via YAML.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version + " (" + revision + ")",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Environment variable takes precedence if flag is not set
			level := logLevel
			if level == "" {
				level = os.Getenv(logger.EnvKey)
			}
			if level != "" {
				logger.Init(level)
			}
			// --verbose is a shorthand for debug level
			if verbose && level == "" {
				logger.Init("debug")
			}
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "V", false, "Show gh command execution details")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level (trace, debug, info, warn, error)")

	root.AddCommand(
		newPlanCmd(),
		newApplyCmd(),
		newImportCmd(),
		newValidateCmd(),
	)

	return root
}
