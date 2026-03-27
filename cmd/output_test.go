package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func init() {
	ui.DisableStyles()
}

func TestSwapChanges_ReversesTypesAndValues(t *testing.T) {
	trueVal := true
	falseVal := false

	changes := []repository.Change{
		{
			Type:  repository.ChangeUpdate,
			Name:  "org/repo",
			Field: "features",
			Children: []repository.Change{
				{
					Type:     repository.ChangeUpdate,
					Field:    "issues",
					OldValue: trueVal,
					NewValue: falseVal,
				},
				{
					Type:     repository.ChangeCreate,
					Field:    "wiki",
					NewValue: trueVal,
				},
				{
					Type:     repository.ChangeDelete,
					Field:    "projects",
					OldValue: falseVal,
				},
			},
		},
	}

	swapped := repository.ReverseChanges(changes)
	children := swapped[0].Children

	if children[0].Type != repository.ChangeUpdate {
		t.Fatalf("child[0] type = %s, want update", children[0].Type)
	}
	if children[0].OldValue != falseVal || children[0].NewValue != trueVal {
		t.Fatalf("child[0] values = %v -> %v, want false -> true", children[0].OldValue, children[0].NewValue)
	}
	if children[1].Type != repository.ChangeDelete {
		t.Fatalf("child[1] type = %s, want delete", children[1].Type)
	}
	if children[1].OldValue != trueVal {
		t.Fatalf("child[1] old value = %v, want true", children[1].OldValue)
	}
	if children[2].Type != repository.ChangeCreate {
		t.Fatalf("child[2] type = %s, want create", children[2].Type)
	}
	if children[2].NewValue != falseVal {
		t.Fatalf("child[2] new value = %v, want false", children[2].NewValue)
	}
}

func TestPrintUnifiedImportPlan_PrintsNestedRepoChanges(t *testing.T) {
	repoChanges := []repository.Change{
		{
			Type:  repository.ChangeUpdate,
			Name:  "org/repo",
			Field: "features",
			Children: []repository.Change{
				{
					Type:     repository.ChangeUpdate,
					Field:    "issues",
					OldValue: "false",
					NewValue: "true",
				},
			},
		},
		{
			Type:     repository.ChangeUpdate,
			Name:     "org/repo",
			Field:    "visibility",
			OldValue: "private",
			NewValue: "public",
		},
	}
	importChanges := []fileset.FileImportChange{
		{
			Target:    "org/repo",
			Path:      ".github/workflows/ci.yml",
			Type:      fileset.FileUpdate,
			Current:   "old\n",
			Desired:   "new\n",
			WriteMode: fileset.ImportWriteSource,
		},
	}

	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)
	printUnifiedImportPlan(p, repoChanges, importChanges)

	out := buf.String()
	if strings.Contains(out, "<nil>") {
		t.Fatalf("unexpected <nil> in output:\n%s", out)
	}
	if !strings.Contains(out, "features") {
		t.Fatalf("expected features group in output:\n%s", out)
	}
	if !strings.Contains(out, "issues") {
		t.Fatalf("expected nested issues field in output:\n%s", out)
	}
	if !strings.Contains(out, "visibility") {
		t.Fatalf("expected visibility field in output:\n%s", out)
	}
}

func TestPrintUnifiedImportPlan_ShowsSkippedAndWarningsInsideFileSet(t *testing.T) {
	repoChanges := []repository.Change{
		{
			Type:     repository.ChangeUpdate,
			Name:     "org/repo",
			Field:    "visibility",
			OldValue: "private",
			NewValue: "public",
		},
	}
	importChanges := []fileset.FileImportChange{
		{
			Target:    "org/repo",
			Path:      "VERSION",
			Type:      fileset.FileNoOp,
			WriteMode: fileset.ImportSkip,
			Reason:    "create_only",
		},
		{
			Target:    "org/repo",
			Path:      ".github/workflows/build.yaml",
			Type:      fileset.FileUpdate,
			Current:   "old\n",
			Desired:   "new\n",
			WriteMode: fileset.ImportWriteSource,
			LocalTarget: "templates/build.yaml",
		},
	}

	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)
	printUnifiedImportPlan(p, repoChanges, importChanges)

	out := buf.String()
	if !strings.Contains(out, "FileSet: 2 files") {
		t.Fatalf("expected FileSet count in output:\n%s", out)
	}
	if !strings.Contains(out, "VERSION") || !strings.Contains(out, "create_only") {
		t.Fatalf("expected skipped file in output:\n%s", out)
	}
	if !strings.Contains(out, "templates/build.yaml") || !strings.Contains(out, "(content changed)") {
		t.Fatalf("expected update line in output:\n%s", out)
	}
}

func TestImportDisplayPath_ShortensLongLocalTarget(t *testing.T) {
	change := fileset.FileImportChange{
		WriteMode:   fileset.ImportWriteSource,
		LocalTarget: "templates/common/.github/PULL_REQUEST_TEMPLATE.md",
	}

	got := importDisplayPath(change)
	want := "templates/.../PULL_REQUEST_TEMPLATE.md"
	if got != want {
		t.Fatalf("importDisplayPath() = %q, want %q", got, want)
	}
}
