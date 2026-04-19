package infra

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
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
			Details: []repository.Change{
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
			Details: []repository.Change{
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
	item := changeToItem(c, ui.IndentItem)

	if item.Icon != ui.IconAdd {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconAdd)
	}
	if item.Field != "homepage" {
		t.Errorf("field = %q, want homepage", item.Field)
	}
	if item.Level != ui.IndentItem {
		t.Errorf("expected Level=IndentItem, got %d", item.Level)
	}
}

func TestChangeToItem_Update(t *testing.T) {
	c := repository.Change{Type: repository.ChangeUpdate, Field: "description", OldValue: "old", NewValue: "new"}
	item := changeToItem(c, ui.IndentSub)

	if item.Icon != ui.IconChange {
		t.Errorf("icon = %q, want %q", item.Icon, ui.IconChange)
	}
	if item.Old != "old" || item.New != "new" {
		t.Errorf("old/new = %q/%q, want old/new", item.Old, item.New)
	}
	if item.Level != ui.IndentSub {
		t.Errorf("expected Level=IndentSub, got %d", item.Level)
	}
}

func TestChangeToItem_Delete(t *testing.T) {
	c := repository.Change{Type: repository.ChangeDelete, Field: "topics", OldValue: []string{"go"}}
	item := changeToItem(c, ui.IndentItem)

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

// ---------------------------------------------------------------------------
// groupRepoChanges
// ---------------------------------------------------------------------------

func TestGroupRepoChanges_NoGroupable(t *testing.T) {
	changes := []repository.Change{
		{Resource: "Repository", Field: "description"},
		{Resource: "Repository", Field: "topics"},
	}
	groups := groupRepoChanges(changes)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.resource != "" {
			t.Errorf("expected empty resource for regular change, got %q", g.resource)
		}
	}
}

func TestGroupRepoChanges_ConsecutiveLabels(t *testing.T) {
	changes := []repository.Change{
		{Resource: manifest.ResourceLabel, Field: "bug"},
		{Resource: manifest.ResourceLabel, Field: "feature"},
	}
	groups := groupRepoChanges(changes)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].resource != manifest.ResourceLabel {
		t.Errorf("resource = %q, want %q", groups[0].resource, manifest.ResourceLabel)
	}
	if len(groups[0].changes) != 2 {
		t.Errorf("expected 2 changes in group, got %d", len(groups[0].changes))
	}
}

func TestGroupRepoChanges_PreservesOrder(t *testing.T) {
	changes := []repository.Change{
		{Resource: "Repository", Field: "description"},
		{Resource: manifest.ResourceLabel, Field: "bug"},
		{Resource: manifest.ResourceLabel, Field: "feature"},
		{Resource: manifest.ResourceMilestone, Field: "v1.0"},
		{Resource: "Actions", Field: "actions", Details: []repository.Change{{Field: "enabled"}}},
	}
	groups := groupRepoChanges(changes)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(groups))
	}
	// Order: regular(description), labels(bug+feature), milestones(v1.0), regular(actions)
	if groups[0].resource != "" {
		t.Errorf("group[0] resource = %q, want empty", groups[0].resource)
	}
	if groups[1].resource != manifest.ResourceLabel {
		t.Errorf("group[1] resource = %q, want Label", groups[1].resource)
	}
	if len(groups[1].changes) != 2 {
		t.Errorf("group[1] should have 2 changes, got %d", len(groups[1].changes))
	}
	if groups[2].resource != manifest.ResourceMilestone {
		t.Errorf("group[2] resource = %q, want Milestone", groups[2].resource)
	}
	if groups[3].resource != "" {
		t.Errorf("group[3] resource = %q, want empty", groups[3].resource)
	}
}

func TestGroupRepoChanges_NonConsecutiveSameResource(t *testing.T) {
	changes := []repository.Change{
		{Resource: manifest.ResourceLabel, Field: "bug"},
		{Resource: "Repository", Field: "description"},
		{Resource: manifest.ResourceLabel, Field: "feature"},
	}
	groups := groupRepoChanges(changes)
	// Labels are split into two separate groups because they're not consecutive
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].resource != manifest.ResourceLabel || groups[0].changes[0].Field != "bug" {
		t.Errorf("group[0] unexpected: %+v", groups[0])
	}
	if groups[1].resource != "" {
		t.Errorf("group[1] resource = %q, want empty", groups[1].resource)
	}
	if groups[2].resource != manifest.ResourceLabel || groups[2].changes[0].Field != "feature" {
		t.Errorf("group[2] unexpected: %+v", groups[2])
	}
}

// ---------------------------------------------------------------------------
// groupIcon
// ---------------------------------------------------------------------------

func TestGroupIcon(t *testing.T) {
	tests := []struct {
		name    string
		changes []repository.Change
		want    string
	}{
		{"all creates", []repository.Change{
			{Type: repository.ChangeCreate},
			{Type: repository.ChangeCreate},
		}, ui.IconAdd},
		{"all deletes", []repository.Change{
			{Type: repository.ChangeDelete},
		}, ui.IconRemove},
		{"mixed", []repository.Change{
			{Type: repository.ChangeCreate},
			{Type: repository.ChangeUpdate},
		}, ui.IconChange},
		{"single update", []repository.Change{
			{Type: repository.ChangeUpdate},
		}, ui.IconChange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupIcon(tt.changes)
			if got != tt.want {
				t.Errorf("groupIcon() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// printPlan with grouped resources
// ---------------------------------------------------------------------------

func TestPrintPlan_GroupedLabelsAndMilestones(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{Type: repository.ChangeUpdate, Resource: "Repository", Name: "org/repo", Field: "description", OldValue: "old", NewValue: "new"},
		{Type: repository.ChangeCreate, Resource: manifest.ResourceLabel, Name: "org/repo", Field: "bug", NewValue: `#d73a4a "A bug"`},
		{Type: repository.ChangeCreate, Resource: manifest.ResourceLabel, Name: "org/repo", Field: "feature", NewValue: `#425df5`},
		{Type: repository.ChangeCreate, Resource: manifest.ResourceMilestone, Name: "org/repo", Field: "v1.0", NewValue: "open"},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	if !strings.Contains(out, "labels") {
		t.Errorf("expected 'labels' group header, got:\n%s", out)
	}
	if !strings.Contains(out, "milestones") {
		t.Errorf("expected 'milestones' group header, got:\n%s", out)
	}
	if !strings.Contains(out, "bug") {
		t.Errorf("expected label 'bug' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "v1.0") {
		t.Errorf("expected milestone 'v1.0' in output, got:\n%s", out)
	}

	// Verify order: description before labels before milestones
	descIdx := strings.Index(out, "description")
	labelsIdx := strings.Index(out, "labels")
	msIdx := strings.Index(out, "milestones")
	if descIdx > labelsIdx {
		t.Errorf("description should appear before labels")
	}
	if labelsIdx > msIdx {
		t.Errorf("labels should appear before milestones")
	}
}

func TestPrintPlan_GroupedLabelUpdate(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{
			Type:     repository.ChangeUpdate,
			Resource: manifest.ResourceLabel,
			Name:     "org/repo",
			Field:    "bug",
			Details: []repository.Change{
				{Type: repository.ChangeUpdate, Field: "color", OldValue: "d73a4a", NewValue: "FF0000"},
			},
		},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	if !strings.Contains(out, "labels") {
		t.Errorf("expected 'labels' group header, got:\n%s", out)
	}
	// Children should be flattened with "name.field" prefix
	if !strings.Contains(out, "bug.color") {
		t.Errorf("expected 'bug.color' flattened field, got:\n%s", out)
	}
}

func TestPrintPlan_RulesetAndBranchProtectionHeaders(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{
			Type:     repository.ChangeUpdate,
			Resource: `Ruleset[main-protection]`,
			Name:     "org/repo",
			Field:    "ruleset",
			NewValue: "main-protection",
			Details: []repository.Change{
				{Type: repository.ChangeUpdate, Field: "enforcement", OldValue: "evaluate", NewValue: "active"},
			},
		},
		{
			Type:     repository.ChangeUpdate,
			Resource: `BranchProtection[main]`,
			Name:     "org/repo",
			Field:    "branch_protection",
			NewValue: "main",
			Details: []repository.Change{
				{Type: repository.ChangeUpdate, Field: "required_reviews", OldValue: 0, NewValue: 1},
			},
		},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	if !strings.Contains(out, `ruleset "main-protection"`) {
		t.Errorf("expected human ruleset header, got:\n%s", out)
	}
	if !strings.Contains(out, `branch protection "main"`) {
		t.Errorf("expected human branch protection header, got:\n%s", out)
	}
	if strings.Contains(out, "ruleset[") || strings.Contains(out, "branch_protection[") {
		t.Errorf("output contains raw collection header:\n%s", out)
	}
}

func TestPrintPlan_DeleteRulesetAndBranchProtectionWithChildren(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	repoChanges := []repository.Change{
		{
			Type:     repository.ChangeDelete,
			Resource: `Ruleset[old-ruleset]`,
			Name:     "org/repo",
			Field:    "ruleset",
			OldValue: 42,
			Details: []repository.Change{
				{Type: repository.ChangeDelete, Field: "target", OldValue: "branch"},
				{Type: repository.ChangeDelete, Field: "enforcement", OldValue: "active"},
			},
		},
		{
			Type:     repository.ChangeDelete,
			Resource: `BranchProtection[main]`,
			Name:     "org/repo",
			Field:    "branch_protection",
			OldValue: "main",
			Details: []repository.Change{
				{Type: repository.ChangeDelete, Field: "required_reviews", OldValue: 1},
				{Type: repository.ChangeDelete, Field: "enforce_admins", OldValue: false},
			},
		},
	}

	printPlan(p, repoChanges, nil)
	out := buf.String()

	for _, want := range []string{
		`ruleset "old-ruleset"`,
		"target",
		"enforcement",
		`branch protection "main"`,
		"required_reviews",
		"enforce_admins",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "branch_protection      main") {
		t.Errorf("expected grouped branch protection delete, got flat output:\n%s", out)
	}
}
