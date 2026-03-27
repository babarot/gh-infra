package infra

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func init() {
	ui.DisableStyles()
}

// ---------------------------------------------------------------------------
// PrintPlan
// ---------------------------------------------------------------------------

func TestPrintPlan_RepoChanges(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{Type: repository.ChangeUpdate, Name: "org/repo", Field: "description", OldValue: "old", NewValue: "new"},
		{Type: repository.ChangeCreate, Name: "org/repo", Field: "homepage", NewValue: "https://example.com"},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	if !strings.Contains(out, "org/repo") {
		t.Errorf("expected repo name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "description") {
		t.Errorf("expected field name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "will be updated") {
		t.Errorf("expected 'will be updated' header, got:\n%s", out)
	}
}

func TestPrintPlan_FileChanges(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	fileChanges := []fileset.Change{
		{Type: fileset.ChangeCreate, Target: "org/repo", Path: ".github/ci.yml", Via: "push"},
		{Type: fileset.ChangeUpdate, Target: "org/repo", Path: ".github/lint.yml", Current: "old\n", Desired: "new\n", Via: "push"},
	}

	printPlan(p, nil, fileChanges)
	out := buf.String()

	if !strings.Contains(out, "org/repo") {
		t.Errorf("expected repo name in output, got:\n%s", out)
	}
	if !strings.Contains(out, "FileSet") {
		t.Errorf("expected FileSet label in output, got:\n%s", out)
	}
	if !strings.Contains(out, ".github/ci.yml") {
		t.Errorf("expected file path in output, got:\n%s", out)
	}
}

func TestPrintPlan_Mixed(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{Type: repository.ChangeUpdate, Name: "org/repo", Field: "visibility", OldValue: "private", NewValue: "public"},
	}
	fileChanges := []fileset.Change{
		{Type: fileset.ChangeCreate, Target: "org/repo", Path: "README.md"},
	}

	printPlan(p, repoChanges, fileChanges)
	out := buf.String()

	// Both repo and file changes should appear under same repo group
	if !strings.Contains(out, "visibility") {
		t.Errorf("expected repo field in output, got:\n%s", out)
	}
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected file path in output, got:\n%s", out)
	}
}

func TestPrintPlan_AllNoOp(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{Type: repository.ChangeNoOp, Name: "org/repo"},
	}
	fileChanges := []fileset.Change{
		{Type: fileset.ChangeNoOp, Target: "org/repo"},
	}

	printPlan(p, repoChanges, fileChanges)

	if buf.Len() != 0 {
		t.Errorf("expected empty output for all no-op, got:\n%s", buf.String())
	}
}

func TestPrintPlan_Empty(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	printPlan(p, nil, nil)

	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil changes, got:\n%s", buf.String())
	}
}

func TestPrintPlan_ChildChanges(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{
			Type:  repository.ChangeUpdate,
			Name:  "org/repo",
			Field: "features",
			Children: []repository.Change{
				{Type: repository.ChangeUpdate, Field: "issues", OldValue: true, NewValue: false},
				{Type: repository.ChangeUpdate, Field: "wiki", OldValue: false, NewValue: true},
			},
		},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	if !strings.Contains(out, "features") {
		t.Errorf("expected parent field in output, got:\n%s", out)
	}
	if !strings.Contains(out, "issues") {
		t.Errorf("expected child field 'issues' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "wiki") {
		t.Errorf("expected child field 'wiki' in output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// PrintApplyResults
// ---------------------------------------------------------------------------

func TestPrintApplyResults_Success(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoResults := []repository.ApplyResult{
		{Change: repository.Change{Type: repository.ChangeUpdate, Name: "org/repo", Field: "description"}, Err: nil},
	}

	printApplyResults(p, repoResults, nil)
	out := buf.String()

	if !strings.Contains(out, "✓") {
		t.Errorf("expected success icon in output, got:\n%s", out)
	}
	if !strings.Contains(out, "description") {
		t.Errorf("expected field name in output, got:\n%s", out)
	}
}

func TestPrintApplyResults_Error(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoResults := []repository.ApplyResult{
		{Change: repository.Change{Type: repository.ChangeUpdate, Name: "org/repo", Field: "description"}, Err: fmt.Errorf("forbidden")},
	}

	printApplyResults(p, repoResults, nil)
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected error icon in output, got:\n%s", out)
	}
	if !strings.Contains(out, "forbidden") {
		t.Errorf("expected error message in output, got:\n%s", out)
	}
}

func TestPrintApplyResults_FileResults(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	fileResults := []fileset.ApplyResult{
		{Change: fileset.Change{Type: fileset.ChangeCreate, Target: "org/repo", Path: "a.txt"}, Via: "push"},
		{Change: fileset.Change{Type: fileset.ChangeUpdate, Target: "org/repo", Path: "b.txt"}, Via: "push"},
	}

	printApplyResults(p, nil, fileResults)
	out := buf.String()

	if !strings.Contains(out, "a.txt") {
		t.Errorf("expected file path a.txt in output, got:\n%s", out)
	}
	if !strings.Contains(out, "via push") {
		t.Errorf("expected delivery method in output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// repoFieldWidth / filePathWidth
// ---------------------------------------------------------------------------

func TestRepoFieldWidth(t *testing.T) {
	changes := []repository.Change{
		{Field: "description"},
		{Field: "homepage_url"},
	}
	w := repoFieldWidth(changes)
	if w != len("homepage_url") {
		t.Errorf("expected %d, got %d", len("homepage_url"), w)
	}
}

func TestRepoFieldWidth_Children(t *testing.T) {
	changes := []repository.Change{
		{
			Field: "features",
			Children: []repository.Change{
				{Field: "short"},
				{Field: "very_long_child_field_name"},
			},
		},
	}
	w := repoFieldWidth(changes)
	if w != len("very_long_child_field_name") {
		t.Errorf("expected %d, got %d", len("very_long_child_field_name"), w)
	}
}

func TestRepoFieldWidth_Empty(t *testing.T) {
	w := repoFieldWidth(nil)
	if w != 0 {
		t.Errorf("expected 0, got %d", w)
	}
}

func TestFilePathWidth(t *testing.T) {
	changes := []fileset.Change{
		{Path: "a.txt"},
		{Path: ".github/workflows/ci.yml"},
	}
	w := filePathWidth(changes)
	if w != len(".github/workflows/ci.yml") {
		t.Errorf("expected %d, got %d", len(".github/workflows/ci.yml"), w)
	}
}

func TestFilePathWidth_Empty(t *testing.T) {
	w := filePathWidth(nil)
	if w != 0 {
		t.Errorf("expected 0, got %d", w)
	}
}

// ---------------------------------------------------------------------------
// changeToItem
// ---------------------------------------------------------------------------

func TestChangeToItem_Create(t *testing.T) {
	c := repository.Change{Type: repository.ChangeCreate, Field: "homepage", NewValue: "https://example.com"}
	item := changeToItem(c, false)

	if item.Icon != ui.IconAdd {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconAdd)
	}
	if item.Field != "homepage" {
		t.Errorf("field = %q, want homepage", item.Field)
	}
	if item.Sub {
		t.Error("expected Sub=false")
	}
}

func TestChangeToItem_Update(t *testing.T) {
	c := repository.Change{Type: repository.ChangeUpdate, Field: "description", OldValue: "old", NewValue: "new"}
	item := changeToItem(c, true)

	if item.Icon != ui.IconChange {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconChange)
	}
	if item.Old != "old" || item.New != "new" {
		t.Errorf("old/new = %q/%q, want old/new", item.Old, item.New)
	}
	if !item.Sub {
		t.Error("expected Sub=true")
	}
}

func TestChangeToItem_Delete(t *testing.T) {
	c := repository.Change{Type: repository.ChangeDelete, Field: "topics", OldValue: []string{"go"}}
	item := changeToItem(c, false)

	if item.Icon != ui.IconRemove {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconRemove)
	}
}

// ---------------------------------------------------------------------------
// fileChangeToItem
// ---------------------------------------------------------------------------

func TestFileChangeToItem_Create(t *testing.T) {
	c := fileset.Change{Type: fileset.ChangeCreate, Path: "README.md"}
	item := fileChangeToItem(c, 10, 0)

	if item.Icon != ui.IconAdd {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconAdd)
	}
	if item.Added != 10 {
		t.Errorf("added = %d, want 10", item.Added)
	}
}

func TestFileChangeToItem_Update(t *testing.T) {
	c := fileset.Change{Type: fileset.ChangeUpdate, Path: "ci.yml"}
	item := fileChangeToItem(c, 5, 3)

	if item.Icon != ui.IconChange {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconChange)
	}
	if item.Added != 5 || item.Removed != 3 {
		t.Errorf("added/removed = %d/%d, want 5/3", item.Added, item.Removed)
	}
}

func TestFileChangeToItem_Delete(t *testing.T) {
	c := fileset.Change{Type: fileset.ChangeDelete, Path: "old.txt"}
	item := fileChangeToItem(c, 0, 20)

	if item.Icon != ui.IconRemove {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconRemove)
	}
	if item.Removed != 20 {
		t.Errorf("removed = %d, want 20", item.Removed)
	}
}
