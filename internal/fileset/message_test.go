package fileset

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/ui"
)

func TestBuildFiles(t *testing.T) {
	changes := []Change{
		{Path: "README.md", Type: ChangeNoOp, Entry: "README.md", EntryIndex: 0},
		{Path: "policy/a.rego", Type: ChangeCreate, Entry: "policy", EntryIndex: 2, Description: "Policies\n"},
		{Path: ".tagpr", Type: ChangeUpdate, Entry: ".tagpr", EntryIndex: 1, Description: "For <% .Repo.Name %>"},
		{Path: "policy/b.rego", Type: ChangeUpdate, Entry: "policy", EntryIndex: 2, Description: "Policies\n"},
		{Path: "old/x", Type: ChangeDelete, Entry: "old", EntryIndex: 3},
	}
	got, err := buildFiles(changes, "owner/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []FileContext{
		{Path: ".tagpr", Paths: []string{".tagpr"}, Count: 1, Type: "update", Action: "Update", Description: "For repo"},
		{Path: "policy", Paths: []string{"policy/a.rego", "policy/b.rego"}, Count: 2, Type: "mixed", Action: "Update", Description: "Policies", dir: true},
		{Path: "old", Paths: []string{"old/x"}, Count: 1, Type: "delete", Action: "Remove", dir: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildFiles =\n%+v\nwant\n%+v", got, want)
	}
}

// Deletions found by authoritative come after every planned file, but the
// entry keeps its place in the manifest.
func TestBuildFiles_ManifestOrder(t *testing.T) {
	changes := []Change{
		{Path: "b", Type: ChangeCreate, Entry: "b", EntryIndex: 1},
		{Path: "a/x", Type: ChangeDelete, Entry: "a", EntryIndex: 0},
	}
	got, err := buildFiles(changes, "owner/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Errorf("buildFiles order = %+v, want a then b", got)
	}
}

func TestBuildFiles_DescriptionTemplateError(t *testing.T) {
	_, err := buildFiles([]Change{{Path: "a", Type: ChangeCreate, Entry: "a", Description: "<% .Repo.Nme %>"}}, "owner/repo", "")
	if err == nil || !strings.Contains(err.Error(), "description of a") {
		t.Errorf("err = %v, want a description error naming the entry", err)
	}
}

func TestBuildFiles_DescriptionHasSourceURL(t *testing.T) {
	got, err := buildFiles([]Change{{Path: "a", Type: ChangeCreate, Entry: "a", Description: "See <% .Source.URL %>"}}, "owner/repo", "https://example.com/1")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Description != "See https://example.com/1" {
		t.Errorf("description = %q", got[0].Description)
	}
}

func TestDefaultCommitMessage(t *testing.T) {
	one := FileContext{Path: ".tagpr", Paths: []string{".tagpr"}, Count: 1, Type: "create", Action: "Add"}
	dir := FileContext{Path: "policy", Paths: []string{"policy/a", "policy/b"}, Count: 2, Type: "create", Action: "Add", dir: true}
	upd := FileContext{Path: "x", Paths: []string{"x"}, Count: 1, Type: "update", Action: "Update"}
	for _, tt := range []struct {
		name  string
		files []FileContext
		want  string
	}{
		{"one file", []FileContext{one}, "Add .tagpr"},
		{"one file of a directory", []FileContext{{Path: "policy", Paths: []string{"policy/a"}, Count: 1, Type: "delete", Action: "Remove", dir: true}}, "Remove policy/a"},
		{"same action", []FileContext{one, dir}, "Add 3 files"},
		{"mixed", []FileContext{one, upd}, "Update 2 files"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultCommitMessage(tt.files); got != tt.want {
				t.Errorf("defaultCommitMessage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultPRBody(t *testing.T) {
	files := []FileContext{
		{Path: ".claude/rules/adr.md", Paths: []string{".claude/rules/adr.md"}, Count: 1, Action: "Add", Description: "The rules for ADRs."},
		{Path: ".regomark/policy", Paths: []string{"a", "b"}, Count: 2, Action: "Add", Description: "Policies.\n\nWith tests.", dir: true},
		{Path: "lib", Paths: []string{"lib/x"}, Count: 1, Action: "Remove", dir: true},
		{Path: ".tagpr", Paths: []string{".tagpr"}, Count: 1, Action: "Update"},
	}
	want := "- Add `.claude/rules/adr.md`: The rules for ADRs.\n" +
		"- Add `.regomark/policy/` (2 files): Policies.\n  \n  With tests.\n" +
		"- Remove `lib/` (1 file)\n" +
		"- Update `.tagpr`"
	if got := defaultPRBody(files); got != want {
		t.Errorf("defaultPRBody =\n%s\nwant\n%s", got, want)
	}
}

func TestEntryFiles(t *testing.T) {
	files := []manifest.FileEntry{
		{Path: "a", Description: "A"},
		{Path: "dir/x", DirScope: "dir", Description: "D"},
		{Path: "dir/y", DirScope: "dir", Description: "D"},
	}
	got, err := entryFiles(files, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	want := []FileContext{
		{Path: "a", Paths: []string{"a"}, Count: 1, Type: "create", Action: "Add", Description: "A"},
		{Path: "dir", Paths: []string{"dir/x", "dir/y"}, Count: 2, Type: "create", Action: "Add", Description: "D", dir: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entryFiles =\n%+v\nwant\n%+v", got, want)
	}
}

// Plan records the manifest entry of each change: the directory for files
// of a directory source and for the files authoritative deletes there.
func TestPlan_ChangesCarryTheirEntry(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "config"): dirContentsJSON([]struct{ Path, Type string }{
				{Path: "config/a.yml", Type: "file"},
				{Path: "config/old.yml", Type: "file"},
			}),
			contentsKey("owner/repo", "config/a.yml"): contentsJSON("old", "sha1"),
		},
		Errors: map[string]error{
			contentsKey("owner/repo", ".tagpr"): gh.ErrNotFound,
		},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: ".tagpr", Content: "x", Description: "Release settings"},
		{Path: "config/a.yml", Content: "new", Reconcile: manifest.ReconcileAuthoritative, DirScope: "config", Description: "Config"},
	})

	changes, err := p.Plan(context.Background(), fileSets, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		Path, Entry string
		Index       int
		Description string
	}
	var got []entry
	for _, c := range changes {
		got = append(got, entry{c.Path, c.Entry, c.EntryIndex, c.Description})
	}
	want := []entry{
		{".tagpr", ".tagpr", 0, "Release settings"},
		{"config/a.yml", "config", 1, "Config"},
		{"config/old.yml", "config", 1, "Config"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entries = %+v, want %+v", got, want)
	}
}

// A template that works on the files apply would change but not on others
// is checked against the planned changes, as apply renders it.
func TestPlan_MessageCheckedAgainstPlannedFiles(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{contentsKey("owner/repo", "a"): contentsJSON("a", "sha")},
		Errors:    map[string]error{contentsKey("owner/repo", "b"): gh.ErrNotFound},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "a", Content: "a"},
		{Path: "b", Content: "b"},
	})
	// Only b changes, so .Files has one entry; index 1 fails as it would at apply.
	fileSets[0].Spec.PRTitle = "<% (index .Files 1).Path %>"

	_, err := p.Plan(context.Background(), fileSets, "", nil)
	if err == nil || !strings.Contains(err.Error(), "pr_title for owner/repo") {
		t.Fatalf("expected a pr_title error from the planned files, got %v", err)
	}
}

func TestCheckMessages_FilesWithoutChanges(t *testing.T) {
	fs := makeFileSet("owner", "repo", []manifest.FileEntry{{Path: "a", Content: "a", Description: "A"}})[0]
	fs.Spec.PRBody = "<% range .Files %><% .Path %>: <% .Description %><% end %>"
	if err := CheckMessages(fs, fs.Spec.Repositories[0], nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	fs.Spec.PRBody = "<% range .Files %><% .Nope %><% end %>"
	if err := CheckMessages(fs, fs.Spec.Repositories[0], nil); err == nil || !strings.Contains(err.Error(), "pr_body for owner/repo") {
		t.Errorf("err = %v, want a pr_body error", err)
	}
	fs.Spec.PRBody = ""
	fs.Spec.Files[0].Description = "<% .Repo.Nme %>"
	if err := CheckMessages(fs, fs.Spec.Repositories[0], nil); err == nil || !strings.Contains(err.Error(), "description of a") {
		t.Errorf("err = %v, want a description error", err)
	}
}

func TestApplyToEmptyRepo_DefaultMessagePerFile(t *testing.T) {
	mock := &WildcardMockRunner{MockRunner: gh.MockRunner{Responses: map[string][]byte{}, Errors: map[string]error{}}}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	changes := []Change{
		{Path: ".github/ci.yml", Type: ChangeCreate, Desired: "name: CI", Entry: ".github", EntryIndex: 0},
		{Path: ".github/lint.yml", Type: ChangeCreate, Desired: "name: Lint", Entry: ".github", EntryIndex: 0},
	}
	if err := p.applyToEmptyRepo(context.Background(), "owner/repo", changes, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"message=Add .github/ci.yml", "message=Add .github/lint.yml"} {
		if i >= len(mock.Called) || !strings.Contains(strings.Join(mock.Called[i], "\x00"), want+"\x00") {
			t.Errorf("call %d: expected %q, got %q", i, want, mock.Called)
		}
	}
}

// A directory replaced by an override, with reconcile: authoritative, is
// planned against the override's files: one only the base has is deleted,
// one only the override has is created.
func TestPlan_ReplacedDirectoryIsAuthoritativeOverItsFiles(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			contentsKey("owner/repo", "policy"): dirContentsJSON([]struct{ Path, Type string }{
				{Path: "policy/a.rego", Type: "file"},
				{Path: "policy/b.rego", Type: "file"},
			}),
			contentsKey("owner/repo", "policy/a.rego"): contentsJSON("a", "sha1"),
		},
		Errors: map[string]error{contentsKey("owner/repo", "policy/c.rego"): gh.ErrNotFound},
	}
	p := NewProcessor(mock, ui.NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{}))
	fileSets := makeFileSet("owner", "repo", []manifest.FileEntry{
		{Path: "policy/a.rego", Content: "a", DirScope: "policy", Reconcile: manifest.ReconcileAuthoritative},
		{Path: "policy/b.rego", Content: "b", DirScope: "policy", Reconcile: manifest.ReconcileAuthoritative},
	})
	fileSets[0].Spec.Repositories[0].Overrides = []manifest.FileEntry{
		{Path: "policy/a.rego", Content: "a", DirScope: "policy"},
		{Path: "policy/c.rego", Content: "c", DirScope: "policy"},
	}

	changes, err := p.Plan(context.Background(), fileSets, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeType{}
	for _, c := range changes {
		got[c.Path] = c.Type
	}
	want := map[string]ChangeType{"policy/a.rego": ChangeNoOp, "policy/c.rego": ChangeCreate, "policy/b.rego": ChangeDelete}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
}
