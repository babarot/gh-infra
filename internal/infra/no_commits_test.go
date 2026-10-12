package infra

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// missingRepoRunner answers as GitHub does for a repository that does not
// exist, and succeeds with no output for anything else.
type missingRepoRunner struct{ repo string }

func (r missingRepoRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(cmd, "repo view "+r.repo+" "):
		return nil, fmt.Errorf("GraphQL: Could not resolve to a Repository with the name '%s'. (repository)", r.repo)
	case strings.HasPrefix(cmd, "api repos/"+r.repo+"/"):
		return nil, gh.ErrNotFound
	}
	return nil, nil
}

func (r missingRepoRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

const pullRequestFileSet = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: o
  name: files
spec:
  via: pull_request
  repositories:
    - new
  files:
    - path: README.md
      content: hi
`

const newRepoWithPullRequestFileSet = `apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: o
  name: new
spec:
  visibility: private
---
` + pullRequestFileSet

// A repository this apply creates has no commits when its FileSets write to
// it, so a pull_request FileSet is planned as a push, with the reason.
func TestPlan_PullRequestToNewRepoIsPush(t *testing.T) {
	path := writeManifest(t, newRepoWithPullRequestFileSet)
	result, err := Plan(context.Background(), PlanOptions{Runner: missingRepoRunner{repo: "o/new"}, Paths: []string{path}, DryRun: true, Output: OutputJSON})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(result.FailedTargets) != 0 {
		t.Fatalf("failed targets: %v", result.TargetErrors)
	}
	if len(result.FileChanges) != 1 {
		t.Fatalf("file changes = %+v", result.FileChanges)
	}
	if c := result.FileChanges[0]; c.Via != manifest.ViaPush || !c.NoCommits {
		t.Errorf("Via = %q, NoCommits = %v, want push and true", c.Via, c.NoCommits)
	}

	doc := result.PlanDocument(false)
	var warned bool
	for _, w := range doc.Warnings {
		warned = warned || (w.Target == "o/new" && w.Message == fileset.NoCommitsNote)
	}
	if !warned {
		t.Errorf("document warnings = %+v, want the no commits note for o/new", doc.Warnings)
	}
	for _, c := range doc.Changes {
		if c.Path == "README.md" && c.Via != manifest.ViaPush {
			t.Errorf("document via = %q, want push", c.Via)
		}
	}
}

// A repository that is not created keeps the FileSet's via: apply fails on
// it rather than push to it.
func TestPlan_PullRequestToMissingRepoKeepsVia(t *testing.T) {
	path := writeManifest(t, pullRequestFileSet)
	result, err := Plan(context.Background(), PlanOptions{Runner: missingRepoRunner{repo: "o/new"}, Paths: []string{path}, DryRun: true, Output: OutputJSON})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(result.FileChanges) != 1 || result.FileChanges[0].Via != manifest.ViaPullRequest {
		t.Errorf("file changes = %+v, want pull_request", result.FileChanges)
	}
}

func TestPrintPlan_NoCommitsNote(t *testing.T) {
	change := fileset.Change{Target: "o/new", Path: "README.md", Type: fileset.ChangeCreate, Desired: "hi", Via: manifest.ViaPush, NoCommits: true}
	var buf bytes.Buffer
	printPlan(ui.NewStandardPrinterWith(&buf, &buf), []repository.Change{{
		Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: "o/new", Field: "repository", NewValue: "o/new",
	}}, []fileset.Change{change}, false)
	out := buf.String()
	if !strings.Contains(out, "via push") || !strings.Contains(out, fileset.NoCommitsNote) {
		t.Errorf("plan lacks via push and the note:\n%s", out)
	}
}
