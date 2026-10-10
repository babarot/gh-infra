package infra

import (
	"fmt"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

func TestFileSetApplyArgs(t *testing.T) {
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Name: "myfs", Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories:  []manifest.FileSetRepository{{Name: "repo1"}, {Name: "repo3"}},
			CommitMessage: "sync files",
			Via:           "push",
			Branch:        "main",
			PRTitle:       "pr title",
			PRBody:        "pr body",
		},
	}
	other := &manifest.FileSet{Metadata: manifest.FileSetMetadata{Name: "otherfs", Owner: "other"}}
	allChanges := []fileset.Change{
		{FileSet: fs, FileSetID: fs.Identity(), Target: "org/repo1", Path: "a.txt"},
		{FileSet: other, FileSetID: other.Identity(), Target: "other/repo2", Path: "b.txt"},
		{FileSet: fs, FileSetID: fs.Identity(), Target: "org/repo3", Path: "c.txt"},
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

func TestFileSetApplyArgs_SameOwnerDifferentName(t *testing.T) {
	fs1 := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Name: "repo-a", Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories: []manifest.FileSetRepository{{Name: "repo-a"}},
		},
	}
	fs2 := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Name: "repo-b", Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories: []manifest.FileSetRepository{{Name: "repo-b"}},
		},
	}
	allChanges := []fileset.Change{
		{FileSet: fs1, FileSetID: fs1.Identity(), Target: "org/repo-a", Path: "a.txt"},
		{FileSet: fs2, FileSetID: fs2.Identity(), Target: "org/repo-b", Path: "b.txt"},
	}

	changes1, _ := fileSetApplyArgs(fs1, allChanges)
	changes2, _ := fileSetApplyArgs(fs2, allChanges)

	if len(changes1) != 1 {
		t.Fatalf("fs1: expected 1 change, got %d", len(changes1))
	}
	if changes1[0].Target != "org/repo-a" {
		t.Errorf("fs1: changes[0].Target = %q, want org/repo-a", changes1[0].Target)
	}
	if len(changes2) != 1 {
		t.Fatalf("fs2: expected 1 change, got %d", len(changes2))
	}
	if changes2[0].Target != "org/repo-b" {
		t.Errorf("fs2: changes[0].Target = %q, want org/repo-b", changes2[0].Target)
	}
}

func TestFileSetApplyArgs_NoMatch(t *testing.T) {
	fs := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Name: "myrepo", Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories: []manifest.FileSetRepository{{Name: "myrepo"}},
		},
	}
	other := &manifest.FileSet{Metadata: manifest.FileSetMetadata{Name: "otherrepo", Owner: "other"}}
	allChanges := []fileset.Change{
		{FileSet: other, FileSetID: other.Identity(), Target: "other/repo"},
	}
	changes, _ := fileSetApplyArgs(fs, allChanges)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(changes))
	}
}

// Two unnamed FileSets targeting the same repos share an Identity; each must
// still get only the changes it planned, applied with its own options.
func TestFileSetApplyArgs_SameIdentity(t *testing.T) {
	repos := []manifest.FileSetRepository{{Name: "repo1"}, {Name: "repo2"}}
	ciFS := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories:  repos,
			Files:         []manifest.FileEntry{{Path: ".github/workflows/ci.yml"}},
			CommitMessage: "ci: sync CI workflow",
		},
	}
	hookFS := &manifest.FileSet{
		Metadata: manifest.FileSetMetadata{Owner: "org"},
		Spec: manifest.FileSetSpec{
			Repositories:  repos,
			Files:         []manifest.FileEntry{{Path: ".hooks/pre-commit"}},
			CommitMessage: "chore: sync pre-commit hook",
		},
	}
	if ciFS.Identity() != hookFS.Identity() {
		t.Fatalf("precondition: identities differ: %q vs %q", ciFS.Identity(), hookFS.Identity())
	}
	allChanges := []fileset.Change{
		{FileSet: ciFS, FileSetID: ciFS.Identity(), Target: "org/repo1", Path: ".github/workflows/ci.yml", Type: fileset.ChangeNoOp},
		{FileSet: hookFS, FileSetID: hookFS.Identity(), Target: "org/repo1", Path: ".hooks/pre-commit", Type: fileset.ChangeUpdate},
		{FileSet: hookFS, FileSetID: hookFS.Identity(), Target: "org/repo2", Path: ".hooks/pre-commit", Type: fileset.ChangeUpdate},
	}

	ciChanges, ciOpts := fileSetApplyArgs(ciFS, allChanges)
	if len(ciChanges) != 1 || ciChanges[0].Path != ".github/workflows/ci.yml" {
		t.Errorf("ci FileSet got changes %+v, want only .github/workflows/ci.yml", ciChanges)
	}
	if fileset.HasChanges(ciChanges) {
		t.Errorf("ci FileSet should have nothing to apply, got %+v", ciChanges)
	}
	if ciOpts.CommitMessage != "ci: sync CI workflow" {
		t.Errorf("ci CommitMessage = %q", ciOpts.CommitMessage)
	}

	hookChanges, hookOpts := fileSetApplyArgs(hookFS, allChanges)
	if len(hookChanges) != 2 {
		t.Fatalf("hook FileSet: expected 2 changes, got %d", len(hookChanges))
	}
	for _, c := range hookChanges {
		if c.Path != ".hooks/pre-commit" {
			t.Errorf("hook FileSet got change for %q", c.Path)
		}
	}
	if hookOpts.CommitMessage != "chore: sync pre-commit hook" {
		t.Errorf("hook CommitMessage = %q", hookOpts.CommitMessage)
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

func TestCountFileResults_SkippedNotCounted(t *testing.T) {
	results := []fileset.ApplyResult{
		{Err: nil},
		{Skipped: true},
		{Skipped: true},
	}
	s, f := countFileResults(results)
	if s != 1 || f != 0 {
		t.Errorf("expected (1, 0), got (%d, %d)", s, f)
	}
}

func TestCountFileResults_Empty(t *testing.T) {
	s, f := countFileResults(nil)
	if s != 0 || f != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", s, f)
	}
}
