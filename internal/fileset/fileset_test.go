package fileset

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/output"
)

func init() {
	output.DisableStyles()
}

// helper: build a GitHub Contents API JSON response
func contentsJSON(content, sha string) []byte {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	resp := struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}{
		Content:  encoded,
		Encoding: "base64",
		SHA:      sha,
	}
	b, _ := json.Marshal(resp)
	return b
}

// helper: build a mock key for the contents API fetch call
func contentsKey(repo, path string) string {
	return fmt.Sprintf("api repos/%s/contents/%s", repo, path)
}

func makeFileSet(name, repo, onDrift string, files []manifest.FileEntry) []*manifest.FileSet {
	return []*manifest.FileSet{
		{
			Metadata: manifest.FileSetMetadata{Name: name},
			Spec: manifest.FileSetSpec{
				Targets: []manifest.FileSetTarget{{Name: repo}},
				Files:   files,
				OnDrift: onDrift,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Plan tests
// ---------------------------------------------------------------------------

func TestPlan_NewFile(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors: map[string]error{
			contentsKey("owner/repo", ".github/ci.yml"): gh.ErrNotFound,
		},
	}
	p := NewProcessor(mock)
	fileSets := makeFileSet("ci-files", "owner/repo", "warn", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "name: CI"},
	})

	changes := p.Plan(fileSets)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != FileCreate {
		t.Errorf("expected FileCreate, got %s", changes[0].Type)
	}
	if changes[0].Desired != "name: CI" {
		t.Errorf("unexpected desired content: %q", changes[0].Desired)
	}
}

func TestPlan_NoChange(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", ".github/ci.yml"): contentsJSON("name: CI", "abc123"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock)
	fileSets := makeFileSet("ci-files", "owner/repo", "warn", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "name: CI"},
	})

	changes := p.Plan(fileSets)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != FileNoOp {
		t.Errorf("expected FileNoOp, got %s", changes[0].Type)
	}
}

func TestPlan_DriftWarn(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", ".github/ci.yml"): contentsJSON("old content", "sha1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock)
	fileSets := makeFileSet("ci-files", "owner/repo", "warn", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "new content"},
	})

	changes := p.Plan(fileSets)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.Type != FileDrift {
		t.Errorf("expected FileDrift, got %s", c.Type)
	}
	if !c.Drifted {
		t.Error("expected Drifted=true")
	}
	if c.SHA != "sha1" {
		t.Errorf("expected SHA=sha1, got %s", c.SHA)
	}
}

func TestPlan_DriftOverwrite(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", ".github/ci.yml"): contentsJSON("old content", "sha1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock)
	fileSets := makeFileSet("ci-files", "owner/repo", "overwrite", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "new content"},
	})

	changes := p.Plan(fileSets)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.Type != FileUpdate {
		t.Errorf("expected FileUpdate, got %s", c.Type)
	}
	if !c.Drifted {
		t.Error("expected Drifted=true")
	}
}

func TestPlan_DriftSkip(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", ".github/ci.yml"): contentsJSON("old content", "sha1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock)
	fileSets := makeFileSet("ci-files", "owner/repo", "skip", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "new content"},
	})

	changes := p.Plan(fileSets)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.Type != FileSkip {
		t.Errorf("expected FileSkip, got %s", c.Type)
	}
	if !c.Drifted {
		t.Error("expected Drifted=true")
	}
}

// ---------------------------------------------------------------------------
// Apply tests
// ---------------------------------------------------------------------------

func TestApply_CreateFile(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock)

	content := "name: CI"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	// Pre-register the expected PUT call
	putKey := fmt.Sprintf("api repos/owner/repo/contents/.github/ci.yml --method PUT -f message=chore: add .github/ci.yml via gh-infra -f content=%s", encoded)
	mock.Responses[putKey] = []byte(`{}`)

	changes := []FileChange{
		{
			FileSet: "ci-files",
			Target:  "owner/repo",
			Path:    ".github/ci.yml",
			Type:    FileCreate,
			Desired: content,
		},
	}

	results := p.Apply(changes)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
	// Verify the mock was called
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	// Check args include PUT and content
	args := mock.Called[0]
	foundPUT := false
	foundContent := false
	for _, a := range args {
		if a == "PUT" {
			foundPUT = true
		}
		if a == fmt.Sprintf("content=%s", encoded) {
			foundContent = true
		}
	}
	if !foundPUT {
		t.Error("expected PUT method in call args")
	}
	if !foundContent {
		t.Error("expected base64 content in call args")
	}
}

func TestApply_UpdateFile(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock)

	content := "name: CI v2"
	sha := "abc123"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	putKey := fmt.Sprintf("api repos/owner/repo/contents/.github/ci.yml --method PUT -f message=chore: update .github/ci.yml via gh-infra -f content=%s -f sha=%s", encoded, sha)
	mock.Responses[putKey] = []byte(`{}`)

	changes := []FileChange{
		{
			FileSet: "ci-files",
			Target:  "owner/repo",
			Path:    ".github/ci.yml",
			Type:    FileUpdate,
			Desired: content,
			SHA:     sha,
		},
	}

	results := p.Apply(changes)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
	// Verify SHA was passed
	if len(mock.Called) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.Called))
	}
	args := mock.Called[0]
	foundSHA := false
	for _, a := range args {
		if a == fmt.Sprintf("sha=%s", sha) {
			foundSHA = true
		}
	}
	if !foundSHA {
		t.Error("expected SHA in call args")
	}
}

func TestApply_DriftWarnSkipsApply(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock)

	changes := []FileChange{
		{
			FileSet: "ci-files",
			Target:  "owner/repo",
			Path:    ".github/ci.yml",
			Type:    FileDrift,
			OnDrift: "warn",
			Drifted: true,
		},
	}

	results := p.Apply(changes)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Error("expected Skipped=true for drift/warn")
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected no runner calls, got %d", len(mock.Called))
	}
}

func TestApply_NoOpAndSkipNotApplied(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock)

	changes := []FileChange{
		{Type: FileNoOp, Target: "owner/repo", Path: "a.txt"},
		{Type: FileSkip, Target: "owner/repo", Path: "b.txt"},
	}

	results := p.Apply(changes)

	if len(results) != 0 {
		t.Errorf("expected 0 results for noop/skip, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected no runner calls, got %d", len(mock.Called))
	}
}

// ---------------------------------------------------------------------------
// HasChanges tests
// ---------------------------------------------------------------------------

func TestHasChanges_AllNoOpAndSkip(t *testing.T) {
	changes := []FileChange{
		{Type: FileNoOp},
		{Type: FileSkip},
		{Type: FileNoOp},
	}
	if HasChanges(changes) {
		t.Error("expected HasChanges=false for all noop/skip")
	}
}

func TestHasChanges_WithCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name    string
		changes []FileChange
		want    bool
	}{
		{
			name:    "with create",
			changes: []FileChange{{Type: FileNoOp}, {Type: FileCreate}},
			want:    true,
		},
		{
			name:    "with update",
			changes: []FileChange{{Type: FileUpdate}},
			want:    true,
		},
		{
			name:    "with drift",
			changes: []FileChange{{Type: FileDrift}},
			want:    true,
		},
		{
			name:    "empty",
			changes: []FileChange{},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasChanges(tt.changes)
			if got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CountChanges tests
// ---------------------------------------------------------------------------

func TestCountChanges(t *testing.T) {
	changes := []FileChange{
		{Type: FileCreate},
		{Type: FileCreate},
		{Type: FileUpdate},
		{Type: FileDrift},
		{Type: FileDrift},
		{Type: FileDrift},
		{Type: FileNoOp},
		{Type: FileSkip},
	}

	creates, updates, drifts := CountChanges(changes)

	if creates != 2 {
		t.Errorf("creates: got %d, want 2", creates)
	}
	if updates != 1 {
		t.Errorf("updates: got %d, want 1", updates)
	}
	if drifts != 3 {
		t.Errorf("drifts: got %d, want 3", drifts)
	}
}

// ---------------------------------------------------------------------------
// PrintPlan tests
// ---------------------------------------------------------------------------

func TestPrintPlan(t *testing.T) {
	var buf bytes.Buffer
	changes := []FileChange{
		{FileSet: "ci", Target: "org/repo-a", Path: ".github/ci.yml", Type: FileCreate},
		{FileSet: "ci", Target: "org/repo-a", Path: ".github/lint.yml", Type: FileUpdate},
		{FileSet: "ci", Target: "org/repo-b", Path: ".github/ci.yml", Type: FileDrift, OnDrift: "warn"},
		{FileSet: "ci", Target: "org/repo-b", Path: ".github/skip.yml", Type: FileSkip, OnDrift: "skip"},
		{FileSet: "ci", Target: "org/repo-c", Path: ".github/ci.yml", Type: FileNoOp},
	}

	PrintPlan(&buf, changes)
	output := buf.String()

	// Check create line
	if !strings.Contains(output, "+ .github/ci.yml") {
		t.Errorf("expected create marker for ci.yml, got:\n%s", output)
	}
	if !strings.Contains(output, "(new file)") {
		t.Errorf("expected '(new file)' in output, got:\n%s", output)
	}
	// Check update line
	if !strings.Contains(output, "~ .github/lint.yml") {
		t.Errorf("expected update marker for lint.yml, got:\n%s", output)
	}
	if !strings.Contains(output, "(content changed)") {
		t.Errorf("expected '(content changed)' in output, got:\n%s", output)
	}
	// Check drift line
	if !strings.Contains(output, "drift detected") {
		t.Errorf("expected 'drift detected' in output, got:\n%s", output)
	}
	// Check skip line
	if !strings.Contains(output, "skip") {
		t.Errorf("expected 'skip' in output, got:\n%s", output)
	}
	// Check grouping headers
	if !strings.Contains(output, "org/repo-a") {
		t.Errorf("expected group header org/repo-a, got:\n%s", output)
	}
	if !strings.Contains(output, "org/repo-b") {
		t.Errorf("expected group header org/repo-b, got:\n%s", output)
	}
}

func TestPrintPlan_AllNoOp(t *testing.T) {
	var buf bytes.Buffer
	changes := []FileChange{
		{FileSet: "ci", Target: "org/repo", Path: "a.txt", Type: FileNoOp},
	}

	PrintPlan(&buf, changes)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for all no-op, got:\n%s", buf.String())
	}
}

func TestPrintPlan_Empty(t *testing.T) {
	var buf bytes.Buffer
	PrintPlan(&buf, []FileChange{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty changes, got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// PrintApplyResults tests
// ---------------------------------------------------------------------------

func TestPrintApplyResults(t *testing.T) {
	var buf bytes.Buffer
	results := []FileApplyResult{
		{
			Change: FileChange{Target: "org/repo", Path: "a.txt", Type: FileCreate},
			Err:    nil,
		},
		{
			Change: FileChange{Target: "org/repo", Path: "b.txt", Type: FileUpdate},
			Err:    fmt.Errorf("permission denied"),
		},
		{
			Change:  FileChange{Target: "org/repo", Path: "c.txt", OnDrift: "warn"},
			Skipped: true,
		},
	}

	PrintApplyResults(&buf, results)
	output := buf.String()

	if !strings.Contains(output, "✓") {
		t.Errorf("expected success marker in output, got:\n%s", output)
	}
	if !strings.Contains(output, "✗") {
		t.Errorf("expected error marker in output, got:\n%s", output)
	}
	if !strings.Contains(output, "permission denied") {
		t.Errorf("expected error message in output, got:\n%s", output)
	}
	if !strings.Contains(output, "⚠") {
		t.Errorf("expected skip/drift marker in output, got:\n%s", output)
	}
	if !strings.Contains(output, "skipped") {
		t.Errorf("expected 'skipped' in output, got:\n%s", output)
	}
}

// ---------------------------------------------------------------------------
// PrintSummary tests
// ---------------------------------------------------------------------------

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	results := []FileApplyResult{
		{Change: FileChange{}, Err: nil},
		{Change: FileChange{}, Err: nil},
		{Change: FileChange{}, Err: fmt.Errorf("error")},
		{Change: FileChange{}, Skipped: true},
	}

	PrintSummary(&buf, results)
	output := buf.String()

	if !strings.Contains(output, "2 changes applied") {
		t.Errorf("expected '2 changes applied', got:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Errorf("expected '1 failed', got:\n%s", output)
	}
	if !strings.Contains(output, "1 skipped") {
		t.Errorf("expected '1 skipped', got:\n%s", output)
	}
}

func TestPrintSummary_AllSuccess(t *testing.T) {
	var buf bytes.Buffer
	results := []FileApplyResult{
		{Change: FileChange{}, Err: nil},
		{Change: FileChange{}, Err: nil},
	}

	PrintSummary(&buf, results)
	output := buf.String()

	if !strings.Contains(output, "2 changes applied") {
		t.Errorf("expected '2 changes applied', got:\n%s", output)
	}
	// Should not contain "failed" or "skipped"
	if strings.Contains(output, "failed") {
		t.Errorf("should not contain 'failed', got:\n%s", output)
	}
	if strings.Contains(output, "skipped") {
		t.Errorf("should not contain 'skipped', got:\n%s", output)
	}
}
