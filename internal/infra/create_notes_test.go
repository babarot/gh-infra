package infra

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func newRepoWithNotes() (*manifest.Repository, []repository.Change) {
	repo := &manifest.Repository{
		Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "new"},
		Reconcile: &manifest.RepositoryReconcile{Labels: manifest.Ptr(manifest.CollectionReconcileAuthoritative)},
		Spec: manifest.RepositorySpec{
			Labels:           []manifest.Label{{Name: "bug", Color: "ff0000"}},
			BranchProtection: []manifest.BranchProtection{{Pattern: "main", RequiredReviews: manifest.Ptr(1)}},
		},
	}
	changes := []repository.Change{
		{Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: "o/new", Field: "repository", NewValue: "o/new"},
		{Type: repository.ChangeCreate, Resource: manifest.ResourceLabel, Name: "o/new", Field: "bug"},
	}
	return repo, changes
}

// What a create leaves to the next plan is said, but not planned as a change.
func TestCreateNotes(t *testing.T) {
	repo, changes := newRepoWithNotes()
	notes := createNotes(changes, []*manifest.Repository{repo})["o/new"]
	for _, want := range []string{"updated instead of created", "branch protection for main", "default labels not declared here are deleted"} {
		if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, want) }) {
			t.Errorf("notes = %q, want one about %q", notes, want)
		}
	}

	existing := []repository.Change{{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/new", Field: "description"}}
	if got := createNotes(existing, []*manifest.Repository{repo}); len(got) != 0 {
		t.Errorf("notes for a repository that exists = %v, want none", got)
	}
}

func TestCreateNotes_ShownInTextAndDocument(t *testing.T) {
	repo, changes := newRepoWithNotes()
	notes := createNotes(changes, []*manifest.Repository{repo})

	var buf bytes.Buffer
	printPlanWithNotes(ui.NewStandardPrinterWith(&buf, &buf), changes, nil, notes, false)
	if !strings.Contains(buf.String(), "branch protection for main is applied by a later plan") {
		t.Errorf("text plan lacks the note:\n%s", buf.String())
	}

	doc := (&PlanResult{RepoChanges: changes, TargetRepos: []*manifest.Repository{repo}, CreateNotes: notes}).PlanDocument(false)
	var found bool
	for _, w := range doc.Warnings {
		if w.Target == "o/new" && strings.Contains(w.Message, "branch protection for main") {
			found = true
		}
	}
	if !found {
		t.Errorf("document warnings = %+v, want the note for o/new", doc.Warnings)
	}
	for _, c := range doc.Changes {
		if strings.Contains(c.Resource, "BranchProtection") {
			t.Errorf("branch protection is listed as a change: %+v", c)
		}
	}
}
