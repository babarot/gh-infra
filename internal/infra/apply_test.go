package infra

import (
	"fmt"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

func TestFileSetApplyArgs(t *testing.T) {
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "org"},
		Spec: manifest.FileSetSpec{
			CommitMessage: "sync files",
			Via:           "push",
			Branch:        "main",
			PRTitle:       "pr title",
			PRBody:        "pr body",
		},
	}
	allChanges := []fileset.Change{
		{FileSetOwner: "org", Target: "org/repo1", Path: "a.txt"},
		{FileSetOwner: "other", Target: "other/repo2", Path: "b.txt"},
		{FileSetOwner: "org", Target: "org/repo3", Path: "c.txt"},
	}

	changes, opts := fileSetApplyArgs(fs, allChanges)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].Target != "org/repo1" {
		t.Errorf("changes[0].Target = %q", changes[0].Target)
	}
	if changes[1].Target != "org/repo3" {
		t.Errorf("changes[1].Target = %q", changes[1].Target)
	}
	if opts.CommitMessage != "sync files" {
		t.Errorf("CommitMessage = %q", opts.CommitMessage)
	}
	if opts.Via != "push" {
		t.Errorf("Via = %q", opts.Via)
	}
	if opts.Branch != "main" {
		t.Errorf("Branch = %q", opts.Branch)
	}
	if opts.PRTitle != "pr title" {
		t.Errorf("PRTitle = %q", opts.PRTitle)
	}
	if opts.PRBody != "pr body" {
		t.Errorf("PRBody = %q", opts.PRBody)
	}
}

func TestFileSetApplyArgs_NoMatch(t *testing.T) {
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "org"},
	}
	allChanges := []fileset.Change{
		{FileSetOwner: "other", Target: "other/repo"},
	}
	changes, _ := fileSetApplyArgs(fs, allChanges)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

func TestCountFileResults(t *testing.T) {
	results := []fileset.ApplyResult{
		{Err: nil},
		{Err: nil},
		{Err: fmt.Errorf("fail")},
		{Err: nil},
	}
	s, f := countFileResults(results)
	if s != 3 {
		t.Errorf("succeeded = %d, want 3", s)
	}
	if f != 1 {
		t.Errorf("failed = %d, want 1", f)
	}
}

func TestCountFileResults_Empty(t *testing.T) {
	s, f := countFileResults(nil)
	if s != 0 || f != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", s, f)
	}
}
