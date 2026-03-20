package fileset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

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
