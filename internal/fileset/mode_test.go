package fileset

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

const (
	repoViewKey = "repo view owner/repo --json defaultBranchRef --jq .defaultBranchRef.name"
	headRefKey  = "api repos/owner/repo/git/ref/heads/main --jq .object.sha"
)

func treeKey(treeish string) string {
	return "api repos/owner/repo/git/trees/" + treeish
}

// treeJSON builds a Trees API response from name/mode pairs.
func treeJSON(pairs ...string) []byte {
	var resp struct {
		Tree []map[string]string `json:"tree"`
	}
	for i := 0; i < len(pairs); i += 2 {
		resp.Tree = append(resp.Tree, map[string]string{"path": pairs[i], "mode": pairs[i+1]})
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestPlan_ExecutableMode(t *testing.T) {
	const script = "#!/bin/sh\necho hi\n"
	tests := []struct {
		name        string
		executable  *bool
		remote      string // remote content; empty means the file does not exist
		remoteMode  string
		wantType    ChangeType
		wantMode    string
		wantCurrent string
		wantChanged bool
	}{
		{"sets x bit on unchanged content", new(true), script, ModeFile, ChangeUpdate, ModeExecutable, ModeFile, true},
		{"already executable", new(true), script, ModeExecutable, ChangeNoOp, ModeExecutable, ModeExecutable, false},
		{"content and mode change", new(true), "old\n", ModeFile, ChangeUpdate, ModeExecutable, ModeFile, true},
		{"false clears x bit", new(false), script, ModeExecutable, ChangeUpdate, ModeFile, ModeExecutable, true},
		{"create executable", new(true), "", "", ChangeCreate, ModeExecutable, "", true},
		{"unset keeps the mode", nil, script, ModeExecutable, ChangeNoOp, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{
				Responses: map[string][]byte{
					repoViewKey:            []byte("main"),
					headRefKey:             []byte("head123"),
					treeKey("head123:bin"): treeJSON("tool", tt.remoteMode),
				},
				Errors: map[string]error{},
			}
			if tt.remote == "" {
				mock.Errors[contentsKey("owner/repo", "bin/tool")] = gh.ErrNotFound
			} else {
				mock.Responses[contentsKey("owner/repo", "bin/tool")] = contentsJSON(tt.remote, "blob1")
			}
			p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
			tracker := &recordingTracker{}

			changes, err := p.Plan(context.Background(), makeFileSet("owner", "repo", []manifest.FileEntry{
				{Path: "bin/tool", Content: script, Executable: tt.executable},
			}), "", tracker)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tracker.errors) != 0 {
				t.Fatalf("unexpected tracker errors: %v", tracker.errors)
			}
			if len(changes) != 1 {
				t.Fatalf("expected 1 change, got %d", len(changes))
			}
			c := changes[0]
			if c.Type != tt.wantType || c.Mode != tt.wantMode || c.CurrentMode != tt.wantCurrent {
				t.Errorf("got type=%s mode=%q current=%q, want type=%s mode=%q current=%q",
					c.Type, c.Mode, c.CurrentMode, tt.wantType, tt.wantMode, tt.wantCurrent)
			}
			if c.ModeChanged() != tt.wantChanged {
				t.Errorf("ModeChanged = %v, want %v", c.ModeChanged(), tt.wantChanged)
			}
			if tt.wantType == ChangeUpdate && tt.remote == script && c.Desired != tt.remote {
				t.Errorf("a mode-only change must write the remote content back as is, got %q", c.Desired)
			}
			if tt.executable == nil && countCalls(mock.Called, "repo view") != 0 {
				t.Error("modes must not be looked up when executable is unset")
			}
		})
	}
}

func TestPlan_ExecutableMode_SkipsRepo(t *testing.T) {
	tests := []struct {
		name    string
		head    string // default branch name; empty means an empty repository
		entry   manifest.FileEntry
		mode    string
		wantErr string
	}{
		{"symlink", "main", manifest.FileEntry{Path: "bin/tool", Content: "x", Executable: new(true)}, "120000", "regular files"},
		{"create in empty repo", "", manifest.FileEntry{Path: "bin/tool", Content: "x", Executable: new(true)}, "", "empty repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{
				Responses: map[string][]byte{
					repoViewKey:            []byte(tt.head),
					headRefKey:             []byte("head123"),
					treeKey("head123:bin"): treeJSON("tool", tt.mode),
				},
				Errors: map[string]error{},
			}
			if tt.head == "" {
				mock.Errors[contentsKey("owner/repo", "bin/tool")] = gh.ErrNotFound
			} else {
				mock.Responses[contentsKey("owner/repo", "bin/tool")] = contentsJSON("x", "blob1")
			}
			p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
			tracker := &recordingTracker{}

			changes, err := p.Plan(context.Background(), makeFileSet("owner", "repo", []manifest.FileEntry{tt.entry}), "", tracker)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(changes) != 0 {
				t.Errorf("expected the repo to be skipped, got %+v", changes)
			}
			errs := tracker.errors["owner/repo"]
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tt.wantErr) {
				t.Errorf("expected one error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

func TestPlan_ExecutableMode_CreateOnlyIgnoresExistingMode(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "bin/tool"): contentsJSON("x", "blob1"),
		},
		Errors: map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

	changes, _ := p.Plan(context.Background(), makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "bin/tool", Content: "x", Executable: new(true), Reconcile: manifest.ReconcileCreateOnly},
	}), "", nil)

	if len(changes) != 1 || changes[0].Type != ChangeNoOp {
		t.Fatalf("expected a single NoOp, got %+v", changes)
	}
	if countCalls(mock.Called, "repo view") != 0 {
		t.Error("create_only must not look up the mode of an existing file")
	}
}

const (
	commitsPostKey = "api repos/owner/repo/git/commits --method POST --input - --jq .sha"
	refPatchKey    = "api repos/owner/repo/git/refs/heads/main --method PATCH --input -"
)

// lastTreeRequest returns the entries of the last tree creation request, by path.
func lastTreeRequest(t *testing.T, mock *gh.MockRunner) map[string]map[string]any {
	t.Helper()
	var req struct {
		Tree []map[string]any `json:"tree"`
	}
	for i, c := range flattenCalls(mock.Called) {
		if c == treesPostKey {
			if err := json.Unmarshal(mock.CalledStdin[i], &req); err != nil {
				t.Fatalf("failed to parse tree request: %v", err)
			}
		}
	}
	entries := make(map[string]map[string]any)
	for _, e := range req.Tree {
		path, _ := e["path"].(string)
		entries[path] = e
	}
	return entries
}

func TestApply_ModeChangeUsesGitDataAPI(t *testing.T) {
	mock := newNoopGuardMock("new-tree")
	mock.Responses[treeKey("head123:bin")] = treeJSON("tool", ModeFile)
	mock.Responses[treeKey("head123:scripts")] = treeJSON("run.sh", ModeExecutable)
	mock.Responses[commitsPostKey] = []byte("commit456\n")
	changes := []Change{
		{Target: "owner/repo", Path: "bin/tool", Type: ChangeUpdate, Desired: "#!/bin/sh\n", Mode: ModeExecutable, CurrentMode: ModeFile},
		{Target: "owner/repo", Path: "scripts/run.sh", Type: ChangeUpdate, Desired: "#!/bin/sh\necho\n"},
		{Target: "owner/repo", Path: "README.md", Type: ChangeCreate, Desired: "hi\n"},
	}

	applyChanges(t, mock, changes, ApplyOptions{FileSetID: "test", Via: manifest.ViaPush})

	if n := countCalls(mock.Called, "api graphql"); n != 0 {
		t.Errorf("expected no createCommitOnBranch call, got %d", n)
	}
	if n := countCalls(mock.Called, commitsPostKey); n != 1 {
		t.Errorf("expected one commit creation, got %d", n)
	}
	if n := countCalls(mock.Called, refPatchKey); n != 1 {
		t.Errorf("expected one ref update, got %d", n)
	}
	entries := lastTreeRequest(t, mock)
	for path, want := range map[string]string{
		"bin/tool":       ModeExecutable, // set by executable
		"scripts/run.sh": ModeExecutable, // keeps its x bit
		"README.md":      ModeFile,       // new file
	} {
		if got := entries[path]["mode"]; got != want {
			t.Errorf("%s: mode = %v, want %s", path, got, want)
		}
	}
	for i, c := range flattenCalls(mock.Called) {
		if c == refPatchKey && !strings.Contains(string(mock.CalledStdin[i]), "commit456") {
			t.Errorf("ref must move to the new commit, got %s", mock.CalledStdin[i])
		}
	}
}

func TestApply_NoModeChangeUsesGraphQL(t *testing.T) {
	mock := newNoopGuardMock("new-tree")
	changes := []Change{
		{Target: "owner/repo", Path: "bin/tool", Type: ChangeUpdate, Desired: "#!/bin/sh\n", Mode: ModeExecutable, CurrentMode: ModeExecutable},
	}

	applyChanges(t, mock, changes, ApplyOptions{FileSetID: "test", Via: manifest.ViaPush})

	if n := countCalls(mock.Called, "api graphql"); n != 1 {
		t.Errorf("expected one createCommitOnBranch call, got %d", n)
	}
	if n := countCalls(mock.Called, commitsPostKey); n != 0 {
		t.Errorf("expected no Git Data API commit, got %d", n)
	}
	if n := countCalls(mock.Called, treeKey("")); n != 0 {
		t.Errorf("expected no tree lookups, got %d", n)
	}
}

func TestApply_ModeChangeInEmptyRepoFails(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{repoViewKey: []byte("")},
		Errors:    map[string]error{},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	changes := []Change{
		{Target: "owner/repo", Path: "bin/tool", Type: ChangeCreate, Desired: "#!/bin/sh\n", Mode: ModeExecutable},
	}

	results := p.Apply(context.Background(), changes, ApplyOptions{FileSetID: "test", Via: manifest.ViaPush}, ui.NoopReporter{})

	if len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "empty repository") {
		t.Fatalf("expected an empty repository error, got %+v", results)
	}
	if n := countCalls(mock.Called, "api repos/owner/repo/contents"); n != 0 {
		t.Errorf("expected no Contents API write, got %d", n)
	}
}
