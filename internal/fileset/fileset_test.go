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
	"github.com/babarot/gh-infra/internal/ui"
)

func init() {
	ui.DisableStyles()
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

func makeFileSet(owner, repo string, files []manifest.FileEntry) []*manifest.FileSetDocument {
	resolved := make([]manifest.ResolvedFile, len(files))
	for i, file := range files {
		resolved[i] = manifest.ResolvedFile{
			FileEntry: file,
			Origin: manifest.FileOrigin{
				Kind:      manifest.FileOriginSpecFiles,
				FileIndex: i,
			},
		}
	}
	return []*manifest.FileSetDocument{
		{
			Resource: &manifest.FileSet{
				Metadata: manifest.FileSetMetadata{Owner: owner},
				Spec: manifest.FileSetSpec{
					Repositories: []manifest.FileSetRepository{{Name: repo}},
					Files:        files,
				},
			},
			ResolvedFiles: resolved,
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
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "name: CI"},
	})

	changes, _ := p.Plan(fileSets, "", nil)

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
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "name: CI"},
	})

	changes, _ := p.Plan(fileSets, "", nil)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != FileNoOp {
		t.Errorf("expected FileNoOp, got %s", changes[0].Type)
	}
}

func TestPlan_ContentDiffers(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", ".github/ci.yml"): contentsJSON("old content", "sha1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: ".github/ci.yml", Content: "new content"},
	})

	changes, _ := p.Plan(fileSets, "", nil)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	c := changes[0]
	if c.Type != FileUpdate {
		t.Errorf("expected FileUpdate, got %s", c.Type)
	}
	if c.SHA != "sha1" {
		t.Errorf("expected SHA=sha1, got %s", c.SHA)
	}
}

func TestPlan_CreateOnly_FileNotExists(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors: map[string]error{
			contentsKey("owner/repo", "VERSION"): gh.ErrNotFound,
		},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "VERSION", Content: "0.1.0", Reconcile: manifest.ReconcileCreateOnly},
	})

	changes, _ := p.Plan(fileSets, "", nil)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != FileCreate {
		t.Errorf("expected FileCreate, got %s", changes[0].Type)
	}
}

func TestPlan_CreateOnly_FileExists(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "VERSION"): contentsJSON("0.2.0", "sha1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "VERSION", Content: "0.1.0", Reconcile: manifest.ReconcileCreateOnly},
	})

	changes, _ := p.Plan(fileSets, "", nil)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != FileNoOp {
		t.Errorf("expected FileNoOp (file exists, create_only ignores), got %s", changes[0].Type)
	}
}

func TestPlan_MultipleFilesDiffer(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "a.txt"): contentsJSON("old a", "sha-a"),
			contentsKey("owner/repo", "b.txt"): contentsJSON("old b", "sha-b"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "a.txt", Content: "new a"},
		{Path: "b.txt", Content: "new b"},
	})

	changes, err := p.Plan(fileSets, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}

	// Both files differ → FileUpdate
	if changes[0].Type != FileUpdate {
		t.Errorf("a.txt: expected FileUpdate, got %s", changes[0].Type)
	}
	if changes[1].Type != FileUpdate {
		t.Errorf("b.txt: expected FileUpdate, got %s", changes[1].Type)
	}
}

// ---------------------------------------------------------------------------
// Apply tests
// ---------------------------------------------------------------------------

// WildcardMockRunner extends MockRunner to return a default response for unmatched keys.
type WildcardMockRunner struct {
	gh.MockRunner
	DefaultResponse []byte
}

func (m *WildcardMockRunner) Run(args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	m.Called = append(m.Called, args)
	if err, ok := m.Errors[key]; ok {
		return nil, err
	}
	if resp, ok := m.Responses[key]; ok {
		return resp, nil
	}
	// Return default response for unmatched calls (Git Data API calls with dynamic args)
	return m.DefaultResponse, nil
}

// setupGitDataAPIMock creates a WildcardMockRunner with Git Data API responses.
func setupGitDataAPIMock(repo string) *WildcardMockRunner {
	mock := &WildcardMockRunner{
		MockRunner: gh.MockRunner{
			Responses: map[string][]byte{
				// Get default branch
				fmt.Sprintf("repo view %s --json defaultBranchRef --jq .defaultBranchRef.name", repo): []byte("main"),
				// Get HEAD SHA
				fmt.Sprintf("api repos/%s/git/ref/heads/main --jq .object.sha", repo): []byte("head123"),
			},
			Errors: map[string]error{},
		},
		// Default response for blob/tree/commit/ref calls
		DefaultResponse: []byte(`{"sha":"mock-sha-123"}`),
	}
	return mock
}

func TestApply_CreateFile(t *testing.T) {
	mock := setupGitDataAPIMock("owner/repo")
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	changes := []FileApplyChange{
		{
			Target: "owner/repo", Path: ".github/ci.yml", Type: FileCreate, Desired: "name: CI",
			FileSet: "ci-files",
		},
	}

	results := p.Apply(changes, ApplyOptions{FileSetName: "test"}, ui.NoopReporter{})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}

	// Verify Git Data API calls were made: get branch, get HEAD, blob, tree, commit, ref update
	callLog := strings.Join(flattenCalls(mock.Called), " | ")
	for _, expected := range []string{"git/ref/heads/main", "git/blobs", "git/trees", "git/commits", "git/refs/heads/main"} {
		if !strings.Contains(callLog, expected) {
			t.Errorf("expected call containing %q, got: %s", expected, callLog)
		}
	}
}

func TestApply_UpdateFile(t *testing.T) {
	mock := setupGitDataAPIMock("owner/repo")
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	changes := []FileApplyChange{
		{
			Target: "owner/repo", Path: ".github/ci.yml", Type: FileUpdate, Desired: "name: CI v2",
			FileSet: "ci-files", SHA: "old123",
		},
	}

	results := p.Apply(changes, ApplyOptions{FileSetName: "test"}, ui.NoopReporter{})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
}

func flattenCalls(calls [][]string) []string {
	var flat []string
	for _, call := range calls {
		flat = append(flat, strings.Join(call, " "))
	}
	return flat
}

func TestApply_NoOpNotApplied(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	changes := []FileApplyChange{
		{Type: FileNoOp, Target: "owner/repo", Path: "a.txt"},
		{Type: FileNoOp, Target: "owner/repo", Path: "b.txt"},
	}

	results := p.Apply(changes, ApplyOptions{FileSetName: "test"}, ui.NoopReporter{})

	if len(results) != 0 {
		t.Errorf("expected 0 results for noop, got %d", len(results))
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected no runner calls, got %d", len(mock.Called))
	}
}

// ---------------------------------------------------------------------------
// HasChanges tests
// ---------------------------------------------------------------------------

func TestHasChanges_AllNoOp(t *testing.T) {
	changes := []FileApplyChange{
		{Type: FileNoOp},
		{Type: FileNoOp},
		{Type: FileNoOp},
	}
	if HasChanges(changes) {
		t.Error("expected HasChanges=false for all noop")
	}
}

func TestHasChanges_WithCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name    string
		changes []FileApplyChange
		want    bool
	}{
		{
			name:    "with create",
			changes: []FileApplyChange{{Type: FileNoOp}, {Type: FileCreate}},
			want:    true,
		},
		{
			name:    "with update",
			changes: []FileApplyChange{{Type: FileUpdate}},
			want:    true,
		},
		{
			name:    "empty",
			changes: []FileApplyChange{},
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
	changes := []FileApplyChange{
		{Type: FileCreate},
		{Type: FileCreate},
		{Type: FileUpdate},
		{Type: FileNoOp},
	}

	creates, updates, deletes := CountChanges(changes)

	if creates != 2 {
		t.Errorf("creates: got %d, want 2", creates)
	}
	if updates != 1 {
		t.Errorf("updates: got %d, want 1", updates)
	}
	if deletes != 0 {
		t.Errorf("deletes: got %d, want 0", deletes)
	}
}

// ---------------------------------------------------------------------------
// PrintPlan tests
// ---------------------------------------------------------------------------

func TestPrintPlan(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	changes := []FileApplyChange{
		{Target: "org/repo-a", Path: ".github/ci.yml", Type: FileCreate, FileSet: "ci"},
		{Target: "org/repo-a", Path: ".github/lint.yml", Type: FileUpdate, FileSet: "ci"},
		{Target: "org/repo-b", Path: ".github/ci.yml", Type: FileDelete, FileSet: "ci"},
		{Target: "org/repo-c", Path: ".github/ci.yml", Type: FileNoOp, FileSet: "ci"},
	}

	PrintPlan(p, changes)
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
	p := ui.NewStandardPrinterWith(&buf, &buf)

	changes := []FileApplyChange{
		{Target: "org/repo", Path: "a.txt", Type: FileNoOp, FileSet: "ci"},
	}

	PrintPlan(p, changes)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for all no-op, got:\n%s", buf.String())
	}
}

func TestPrintPlan_Empty(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	PrintPlan(p, []FileApplyChange{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty changes, got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// PrintApplyResults tests
// ---------------------------------------------------------------------------

func TestPrintApplyResults(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	results := []FileApplyResult{
		{
			Change: FileApplyChange{Target: "org/repo", Path: "a.txt", Type: FileCreate},
			Err:    nil,
		},
		{
			Change: FileApplyChange{Target: "org/repo", Path: "b.txt", Type: FileUpdate},
			Err:    fmt.Errorf("permission denied"),
		},
	}

	PrintApplyResults(p, results)
	output := buf.String()

	if !strings.Contains(output, "a.txt") {
		t.Errorf("expected success entry in output, got:\n%s", output)
	}
	if !strings.Contains(output, "permission denied") {
		t.Errorf("expected error message in output, got:\n%s", output)
	}
}

// ---------------------------------------------------------------------------
// PrintSummary tests
// ---------------------------------------------------------------------------

func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	results := []FileApplyResult{
		{Change: FileApplyChange{}, Err: nil},
		{Change: FileApplyChange{}, Err: nil},
		{Change: FileApplyChange{}, Err: fmt.Errorf("error")},
	}

	PrintSummary(p, results)
	output := buf.String()

	if !strings.Contains(output, "2 changes applied") {
		t.Errorf("expected '2 changes applied', got:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Errorf("expected '1 failed', got:\n%s", output)
	}
}

func TestPrintSummary_AllSuccess(t *testing.T) {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	results := []FileApplyResult{
		{Change: FileApplyChange{}, Err: nil},
		{Change: FileApplyChange{}, Err: nil},
	}

	PrintSummary(p, results)
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

// ---------------------------------------------------------------------------
// Mirror mode tests
// ---------------------------------------------------------------------------

// dirContentsJSON builds a GitHub Contents API JSON response for a directory listing.
func dirContentsJSON(files []struct{ Path, Type string }) []byte {
	type item struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	var items []item
	for _, f := range files {
		items = append(items, item{Path: f.Path, Type: f.Type})
	}
	b, _ := json.Marshal(items)
	return b
}

func TestPlan_MirrorDetectsOrphans(t *testing.T) {
	// file1.yml is declared in YAML, file2.yml is NOT → file2.yml should be FileDelete
	dirFiles := []struct{ Path, Type string }{
		{Path: "config/file1.yml", Type: "file"},
		{Path: "config/file2.yml", Type: "file"},
	}

	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			// file1.yml exists in repo with same content → NoOp
			contentsKey("owner/repo", "config/file1.yml"): contentsJSON("content1", "sha1"),
			// directory listing for mirror orphan detection
			contentsKey("owner/repo", "config"): dirContentsJSON(dirFiles),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	fileSets := []*manifest.FileSetDocument{
		{
			Resource: &manifest.FileSet{
				Metadata: manifest.FileSetMetadata{Owner: "owner"},
				Spec: manifest.FileSetSpec{
					Repositories: []manifest.FileSetRepository{{Name: "repo"}},
					Files: []manifest.FileEntry{
						{
							Path:      "config/file1.yml",
							Content:   "content1",
							Reconcile: manifest.ReconcileMirror,
							DirScope:  "config",
						},
					},
				},
			},
			ResolvedFiles: []manifest.ResolvedFile{
				{FileEntry: manifest.FileEntry{
					Path:      "config/file1.yml",
					Content:   "content1",
					Reconcile: manifest.ReconcileMirror,
					DirScope:  "config",
				}},
			},
		},
	}

	changes, err := p.Plan(fileSets, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 2 changes: NoOp for file1.yml, Delete for file2.yml
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(changes), changes)
	}

	var foundDelete bool
	for _, c := range changes {
		if c.Path == "config/file2.yml" && c.Type == FileDelete {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Errorf("expected FileDelete for config/file2.yml, changes: %+v", changes)
	}
}

func TestPlan_PatchIgnoresOrphans(t *testing.T) {
	// Same setup but with patch mode — no deletes should be generated
	dirFiles := []struct{ Path, Type string }{
		{Path: "config/file1.yml", Type: "file"},
		{Path: "config/file2.yml", Type: "file"},
	}

	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "config/file1.yml"): contentsJSON("content1", "sha1"),
			// directory listing should NOT be called for patch mode, but include it to be safe
			contentsKey("owner/repo", "config"): dirContentsJSON(dirFiles),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	fileSets := []*manifest.FileSetDocument{
		{
			Resource: &manifest.FileSet{
				Metadata: manifest.FileSetMetadata{Owner: "owner"},
				Spec: manifest.FileSetSpec{
					Repositories: []manifest.FileSetRepository{{Name: "repo"}},
					Files: []manifest.FileEntry{
						{
							Path:      "config/file1.yml",
							Content:   "content1",
							Reconcile: manifest.ReconcilePatch,
							DirScope:  "config",
						},
					},
				},
			},
			ResolvedFiles: []manifest.ResolvedFile{
				{FileEntry: manifest.FileEntry{
					Path:      "config/file1.yml",
					Content:   "content1",
					Reconcile: manifest.ReconcilePatch,
					DirScope:  "config",
				}},
			},
		},
	}

	changes, err := p.Plan(fileSets, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range changes {
		if c.Type == FileDelete {
			t.Errorf("patch mode should not generate FileDelete, got delete for %s", c.Path)
		}
	}
}

func TestCountChanges_WithDeletes(t *testing.T) {
	changes := []FileApplyChange{
		{Type: FileCreate},
		{Type: FileUpdate},
		{Type: FileDelete},
		{Type: FileDelete},
		{Type: FileNoOp},
	}

	creates, updates, deletes := CountChanges(changes)

	if creates != 1 {
		t.Errorf("creates: got %d, want 1", creates)
	}
	if updates != 1 {
		t.Errorf("updates: got %d, want 1", updates)
	}
	if deletes != 2 {
		t.Errorf("deletes: got %d, want 2", deletes)
	}
}

func TestHasChanges_FileDelete(t *testing.T) {
	changes := []FileApplyChange{
		{Type: FileNoOp},
		{Type: FileDelete},
	}
	if !HasChanges(changes) {
		t.Error("expected HasChanges=true when FileDelete is present")
	}
}

func TestHasChanges_OnlyDeletes(t *testing.T) {
	changes := []FileApplyChange{
		{Type: FileDelete},
	}
	if !HasChanges(changes) {
		t.Error("expected HasChanges=true when only FileDelete changes exist")
	}
}
