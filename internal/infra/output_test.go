package infra

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

func testPlanResult() *PlanResult {
	return &PlanResult{
		RepoChanges: []repository.Change{
			{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/a", Field: "description", OldValue: "old", NewValue: "new"},
			{Type: repository.ChangeNoOp, Resource: manifest.ResourceRepository, Name: "o/a", Field: "homepage"},
			{
				Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/a", Field: "features",
				Details: []repository.Change{{Type: repository.ChangeUpdate, Field: "wiki", OldValue: true, NewValue: false}},
			},
			{
				Type: repository.ChangeCreate, Resource: manifest.ResourceRuleset + "[main]", Name: "o/a", Field: "ruleset", NewValue: "main",
				Details: []repository.Change{{Type: repository.ChangeCreate, Field: "enforcement", NewValue: "active"}},
			},
			{Type: repository.ChangeCreate, Resource: manifest.ResourceSecret, Name: "o/a", Field: "TOKEN", NewValue: "(new)"},
			{Type: repository.ChangeDelete, Resource: manifest.ResourceLabel, Name: "o/b", Field: "old-label", OldValue: "#ffffff"},
		},
		FileChanges: []fileset.Change{
			{Type: fileset.ChangeUpdate, Target: "o/a", Path: "a.sh", Current: "x\n", Desired: "y\n", FileSetID: "o", Via: "push", Mode: fileset.ModeExecutable, CurrentMode: fileset.ModeFile},
			{Type: fileset.ChangeNoOp, Target: "o/a", Path: "same.txt"},
			{Type: fileset.ChangeCreate, Target: "o/c", Path: "new.txt", Desired: "hello\n", Via: "pull_request"},
		},
		TargetErrors: []ui.TaskError{{Name: "o/d", Err: errors.New("fetch secrets: forbidden (HTTP 403)")}},
		Warnings:     []string{"label_sync is deprecated"},
	}
}

func TestPlanDocument(t *testing.T) {
	doc := testPlanResult().PlanDocument(false)

	if doc.FormatVersion != 1 || doc.Command != "plan" {
		t.Fatalf("header = %d %q", doc.FormatVersion, doc.Command)
	}
	if doc.Summary.Create != 3 || doc.Summary.Update != 3 || doc.Summary.Delete != 1 {
		t.Errorf("summary = %+v, want create 3, update 3, delete 1", doc.Summary)
	}
	if got := len(doc.Changes); got != 7 {
		t.Fatalf("changes = %d, want 7 (noops dropped)", got)
	}

	// Order follows plan output: by target, repository changes before files.
	var order []string
	for _, c := range doc.Changes {
		order = append(order, c.Target+":"+c.Resource)
	}
	want := "o/a:Repository o/a:Repository o/a:Ruleset o/a:Secret o/a:File o/b:Label o/c:File"
	if strings.Join(order, " ") != want {
		t.Errorf("order = %s, want %s", strings.Join(order, " "), want)
	}

	features := doc.Changes[1]
	if len(features.Details) != 1 || features.Details[0].Field != "wiki" || features.Details[0].Before != true || features.Details[0].After != false {
		t.Errorf("features details = %+v", features.Details)
	}

	ruleset := doc.Changes[2]
	if ruleset.Resource != "Ruleset" || ruleset.Name != "main" || ruleset.Field != "" || ruleset.Before != nil {
		t.Errorf("ruleset entry = %+v, want resource Ruleset, name main, no field, no before", ruleset)
	}

	secret := doc.Changes[3]
	if secret.Name != "TOKEN" || secret.After != "(new)" {
		t.Errorf("secret entry = %+v, want only the placeholder", secret)
	}

	file := doc.Changes[4]
	if file.Path != "a.sh" || file.Field != "" || file.Lines == nil || *file.Lines != (Lines{Added: 1, Removed: 1}) {
		t.Errorf("file entry = %+v", file)
	}
	if file.Mode == nil || *file.Mode != (Mode{Before: "100644", After: "100755"}) {
		t.Errorf("file mode = %+v", file.Mode)
	}
	if file.Diff != "" {
		t.Errorf("diff should be empty without showDiff, got %q", file.Diff)
	}
	if doc.Changes[6].Mode != nil {
		t.Errorf("mode should be omitted when it does not change: %+v", doc.Changes[6].Mode)
	}

	if len(doc.Errors) != 1 || doc.Errors[0].Target != "o/d" || !strings.Contains(doc.Errors[0].Message, "HTTP 403") {
		t.Errorf("errors = %+v", doc.Errors)
	}
	if len(doc.Warnings) != 2 || doc.Warnings[0].Message != "label_sync is deprecated" || doc.Warnings[1].Target != "o/a" {
		t.Errorf("warnings = %+v, want the deprecation and the unsigned commit note for o/a", doc.Warnings)
	}
}

func TestPlanDocument_ShowDiff(t *testing.T) {
	doc := testPlanResult().PlanDocument(true)
	if !strings.Contains(doc.Changes[4].Diff, "+y") {
		t.Errorf("diff = %q, want the unified diff", doc.Changes[4].Diff)
	}
}

func TestPlanDocument_EmptyListsEncodeAsArrays(t *testing.T) {
	var buf bytes.Buffer
	if err := (&PlanResult{}).PlanDocument(false).Write(&buf); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"changes", "warnings", "errors"} {
		if _, ok := raw[key].([]any); !ok {
			t.Errorf("%s = %v, want an empty array", key, raw[key])
		}
	}
	summary, _ := raw["summary"].(map[string]any)
	if _, ok := summary["applied"]; ok {
		t.Error("plan summary should not have applied")
	}
}

func TestApplyDocument(t *testing.T) {
	result := testPlanResult()
	outcome := &ApplyOutcome{
		RepoResults: []repository.ApplyResult{
			{Change: result.RepoChanges[0]},
			{Change: result.RepoChanges[2]},
			{Change: result.RepoChanges[3], Err: errors.New("validation failed (HTTP 422)")},
			{Change: result.RepoChanges[4]},
			{Change: result.RepoChanges[5]},
		},
		FileResults: []fileset.ApplyResult{
			{Change: result.FileChanges[0], Skipped: true},
			{Change: result.FileChanges[2], PRURL: "https://github.com/o/c/pull/1"},
		},
	}

	doc := result.ApplyDocument(outcome)
	if doc.Command != "apply" {
		t.Errorf("command = %q", doc.Command)
	}
	statuses := make([]string, len(doc.Changes))
	for i, c := range doc.Changes {
		statuses[i] = c.Status
	}
	want := "applied applied failed applied skipped applied applied"
	if strings.Join(statuses, " ") != want {
		t.Errorf("statuses = %s, want %s", strings.Join(statuses, " "), want)
	}
	if doc.Changes[2].Error != "validation failed (HTTP 422)" {
		t.Errorf("error = %q", doc.Changes[2].Error)
	}
	if doc.Changes[6].PRURL != "https://github.com/o/c/pull/1" {
		t.Errorf("pr_url = %q", doc.Changes[6].PRURL)
	}
	if *doc.Summary.Applied != 5 || *doc.Summary.Failed != 1 || *doc.Summary.Skipped != 1 {
		t.Errorf("summary = applied %d, failed %d, skipped %d", *doc.Summary.Applied, *doc.Summary.Failed, *doc.Summary.Skipped)
	}
}

func TestValidateOutput(t *testing.T) {
	for _, ok := range []string{"", "text", "json"} {
		if err := ValidateOutput(ok); err != nil {
			t.Errorf("ValidateOutput(%q) = %v", ok, err)
		}
	}
	if err := ValidateOutput("yaml"); err == nil {
		t.Error("ValidateOutput(yaml) should fail")
	}
}
