package cmd

import (
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/ui"
)

func TestBuildDiffEntries(t *testing.T) {
	changes := []fileset.FileApplyChange{
		{FileChange: fileset.FileChange{Path: "a.txt", Target: "org/repo", Type: fileset.FileCreate, Desired: "new\n"}},
		{FileChange: fileset.FileChange{Path: "b.txt", Target: "org/repo", Type: fileset.FileUpdate, Current: "old\n", Desired: "new\n"}},
		{FileChange: fileset.FileChange{Path: "c.txt", Target: "org/repo", Type: fileset.FileNoOp}},
		{FileChange: fileset.FileChange{Path: "e.txt", Target: "org/repo", Type: fileset.FileDelete, Current: "bye\n"}},
	}

	entries := buildDiffEntries(changes)

	// Should filter out NoOp
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Check icons
	if entries[0].Icon != ui.IconAdd {
		t.Errorf("a.txt icon = %q, want %q", entries[0].Icon, ui.IconAdd)
	}
	if entries[1].Icon != ui.IconChange {
		t.Errorf("b.txt icon = %q, want %q", entries[1].Icon, ui.IconChange)
	}
	if entries[2].Icon != ui.IconRemove {
		t.Errorf("e.txt icon = %q, want %q", entries[2].Icon, ui.IconRemove)
	}

	// Check Target passthrough
	if entries[0].Target != "org/repo" {
		t.Errorf("a.txt Target = %q, want org/repo", entries[0].Target)
	}

	// Check Current/Desired passthrough
	if entries[1].Current != "old\n" || entries[1].Desired != "new\n" {
		t.Errorf("b.txt Current/Desired mismatch")
	}
}

func TestApplySkipSelections_NoSkip(t *testing.T) {
	changes := []fileset.FileApplyChange{
		{FileChange: fileset.FileChange{Path: "a.txt", Target: "org/repo", Type: fileset.FileUpdate}},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", Target: "org/repo", Skip: false},
	}

	applySkipSelections(changes, entries)

	if changes[0].Type != fileset.FileUpdate {
		t.Errorf("expected FileUpdate (unchanged), got %s", changes[0].Type)
	}
}

func TestApplySkipSelections_SkipMarked(t *testing.T) {
	changes := []fileset.FileApplyChange{
		{FileChange: fileset.FileChange{Path: "a.txt", Target: "org/repo", Type: fileset.FileUpdate}},
		{FileChange: fileset.FileChange{Path: "b.txt", Target: "org/repo", Type: fileset.FileCreate}},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", Target: "org/repo", Skip: true},
		{Path: "b.txt", Target: "org/repo", Skip: false},
	}

	applySkipSelections(changes, entries)

	if changes[0].Type != fileset.FileNoOp {
		t.Errorf("a.txt: expected FileNoOp (skipped), got %s", changes[0].Type)
	}
	if changes[1].Type != fileset.FileCreate {
		t.Errorf("b.txt: expected FileCreate (not skipped), got %s", changes[1].Type)
	}
}

func TestApplySkipSelections_DifferentTargets(t *testing.T) {
	changes := []fileset.FileApplyChange{
		{FileChange: fileset.FileChange{Path: "a.txt", Target: "org/repo-a", Type: fileset.FileUpdate}},
		{FileChange: fileset.FileChange{Path: "a.txt", Target: "org/repo-b", Type: fileset.FileUpdate}},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", Target: "org/repo-a", Skip: true},
		{Path: "a.txt", Target: "org/repo-b", Skip: false},
	}

	applySkipSelections(changes, entries)

	if changes[0].Type != fileset.FileNoOp {
		t.Errorf("repo-a: expected FileNoOp (skipped), got %s", changes[0].Type)
	}
	if changes[1].Type != fileset.FileUpdate {
		t.Errorf("repo-b: expected FileUpdate (not skipped), got %s", changes[1].Type)
	}
}
