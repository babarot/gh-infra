package infra

import (
	"context"
	"fmt"

	goyaml "github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

const defaultImportParallel = 5

// ImportTarget specifies a repository to import.
type ImportTarget struct {
	Owner string
	Name  string
}

// FullName returns "owner/name".
func (t ImportTarget) FullName() string {
	return t.Owner + "/" + t.Name
}

// ImportResult holds the outcome of the import phase.
type ImportResult struct {
	// YAMLDocs contains the marshaled YAML for each successfully imported target, in order.
	YAMLDocs [][]byte
	// Errors maps target full name to its error (nil if successful).
	Errors map[string]error

	Succeeded int
	Failed    int

	engine *engine
}

// Printer returns the printer used during this import session.
func (r *ImportResult) Printer() ui.Printer {
	if r.engine == nil {
		return ui.NewStandardPrinter()
	}
	return r.engine.printer
}

// Import fetches current state of the given repositories and converts them to YAML manifests.
func Import(targets []ImportTarget) (*ImportResult, error) {
	p := ui.NewStandardPrinter()

	runner := gh.NewRunner(false)
	resolver := manifest.NewResolver(runner, targets[0].Owner)
	eng := newEngine(runner, resolver, p)

	label := "repository"
	if len(targets) != 1 {
		label = "repositories"
	}
	p.Phase(fmt.Sprintf("Importing %d %s from GitHub API ...", len(targets), label))
	p.BlankLine()

	// Start spinner display
	names := make([]string, len(targets))
	tasks := make([]ui.RefreshTask, len(targets))
	for i, t := range targets {
		names[i] = t.FullName()
		tasks[i] = ui.RefreshTask{
			Name:      "Importing " + names[i],
			DoneLabel: "Imported " + names[i],
			FailLabel: "Failed " + names[i],
		}
	}
	tracker := ui.RunRefresh(tasks)

	// Create a cancellable context; cancel when the spinner is interrupted via Ctrl+C.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ch := tracker.Cancelled()
		if ch == nil {
			return
		}
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Fetch all repos in parallel
	type fetchResult struct {
		data []byte
		err  error
	}
	results := parallel.Map(ctx, targets, defaultImportParallel, func(ctx context.Context, _ int, t ImportTarget) fetchResult {
		fullName := t.FullName()
		key := "Importing " + fullName
		current, err := eng.repo.FetchRepository(ctx, t.Owner, t.Name)
		if err != nil {
			tracker.Fail(key)
			return fetchResult{err: err}
		}
		if current.IsNew {
			tracker.Fail(key)
			return fetchResult{err: fmt.Errorf("repository %s not found on GitHub", fullName)}
		}
		m := repository.ToManifest(ctx, current, resolver)
		data, err := goyaml.Marshal(m)
		if err != nil {
			tracker.Fail(key)
		} else {
			tracker.Done(key)
		}
		return fetchResult{data: data, err: err}
	})
	tracker.Wait()

	if ctx.Err() != nil {
		return nil, context.Canceled
	}

	// Collect results
	importResult := &ImportResult{
		Errors: make(map[string]error),
		engine: eng,
	}
	for i, r := range results {
		fullName := names[i]
		if r.err != nil {
			importResult.Errors[fullName] = r.err
			importResult.Failed++
		} else {
			importResult.YAMLDocs = append(importResult.YAMLDocs, r.data)
			importResult.Succeeded++
		}
	}

	return importResult, nil
}
