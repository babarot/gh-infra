package importer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// githubRunner answers the Contents API with the content of each
// "owner/repo/path" it has; any other file is not found.
type githubRunner map[string]string

func (r githubRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	for key, content := range r {
		parts := strings.SplitN(key, "/", 3)
		if len(args) == 2 && args[1] == "repos/"+parts[0]+"/"+parts[1]+"/contents/"+parts[2] {
			return json.Marshal(map[string]string{"content": base64.StdEncoding.EncodeToString([]byte(content)), "encoding": "base64", "sha": "x"})
		}
	}
	return nil, gh.ErrNotFound
}

func (r githubRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func githubFiles(files map[string]string) gh.Runner { return githubRunner(files) }

// diffManifest writes the files under a temporary directory, parses
// manifest (fs.yaml), and diffs it against runner.
func diffManifest(t *testing.T, tree map[string]string, runner gh.Runner, resolver *manifest.SourceResolver) ([]Change, string) {
	t.Helper()
	dir := t.TempDir()
	for p, content := range tree {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if resolver == nil {
		resolver = &manifest.SourceResolver{}
	}
	result, err := manifest.ParseAll(filepath.Join(dir, "fs.yaml"), manifest.ParseOptions{Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := DiffFiles(context.Background(), runner, result.FileDocs, DiffFilesOptions{SourceRefCount: buildSourceRefCount(result.FileDocs)})
	if err != nil {
		t.Fatal(err)
	}
	return changes, filepath.Join(dir, "fs.yaml")
}

func findChange(t *testing.T, changes []Change, target, path string) Change {
	t.Helper()
	for _, c := range changes {
		if c.Target == target && c.Path == path {
			return c
		}
	}
	t.Fatalf("no change for %s %s", target, path)
	return Change{}
}

const originManifest = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - one
    - name: two
      overrides:
        - path: policy
          source: ./alt/
        - path: LICENSE
          content: two's license
  files:
    - path: policy
      source: ./base/
    - path: LICENSE
      content: mit
`

// Write-back targets are the entries as written, not positions counted
// after directories are expanded.
func TestDiffFiles_WritesBackToTheEntry(t *testing.T) {
	tree := map[string]string{"fs.yaml": originManifest, "base/a.rego": "a\n", "base/b.rego": "b\n", "alt/a.rego": "A2\n"}
	remote := map[string]string{}
	for _, repo := range []string{"one", "two"} {
		for _, p := range []string{"policy/a.rego", "policy/b.rego", "LICENSE"} {
			remote["org/"+repo+"/"+p] = "drift\n"
		}
	}
	changes, _ := diffManifest(t, tree, githubFiles(remote), nil)

	if c := findChange(t, changes, "org/one", "LICENSE"); c.YAMLPath != "$.spec.files[1].content" {
		t.Errorf("one LICENSE YAMLPath = %q, want $.spec.files[1].content", c.YAMLPath)
	}
	if c := findChange(t, changes, "org/two", "LICENSE"); c.YAMLPath != "$.spec.repositories[1].overrides[1].content" {
		t.Errorf("two LICENSE YAMLPath = %q, want $.spec.repositories[1].overrides[1].content", c.YAMLPath)
	}
	for _, tc := range []struct{ target, path string }{
		{"org/one", "policy/a.rego"}, {"org/one", "policy/b.rego"}, {"org/two", "policy/a.rego"},
	} {
		c := findChange(t, changes, tc.target, tc.path)
		if c.PatchYAMLPath != "" || !slices.Equal(c.AvailableModes, []WriteMode{WriteSource}) {
			t.Errorf("%s %s: patch %q, modes %v; want no patch target and write only", tc.target, tc.path, c.PatchYAMLPath, c.AvailableModes)
		}
	}
	if c := findChange(t, changes, "org/one", "policy/a.rego"); !slices.Contains(c.Warnings, "shared manifest: affects 2 repositories") {
		t.Errorf("one policy/a.rego warnings = %v, want the shared manifest warning", c.Warnings)
	}
}

// Writing the inline change back edits that entry only.
func TestWrite_InlineAfterDirectoryEntry(t *testing.T) {
	tree := map[string]string{"fs.yaml": originManifest, "base/a.rego": "a\n", "base/b.rego": "b\n", "alt/a.rego": "A2\n"}
	changes, manifestPath := diffManifest(t, tree, githubFiles(map[string]string{"org/one/LICENSE": "apache\n"}), nil)
	c := findChange(t, changes, "org/one", "LICENSE")
	if _, err := Write(&Result{ManifestEdits: map[string][]byte{}, FileChanges: []Change{c}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(originManifest, "      content: mit\n", "      content: |\n        apache\n", 1)
	if string(got) != want {
		t.Errorf("manifest =\n%s\nwant\n%s", got, want)
	}
}

// A file of a directory that other FileSets share is skipped unless the
// user picks write, since there is no patch to write instead.
func TestDiffFiles_SharedDirectorySkipsByDefault(t *testing.T) {
	doc := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories: [one]
  files:
    - path: policy
      source: ./base/
---
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories: [two]
  files:
    - path: policy
      source: ./base/
`
	tree := map[string]string{"fs.yaml": doc, "base/a.rego": "a\n", "base/b.rego": "b\n"}
	changes, _ := diffManifest(t, tree, githubFiles(map[string]string{"org/one/policy/a.rego": "drift\n"}), nil)
	c := findChange(t, changes, "org/one", "policy/a.rego")
	if c.DefaultSkipReason == "" || c.Type != fileset.ChangeUpdate || !slices.Equal(c.AvailableModes, []WriteMode{WriteSource}) {
		t.Errorf("change = %+v, want an update that is skipped by default and can be written", c)
	}
}

// An override cannot remove the patches of the entry it overrides: an
// empty patch would only drop its own, and the entry's would apply again.
func TestDiffFiles_OverrideCannotRemoveBasePatches(t *testing.T) {
	doc := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: one
      overrides:
        - path: a.txt
          vars: {k: v}
  files:
    - path: a.txt
      source: ./a.txt
      patches:
        - |
          --- a/a.txt
          +++ b/a.txt
          @@ -1 +1 @@
          -A
          +B
`
	tree := map[string]string{"fs.yaml": doc, "a.txt": "A\n"}
	changes, _ := diffManifest(t, tree, githubFiles(map[string]string{"org/one/a.txt": "A\n"}), nil)
	c := findChange(t, changes, "org/one", "a.txt")
	if c.WriteMode != WriteSkip || !strings.Contains(c.Reason, "cannot be removed by an override") {
		t.Errorf("change = %+v, want a hard skip", c)
	}
}

// A github:// file keeps its source after parse, so it is skipped as
// remote rather than written back as inline content next to source:.
func TestDiffFiles_GitHubSourceIsRemote(t *testing.T) {
	doc := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories: [one]
  files:
    - path: ci.yml
      source: github://owner/templates/ci.yml
`
	fileResp, _ := json.Marshal(map[string]string{"content": base64.StdEncoding.EncodeToString([]byte("ci\n")), "encoding": "base64"})
	resolver := &manifest.SourceResolver{RunGH: func(_ context.Context, args ...string) ([]byte, error) { return fileResp, nil }}
	changes, _ := diffManifest(t, map[string]string{"fs.yaml": doc}, githubFiles(map[string]string{"org/one/ci.yml": "drift\n"}), resolver)
	c := findChange(t, changes, "org/one", "ci.yml")
	if c.WriteMode != WriteSkip || c.YAMLPath != "" {
		t.Errorf("change = %+v, want a remote skip with no inline target", c)
	}
}
