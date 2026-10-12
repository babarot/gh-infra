package repository

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// failRunner fails the commands starting with one of fail, with msg, and
// records every command.
type failRunner struct {
	mu    sync.Mutex
	calls []string
	fail  []string
	msg   string
}

func (r *failRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, cmd)
	r.mu.Unlock()
	for _, f := range r.fail {
		if strings.HasPrefix(cmd, f) {
			return nil, errors.New(r.msg)
		}
	}
	return nil, nil
}

func (r *failRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func (r *failRunner) ran(prefix string) bool {
	return slices.ContainsFunc(r.calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func newRepoChanges() (*manifest.Repository, []Change) {
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec: manifest.RepositorySpec{
			Topics:    []string{"go"},
			Labels:    []manifest.Label{{Name: "bug", Color: "ff0000"}},
			Variables: []manifest.Variable{{Name: "ENV", Value: "dev"}},
		},
	}
	return repo, []Change{
		{Type: ChangeCreate, Resource: manifest.ResourceRepository, Name: "o/r", Field: "repository", NewValue: "o/r"},
		{Type: ChangeCreate, Resource: manifest.ResourceLabel, Name: "o/r", Field: "bug"},
		{Type: ChangeCreate, Resource: manifest.ResourceVariable, Name: "o/r", Field: "ENV", NewValue: "dev"},
	}
}

// When gh repo create fails, nothing else can be applied, so the rest is
// left out with a result that says why.
func TestApply_NothingAfterAFailedCreate(t *testing.T) {
	repo, changes := newRepoChanges()
	runner := &failRunner{fail: []string{"repo create"}, msg: "boom"}
	results := NewProcessor(runner, nil).Apply(context.Background(), changes, []*manifest.Repository{repo}, nil)

	if len(runner.calls) != 1 {
		t.Errorf("gh calls = %q, want only the create", runner.calls)
	}
	if len(results) != 3 || !errors.Is(results[0].Err, errRepoNotCreated) {
		t.Fatalf("results = %+v", results)
	}
	for _, r := range results[1:] {
		if !errors.Is(r.Err, errNotApplied) {
			t.Errorf("%s: err = %v, want %v", r.Change.Field, r.Err, errNotApplied)
		}
	}
}

// When the repository was created but a setting after it failed, the rest
// still applies to the repository that exists.
func TestApply_RestAfterAFailedSetting(t *testing.T) {
	repo, changes := newRepoChanges()
	runner := &failRunner{fail: []string{"repo edit o/r --add-topic"}, msg: "boom"}
	results := NewProcessor(runner, nil).Apply(context.Background(), changes, []*manifest.Repository{repo}, nil)

	if results[0].Err == nil || errors.Is(results[0].Err, errRepoNotCreated) {
		t.Errorf("create result = %v, want the setting's failure", results[0].Err)
	}
	if !runner.ran("label create bug") || !runner.ran("variable set ENV") {
		t.Errorf("the rest did not run: %q", runner.calls)
	}
}

// A label that GitHub created with the repository is updated instead.
func TestApply_DefaultLabelOnANewRepositoryIsUpdated(t *testing.T) {
	repo, changes := newRepoChanges()
	runner := &failRunner{fail: []string{"label create bug"}, msg: `label with name "bug" already exists; use --force to update its color and description`}
	results := NewProcessor(runner, nil).Apply(context.Background(), changes, []*manifest.Repository{repo}, nil)

	for _, r := range results {
		if r.Err != nil {
			t.Errorf("%s: %v", r.Change.Field, r.Err)
		}
	}
	if !runner.ran("api repos/o/r/labels/bug --method PATCH") {
		t.Errorf("the label was not updated: %q", runner.calls)
	}
}

// For a repository that existed before the run, the plan saw no such label,
// so "already exists" is reported, not papered over.
func TestApply_ExistingLabelOnAnOldRepositoryFails(t *testing.T) {
	repo, changes := newRepoChanges()
	runner := &failRunner{fail: []string{"label create bug"}, msg: `label with name "bug" already exists`}
	results := NewProcessor(runner, nil).Apply(context.Background(), changes[1:2], []*manifest.Repository{repo}, nil)

	if len(results) != 1 || results[0].Err == nil {
		t.Errorf("results = %+v, want the create to fail", results)
	}
	if runner.ran("api repos/o/r/labels/bug") {
		t.Errorf("updated a label the plan did not see: %q", runner.calls)
	}
}
