package fileset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

func repoViewKeyOf(repo string) string {
	return fmt.Sprintf("repo view %s --json defaultBranchRef --jq .defaultBranchRef.name", repo)
}

// A pull_request FileSet for a repository with no commits is planned as a
// push, since apply cannot open a pull request there; other targets of the
// same FileSet keep pull_request.
func TestPlan_PullRequestToEmptyRepoIsPush(t *testing.T) {
	mock := &gh.MockRunner{
		Strict: true,
		Responses: map[string][]byte{
			repoViewKeyOf("owner/empty"): []byte(""),
			repoViewKeyOf("owner/full"):  []byte("main"),
		},
		Errors: map[string]error{
			contentsKey("owner/empty", "README.md"): gh.ErrNotFound,
			contentsKey("owner/full", "README.md"):  gh.ErrNotFound,
		},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fs := makeFileSet("owner", "empty", []manifest.FileEntry{{Path: "README.md", Content: "hi"}})
	fs[0].Spec.Repositories = append(fs[0].Spec.Repositories, manifest.FileSetRepository{Name: "full"})
	fs[0].Spec.Via = manifest.ViaPullRequest
	tracker := &recordingTracker{}

	changes, err := p.Plan(context.Background(), fs, "", tracker)
	if err != nil || len(tracker.errors) != 0 {
		t.Fatalf("err = %v, tracker errors = %v", err, tracker.errors)
	}
	got := map[string]Change{}
	for _, c := range changes {
		got[c.Target] = c
	}
	if c := got["owner/empty"]; c.Via != manifest.ViaPush || !c.NoCommits {
		t.Errorf("empty repo: Via = %q, NoCommits = %v, want push and true", c.Via, c.NoCommits)
	}
	if c := got["owner/full"]; c.Via != manifest.ViaPullRequest || c.NoCommits {
		t.Errorf("repo with commits: Via = %q, NoCommits = %v, want pull_request and false", c.Via, c.NoCommits)
	}
}

func TestPlan_ViaLookup(t *testing.T) {
	tests := []struct {
		name       string
		via        string
		remote     string // remote content; empty means the file does not exist
		executable *bool
		viewErr    error // error of repo view
		wantViews  int
		wantVia    string
		wantErr    bool
	}{
		{"push does not look", manifest.ViaPush, "", nil, nil, 0, manifest.ViaPush, false},
		{"no changes do not look", manifest.ViaPullRequest, "hi", nil, nil, 0, manifest.ViaPullRequest, false},
		{"executable looks once", manifest.ViaPullRequest, "", new(false), nil, 1, manifest.ViaPush, false},
		{"missing repository is left to the repository plan", manifest.ViaPullRequest, "",
			nil, errors.New("GraphQL: Could not resolve to a Repository with the name 'owner/repo'. (repository)"), 1, manifest.ViaPullRequest, false},
		{"unreadable repository fails", manifest.ViaPullRequest, "", nil, gh.ErrForbidden, 1, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{
				Strict:    true,
				Responses: map[string][]byte{repoViewKey: []byte("")},
				Errors:    map[string]error{},
			}
			if tt.viewErr != nil {
				mock.Errors[repoViewKey] = tt.viewErr
			}
			if tt.remote == "" {
				mock.Errors[contentsKey("owner/repo", "README.md")] = gh.ErrNotFound
			} else {
				mock.Responses[contentsKey("owner/repo", "README.md")] = contentsJSON(tt.remote, "blob1")
			}
			p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
			fs := makeFileSet("owner", "repo", []manifest.FileEntry{{Path: "README.md", Content: "hi", Executable: tt.executable}})
			fs[0].Spec.Via = tt.via
			tracker := &recordingTracker{}

			changes, err := p.Plan(context.Background(), fs, "", tracker)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n := countCalls(mock.Called, "repo view"); n != tt.wantViews {
				t.Errorf("repo view calls = %d, want %d", n, tt.wantViews)
			}
			if tt.wantErr {
				if len(changes) != 0 || len(tracker.errors["owner/repo"]) != 1 {
					t.Errorf("want the repo skipped with an error, got changes %+v, errors %v", changes, tracker.errors)
				}
				return
			}
			if len(tracker.errors) != 0 || len(changes) != 1 {
				t.Fatalf("changes = %+v, errors = %v", changes, tracker.errors)
			}
			if changes[0].Via != tt.wantVia {
				t.Errorf("Via = %q, want %q", changes[0].Via, tt.wantVia)
			}
		})
	}
}

// Apply delivers as the plan showed for each repository, not as the FileSet's
// via says.
func TestApply_FollowsPlannedVia(t *testing.T) {
	create := func(via string) []Change {
		return []Change{{FileSetID: "test", Target: "owner/repo", Path: "README.md", Type: ChangeCreate, Desired: "hi", Via: via}}
	}
	prOpts := ApplyOptions{FileSetID: "test", Via: manifest.ViaPullRequest}

	t.Run("planned pull_request but the repository is empty", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: map[string][]byte{repoViewKey: []byte("")}}
		p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

		results := p.Apply(context.Background(), create(manifest.ViaPullRequest), prOpts, ui.NoopReporter{})

		if len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "run plan again") {
			t.Fatalf("results = %+v, want a run plan again error", results)
		}
		if n := countCalls(mock.Called, "api repos/owner/repo/contents"); n != 0 {
			t.Errorf("contents calls = %d, want nothing written", n)
		}
	})

	t.Run("planned push to an empty repository", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: map[string][]byte{repoViewKey: []byte("")}}
		p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

		results := p.Apply(context.Background(), create(manifest.ViaPush), prOpts, ui.NoopReporter{})

		if len(results) != 1 || results[0].Err != nil || results[0].Via != manifest.ViaPush {
			t.Fatalf("results = %+v, want pushed", results)
		}
		if n := countCalls(mock.Called, "api repos/owner/repo/contents/README.md --method PUT"); n != 1 {
			t.Errorf("contents PUT calls = %d, want 1", n)
		}
	})

	t.Run("planned push to a repository that got commits", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: map[string][]byte{repoViewKey: []byte("main"), headRefKey: []byte("head123")}}
		p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))

		results := p.Apply(context.Background(), create(manifest.ViaPush), prOpts, ui.NoopReporter{})

		if len(results) != 1 || results[0].Err != nil || results[0].Via != manifest.ViaPush || results[0].PRURL != "" {
			t.Fatalf("results = %+v, want pushed without a pull request", results)
		}
		if n := countCalls(mock.Called, "pr "); n != 0 {
			t.Errorf("pr calls = %d, want none", n)
		}
		if n := countCalls(mock.Called, "api repos/owner/repo/git/refs --method POST"); n != 0 {
			t.Errorf("branch creations = %d, want none", n)
		}
	})
}
