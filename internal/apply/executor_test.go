package apply

import (
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/plan"
)

func newTestRepo(owner, name string) *manifest.Repository {
	return &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{
			Owner: owner,
			Name:  name,
		},
	}
}

func TestApplyRepoDescription(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			OldValue: "old desc",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--description", "new desc"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyHomepage(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "homepage",
			NewValue: "https://example.com",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--homepage", "https://example.com"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyVisibility(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "visibility",
			NewValue: "private",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--visibility", "private"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyTopics(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"repo view myorg/myrepo --json repositoryTopics --jq .[].name": []byte("old-topic\nkeep-topic\n"),
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Topics = []string{"keep-topic", "new-topic"}

	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "topics",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}

	// Expect: view call, remove old-topic, add new-topic
	// (keep-topic should not be touched)
	var removeCalls, addCalls []string
	for _, call := range mock.Called {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "--remove-topic") {
			removeCalls = append(removeCalls, joined)
		}
		if strings.Contains(joined, "--add-topic") {
			addCalls = append(addCalls, joined)
		}
	}

	if len(removeCalls) != 1 {
		t.Fatalf("expected 1 remove-topic call, got %d: %v", len(removeCalls), removeCalls)
	}
	if !strings.Contains(removeCalls[0], "old-topic") {
		t.Errorf("expected remove old-topic, got %s", removeCalls[0])
	}

	if len(addCalls) != 1 {
		t.Fatalf("expected 1 add-topic call, got %d: %v", len(addCalls), addCalls)
	}
	if !strings.Contains(addCalls[0], "new-topic") {
		t.Errorf("expected add new-topic, got %s", addCalls[0])
	}
}

func TestApplyFeatureToggle(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "wiki",
			NewValue: false,
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"repo", "edit", "myorg/myrepo", "--enable-wiki=false"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplyWithErrNotFound(t *testing.T) {
	notFoundErr := fmt.Errorf("%w: %w", gh.ErrNotFound, &gh.ExitError{
		Cmd: "repo edit myorg/myrepo", ExitCode: 1,
		APIError: &gh.APIError{Status: 404, Message: "Not Found"},
	})

	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo edit myorg/myrepo --description new desc": notFoundErr,
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("expected user-friendly not found message, got %q", errMsg)
	}
}

func TestApplyWithErrForbidden(t *testing.T) {
	forbiddenErr := fmt.Errorf("%w: %w", gh.ErrForbidden, &gh.ExitError{
		Cmd: "repo edit myorg/myrepo", ExitCode: 1,
		APIError: &gh.APIError{Status: 403, Message: "Forbidden"},
	})

	mock := &gh.MockRunner{
		Errors: map[string]error{
			"repo edit myorg/myrepo --description new desc": forbiddenErr,
		},
	}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeUpdate,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
			NewValue: "new desc",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err == nil {
		t.Fatal("expected error, got nil")
	}
	errMsg := results[0].Err.Error()
	if !strings.Contains(errMsg, "no permission") {
		t.Errorf("expected user-friendly forbidden message, got %q", errMsg)
	}
}

func TestApplyVariableSet(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Variables = []manifest.Variable{
		{Name: "MY_VAR", Value: "my-value"},
	}

	changes := []plan.Change{
		{
			Type:     plan.ChangeCreate,
			Resource: "Variable",
			Name:     "myorg/myrepo",
			Field:    "MY_VAR",
			NewValue: "my-value",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"variable", "set", "MY_VAR", "--repo", "myorg/myrepo", "--body", "my-value"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplySecretSet(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	repo.Spec.Secrets = []manifest.Secret{
		{Name: "MY_SECRET", Value: "secret-value"},
	}

	changes := []plan.Change{
		{
			Type:     plan.ChangeCreate,
			Resource: "Secret",
			Name:     "myorg/myrepo",
			Field:    "MY_SECRET",
			NewValue: "secret-value",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	args := mock.Called[0]
	expected := []string{"secret", "set", "MY_SECRET", "--repo", "myorg/myrepo", "--body", "secret-value"}
	if strings.Join(args, " ") != strings.Join(expected, " ") {
		t.Errorf("args: got %v, want %v", args, expected)
	}
}

func TestApplySkipsNoOp(t *testing.T) {
	mock := &gh.MockRunner{}
	exec := NewExecutor(mock)

	repo := newTestRepo("myorg", "myrepo")
	changes := []plan.Change{
		{
			Type:     plan.ChangeNoOp,
			Resource: "Repository",
			Name:     "myorg/myrepo",
			Field:    "description",
		},
	}

	results := exec.Apply(changes, []*manifest.Repository{repo})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for noop, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Fatalf("expected 0 calls for noop, got %d", len(mock.Called))
	}
}
