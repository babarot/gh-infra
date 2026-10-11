package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/logger"
	"github.com/babarot/gh-infra/internal/ui"
)

var (
	verbose     bool
	logLevel    string
	concurrency int
)

// concurrencyEnvKey sets --concurrency when the flag is not given.
const concurrencyEnvKey = "GH_INFRA_CONCURRENCY"

// newRunner returns the runner for one command run. Every gh call of the
// command goes through it, so --concurrency limits all of them together.
func newRunner() (*gh.GHRunner, error) {
	n := concurrency
	if n == 0 {
		if v := os.Getenv(concurrencyEnvKey); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 {
				return nil, fmt.Errorf("%s must be a positive integer, got %q", concurrencyEnvKey, v)
			}
			n = parsed
		}
	}
	if n < 0 {
		return nil, fmt.Errorf("--concurrency must be a positive integer, got %d", n)
	}
	return gh.NewRunnerWithConcurrency(false, n), nil
}

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
	root.PersistentFlags().IntVar(&concurrency, "concurrency", 0,
		fmt.Sprintf("Maximum number of GitHub API calls (gh processes) at once (default %d, or $%s)", gh.DefaultConcurrency, concurrencyEnvKey))

	root.AddCommand(
		newPlanCmd(),
		newApplyCmd(),
		newImportCmd(),
		newValidateCmd(),
	)

	return root
}

func printCancelled() {
	fmt.Fprintf(os.Stderr, "\n%s\n", ui.Dim.Render("Interrupted."))
}
