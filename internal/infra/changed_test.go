package infra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// gitRepo creates a git repository in a temp dir with files committed, and
// returns its root (symlinks resolved) after changing into it.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := normalizePath(t.TempDir())
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	git(t, dir, "init", "-q", "-b", "main")
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "init")
	t.Chdir(dir)
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func changedList(t *testing.T, c *gitChanges) []string {
	t.Helper()
	var out []string
	for p := range c.paths {
		rel, err := filepath.Rel(c.toplevel, p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

func TestChangedSince(t *testing.T) {
	dir := gitRepo(t, map[string]string{
		"a.yaml":           "a",
		"b.yaml":           "b",
		"old.yaml":         "old",
		"templates/ci.yml": "ci",
	})
	git(t, dir, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(dir, "a.yaml"), "a2") // committed on the branch
	git(t, dir, "mv", "old.yaml", "new.yaml")
	git(t, dir, "commit", "-q", "-am", "change")

	// main moves on after the branch point; merge-base semantics ignore it.
	git(t, dir, "checkout", "-q", "main")
	writeFile(t, filepath.Join(dir, "b.yaml"), "b2")
	git(t, dir, "commit", "-q", "-am", "main moves")
	git(t, dir, "checkout", "-q", "feature")

	writeFile(t, filepath.Join(dir, "templates/ci.yml"), "ci2")  // uncommitted
	writeFile(t, filepath.Join(dir, "other/untracked.txt"), "x") // untracked, outside the cwd below

	// Run from a subdirectory: untracked files elsewhere must still count.
	t.Chdir(filepath.Join(dir, "templates"))
	changes, err := changedSince(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if changes.toplevel != dir {
		t.Errorf("toplevel = %s, want %s", changes.toplevel, dir)
	}
	got := strings.Join(changedList(t, changes), " ")
	want := "a.yaml new.yaml old.yaml other/untracked.txt templates/ci.yml"
	if got != want {
		t.Errorf("changed = %s, want %s", got, want)
	}

	if _, err := changedSince(context.Background(), "no-such-ref"); err == nil || !strings.Contains(err.Error(), "bad revision") {
		t.Errorf("expected a bad revision error, got %v", err)
	}
}

func TestParseChanged_Selects(t *testing.T) {
	const fileSet = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - %s
  files:
    - path: %s
      source: %s
`
	repo := func(name string) string {
		return "apiVersion: gh-infra/v1\nkind: Repository\nmetadata:\n  owner: org\n  name: " + name + "\nspec:\n  description: x\n"
	}
	dir := gitRepo(t, map[string]string{
		"repos/a.yaml":             repo("a"),
		"repos/b.yaml":             repo("b"),
		"repos/ci.yaml":            sprintf(fileSet, "a", ".github/ci.yml", "../templates/ci.yml"),
		"repos/hooks.yaml":         sprintf(fileSet, "b", ".hooks", "../templates/hooks/"),
		"repos/remote.yaml":        sprintf(fileSet, "c", "x.txt", "github://org/src/x.txt") + "      patches:\n        - ../patches/x.patch\n",
		"templates/ci.yml":         "ci",
		"templates/hooks/pre":      "pre",
		"patches/x.patch":          "--- a/x.txt\n+++ b/x.txt\n",
		"templates/unused/file.md": "u",
	})
	paths := []string{filepath.Join(dir, "repos")}
	noRemote := manifest.ParseOptions{Resolver: manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
		t.Fatalf("unexpected gh call: %v", args)
		return nil, nil
	})}

	selected := func(t *testing.T) []string {
		t.Helper()
		parsed, _, n, err := parseChanged(context.Background(), paths, "HEAD", noRemote)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range parsed.Repositories {
			names = append(names, "repo:"+r.Metadata.Name)
		}
		for _, fs := range parsed.FileSets {
			names = append(names, "fileset:"+fs.Spec.Repositories[0].Name)
		}
		if n != len(names) {
			t.Errorf("count = %d, want %d", n, len(names))
		}
		sort.Strings(names)
		return names
	}
	reset := func() { git(t, dir, "checkout", "-q", "."); git(t, dir, "clean", "-qfd") }

	for _, tt := range []struct {
		name   string
		change func()
		want   string
	}{
		{"nothing", func() {}, ""},
		{"manifest", func() { writeFile(t, filepath.Join(dir, "repos/a.yaml"), repo("a")+"  homepage: y\n") }, "repo:a"},
		{"template file", func() { writeFile(t, filepath.Join(dir, "templates/ci.yml"), "ci2") }, "fileset:a"},
		{"file added to source dir", func() { writeFile(t, filepath.Join(dir, "templates/hooks/post"), "post") }, "fileset:b"},
		{"patch file", func() { writeFile(t, filepath.Join(dir, "patches/x.patch"), "--- a/x.txt\n+++ b/x.txt\n\n") }, ""},
		{"unrelated file", func() { writeFile(t, filepath.Join(dir, "templates/unused/file.md"), "u2") }, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer reset()
			tt.change()
			if tt.name == "patch file" {
				// The remote FileSet is selected; parsing it again fetches its
				// github:// source, which noRemote rejects.
				if _, _, _, err := parseChanged(context.Background(), paths, "HEAD", manifest.ParseOptions{
					Resolver: manifest.NewSourceResolver(func(ctx context.Context, args ...string) ([]byte, error) {
						return nil, errFetched
					}),
				}); err == nil || !strings.Contains(err.Error(), errFetched.Error()) {
					t.Errorf("expected the remote FileSet to be selected and fetched, got %v", err)
				}
				return
			}
			if got := strings.Join(selected(t), " "); got != tt.want {
				t.Errorf("selected = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("path outside the repository", func(t *testing.T) {
		_, _, _, err := parseChanged(context.Background(), []string{t.TempDir()}, "HEAD", noRemote)
		if err == nil || !strings.Contains(err.Error(), "is not in the git repository") {
			t.Errorf("expected an outside-repository error, got %v", err)
		}
	})

	t.Run("validation still covers unchanged manifests", func(t *testing.T) {
		defer reset()
		pr := "apiVersion: gh-infra/v1\nkind: FileSet\nmetadata:\n  owner: org\nspec:\n  repositories:\n    - a\n  files:\n    - path: p.txt\n      content: p\n  via: pull_request\n  branch: sync\n"
		writeFile(t, filepath.Join(dir, "repos/pr1.yaml"), pr)
		git(t, dir, "add", "-A")
		git(t, dir, "commit", "-q", "-m", "pr1")
		writeFile(t, filepath.Join(dir, "repos/pr2.yaml"), pr) // untracked: changed
		_, _, _, err := parseChanged(context.Background(), paths, "HEAD", noRemote)
		if err == nil || !strings.Contains(err.Error(), "both open a pull request") {
			t.Errorf("expected the shared PR branch error, got %v", err)
		}
		git(t, dir, "rm", "-q", "repos/pr1.yaml")
		git(t, dir, "commit", "-q", "-m", "drop pr1")
	})
}

var errFetched = errors.New("remote source fetched")

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
