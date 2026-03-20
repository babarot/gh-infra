package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/state"
	goyaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <owner/repo | owner/>",
		Short: "Export existing repository settings as YAML",
		Long:  "Fetch current GitHub repository settings and output them as gh-infra YAML.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(args[0])
		},
	}
	return cmd
}

func runImport(target string) error {
	runner := gh.NewRunner(false)
	fetcher := state.NewFetcher(runner)

	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid target: %q (expected owner/repo or owner/)", target)
	}

	owner := parts[0]
	name := parts[1]

	if name == "" {
		return importAllRepos(owner, runner, fetcher)
	}

	return importSingleRepo(owner, name, fetcher)
}

func importSingleRepo(owner, name string, fetcher *state.Fetcher) error {
	current, err := fetcher.FetchRepository(owner, name)
	if err != nil {
		return err
	}

	m := state.ToManifest(current)
	data, err := goyaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	fmt.Fprint(os.Stdout, string(data))
	return nil
}

func importAllRepos(owner string, runner gh.Runner, fetcher *state.Fetcher) error {
	out, err := runner.Run("repo", "list", owner, "--json", "name", "--limit", manifest.DefaultMaxRepoList)
	if err != nil {
		return fmt.Errorf("list repos for %s: %w", owner, err)
	}

	var repos []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &repos); err != nil {
		return fmt.Errorf("parse repo list: %w", err)
	}

	for i, r := range repos {
		if i > 0 {
			fmt.Println("---")
		}
		fmt.Fprintf(os.Stderr, "Importing %s/%s ...\n", owner, r.Name)
		if err := importSingleRepo(owner, r.Name, fetcher); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ skipping %s/%s: %v\n", owner, r.Name, err)
		}
	}
	return nil
}
