package importer

import (
	"testing"

	"github.com/babarot/gh-infra/internal/repository"
)

func TestMatchesHelpers(t *testing.T) {
	var m Matches
	if !m.IsEmpty() {
		t.Fatalf("empty matches should be empty")
	}
	if m.HasRepo() || m.HasFiles() {
		t.Fatalf("empty matches should have no repo/files")
	}
}

func TestIntoPlanSummary(t *testing.T) {
	plan := IntoPlan{
		UpdatedDocs: 2,
		FileChanges: []FileChange{
			{Type: "update", WriteMode: ImportWriteSource},
			{Type: "noop", WriteMode: ImportWriteSource},
			{Type: "noop", WriteMode: ImportSkip},
		},
	}

	written, unchanged, skipped := plan.Summary()
	if written != 3 || unchanged != 1 || skipped != 1 {
		t.Fatalf("Summary() = (%d, %d, %d), want (3, 1, 1)", written, unchanged, skipped)
	}
}

func TestIntoPlanAddRepoPlan(t *testing.T) {
	plan := IntoPlan{ManifestEdits: make(map[string][]byte)}
	repoPlan := RepoPlan{
		Changes: []repository.Change{{Field: "visibility"}},
		ManifestEdits: map[string][]byte{
			"repo.yaml": []byte("spec: {}"),
		},
		UpdatedDocs: 1,
	}

	plan.AddRepoPlan(repoPlan)

	if len(plan.RepoChanges) != 1 {
		t.Fatalf("len(RepoChanges) = %d, want 1", len(plan.RepoChanges))
	}
	if len(plan.ManifestEdits) != 1 {
		t.Fatalf("len(ManifestEdits) = %d, want 1", len(plan.ManifestEdits))
	}
	if plan.UpdatedDocs != 1 {
		t.Fatalf("UpdatedDocs = %d, want 1", plan.UpdatedDocs)
	}
}
