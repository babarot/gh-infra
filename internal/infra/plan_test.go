package infra

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/ui"
)

func TestPlanResult_Printer_WithEngine(t *testing.T) {
	p := ui.NewStandardPrinter()
	r := &PlanResult{
		engine: &engine{printer: p},
	}
	if r.Printer() != p {
		t.Error("expected Printer() to return engine's printer")
	}
}

func TestPlanResult_Printer_NilEngine(t *testing.T) {
	r := &PlanResult{}
	p := r.Printer()
	if p == nil {
		t.Fatal("expected non-nil fallback printer")
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"no duplicates", []string{"a", "b", "c"}, 3},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, 3},
		{"all same", []string{"x", "x", "x"}, 1},
		{"empty", []string{}, 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueStrings(tt.in)
			if len(got) != tt.want {
				t.Errorf("uniqueStrings(%v) = %d items, want %d", tt.in, len(got), tt.want)
			}
		})
	}
}

func TestUniqueStrings_PreservesOrder(t *testing.T) {
	got := uniqueStrings([]string{"c", "a", "b", "a", "c"})
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFailedTargets(t *testing.T) {
	errs := []ui.TaskError{
		{Name: "owner/b", Err: errors.New("fetch secrets: HTTP 403")},
		{Name: "owner/a", Err: errors.New("fetch variables: HTTP 403")},
		{Name: "owner/b", Err: errors.New("fetch files: HTTP 403")},
	}
	got := failedTargets(errs)
	want := []string{"owner/b", "owner/a"}
	if len(got) != len(want) {
		t.Fatalf("failedTargets() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := failedTargets(nil); len(got) != 0 {
		t.Errorf("failedTargets(nil) = %v, want empty", got)
	}
}

func TestPlanResult_FailedTargetsError(t *testing.T) {
	var nilResult *PlanResult
	if err := nilResult.FailedTargetsError(); err != nil {
		t.Errorf("nil result: got %v, want nil", err)
	}
	if err := (&PlanResult{}).FailedTargetsError(); err != nil {
		t.Errorf("no failures: got %v, want nil", err)
	}

	r := &PlanResult{FailedTargets: []string{"owner/a", "owner/b"}}
	err := r.FailedTargetsError()
	if err == nil {
		t.Fatal("expected an error when targets failed")
	}
	for _, s := range []string{"2 repositories", "owner/a", "owner/b"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not contain %q", err, s)
		}
	}
}

// Plan (and so apply) rejects FileSets that would overwrite each other's PR
// branch before fetching anything from GitHub.
func TestPlan_RejectsSharedPRBranch(t *testing.T) {
	doc := `apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: a.txt
      content: a
  via: pull_request
`
	path := filepath.Join(t.TempDir(), "files.yaml")
	if err := os.WriteFile(path, []byte(doc+"---\n"+doc), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Plan(PlanOptions{Paths: []string{path}, DryRun: true})

	if err == nil || !strings.Contains(err.Error(), `from branch "gh-infra/sync-org-repo"`) {
		t.Fatalf("expected shared PR branch error, got %v", err)
	}
}
