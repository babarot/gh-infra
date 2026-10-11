package infra

import (
	"context"
	"fmt"
	"strings"

	goyaml "github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// Import fetches current state of the given repositories and outputs them as YAML to stdout.
// All display is handled internally. runner runs every gh call.
func Import(runner gh.Runner, args []string) error {
	targets, err := parseArgs(args)
	if err != nil {
		return err
	}

	p := ui.NewStandardPrinter()

	resolver := manifest.NewResolver(runner, targets[0].Target.Owner)
	eng := newEngine(runner, resolver, p)

	label := "repository"
	if len(targets) != 1 {
		label = "repositories"
	}
	p.Phase(fmt.Sprintf("Fetching current state of %d %s from GitHub API ...", len(targets), label))
	p.BlankLine()

	// Start spinner display
	names := make([]string, len(targets))
	tasks := make([]ui.RefreshTask, len(targets))
	for i, t := range targets {
		names[i] = t.Target.FullName()
		tasks[i] = ui.RefreshTask{
			Name:      names[i],
			FailLabel: names[i],
		}
	}
	tracker := ui.RunRefresh(tasks)

	ctx, cancel := withTrackerCancelContext(tracker)
	defer cancel()

	type fetchResult struct {
		data []byte
		err  error
	}
	results := parallel.Map(ctx, targets, parallel.DefaultConcurrency, func(ctx context.Context, _ int, t importer.TargetMatches) fetchResult {
		fullName := t.Target.FullName()
		onStatus := func(s string) {
			tracker.UpdateStatus(fullName, s)
		}
		current, err := eng.repo.FetchRepository(ctx, t.Target.Owner, t.Target.Name, onStatus)
		if err != nil {
			tracker.Error(fullName, fmt.Errorf("fetch failed: %w", err))
			return fetchResult{err: err}
		}
		if current.IsNew {
			tracker.Error(fullName, fmt.Errorf("repository not found on GitHub"))
			return fetchResult{err: fmt.Errorf("repository %s not found on GitHub", fullName)}
		}
		m := repository.ToManifest(ctx, current, resolver)
		data, err := goyaml.Marshal(m)
		if err != nil {
			tracker.Error(fullName, fmt.Errorf("marshal: %w", err))
		} else {
			tracker.Done(fullName)
		}
		return fetchResult{data: data, err: err}
	})
	tracker.Wait()
	tracker.PrintErrors()

	if ctx.Err() != nil {
		return context.Canceled
	}

	// Collect YAML docs for successful targets; failures are already reported
	// by tracker.PrintErrors above.
	var yamlDocs [][]byte
	var succeeded, failed int
	for _, r := range results {
		if r.err != nil {
			failed++
		} else {
			yamlDocs = append(yamlDocs, r.data)
			succeeded++
		}
	}

	p.Separator()

	out := p.OutWriter()
	for i, doc := range yamlDocs {
		if i > 0 {
			fmt.Fprintln(out, "---")
		}
		fmt.Fprint(out, string(doc))
	}

	summaryMsg := fmt.Sprintf("Import complete! %s exported", ui.Bold.Render(fmt.Sprintf("%d", succeeded)))
	if failed > 0 {
		summaryMsg += fmt.Sprintf(", %s failed", ui.Bold.Render(fmt.Sprintf("%d", failed)))
	}
	summaryMsg += "."
	p.Summary(summaryMsg)

	return nil
}

// parseArgs parses owner/repo arguments into targets.
func parseArgs(args []string) ([]importer.TargetMatches, error) {
	var targets []importer.TargetMatches
	for _, arg := range args {
		parts := strings.SplitN(arg, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target: %q (expected owner/repo)", arg)
		}
		targets = append(targets, importer.TargetMatches{
			Target: importer.Target{Owner: parts[0], Name: parts[1]},
		})
	}
	return targets, nil
}
