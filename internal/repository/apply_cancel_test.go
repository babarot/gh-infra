package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// cancelOnFirstRunner cancels the run on its first command and fails it as
// an interrupted gh process would; it records every command.
type cancelOnFirstRunner struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	calls  []string
}

func (r *cancelOnFirstRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(args, " "))
	first := len(r.calls) == 1
	r.mu.Unlock()
	if first {
		r.cancel()
	}
	return nil, errors.New("signal: killed")
}

func (r *cancelOnFirstRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func TestApply_StopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &cancelOnFirstRunner{cancel: cancel}
	p := NewProcessor(runner, nil)

	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec: manifest.RepositorySpec{Variables: []manifest.Variable{
			{Name: "A", Value: "1"}, {Name: "B", Value: "2"}, {Name: "C", Value: "3"},
		}},
	}
	var changes []Change
	for _, v := range repo.Spec.Variables {
		changes = append(changes, Change{
			Type: ChangeCreate, Resource: manifest.ResourceVariable, Name: "o/r", Field: v.Name, NewValue: v.Value,
		})
	}

	results := p.Apply(ctx, changes, []*manifest.Repository{repo}, nil)

	if len(runner.calls) != 1 {
		t.Errorf("gh calls = %q, want only the first", runner.calls)
	}
	if len(results) != 1 {
		t.Errorf("results = %d, want 1: changes after the cancel are left unapplied, not failed", len(results))
	}
}
