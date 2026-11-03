package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFilesFor_NoOverrides(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "a.txt", Content: "aaa"},
				{Path: "b.txt", Content: "bbb"},
			},
		},
	}
	target := FileSetRepository{Name: "repo"}

	result := filesFor(t, fs, target)

	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}
	if result[0].Content != "aaa" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "aaa")
	}
	if result[1].Content != "bbb" {
		t.Errorf("result[1].Content = %q, want %q", result[1].Content, "bbb")
	}
}

func TestFilesFor_WithOverrides(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "a.txt", Content: "original-a"},
				{Path: "b.txt", Content: "original-b"},
				{Path: "c.txt", Content: "original-c"},
			},
		},
	}
	target := FileSetRepository{
		Name: "owner/repo",
		Overrides: []FileEntry{
			{Path: "b.txt", Content: "overridden-b"},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result))
	}
	if result[0].Content != "original-a" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "original-a")
	}
	if result[1].Content != "overridden-b" {
		t.Errorf("result[1].Content = %q, want %q", result[1].Content, "overridden-b")
	}
	if result[2].Content != "original-c" {
		t.Errorf("result[2].Content = %q, want %q", result[2].Content, "original-c")
	}
}

func TestFilesFor_InheritsDirScopeAndReconcile(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{
					Path:      "config/a.yml",
					Content:   "original-a",
					DirScope:  "config",
					Reconcile: ReconcileAuthoritative,
					Vars:      map[string]string{"env": "prod"},
				},
				{
					Path:      "config/b.yml",
					Content:   "original-b",
					DirScope:  "config",
					Reconcile: ReconcileAuthoritative,
				},
			},
		},
	}
	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			// Override content but don't set DirScope or Reconcile
			{Path: "config/a.yml", Content: "overridden-a"},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}

	// Check the overridden entry inherits DirScope and Reconcile
	if result[0].Content != "overridden-a" {
		t.Errorf("result[0].Content = %q, want %q", result[0].Content, "overridden-a")
	}
	if result[0].DirScope != "config" {
		t.Errorf("result[0].DirScope = %q, want %q (should inherit from original)", result[0].DirScope, "config")
	}
	if result[0].Reconcile != ReconcileAuthoritative {
		t.Errorf("result[0].Reconcile = %q, want %q (should inherit from original)", result[0].Reconcile, ReconcileAuthoritative)
	}
	// Vars should also be inherited
	if result[0].Vars == nil || result[0].Vars["env"] != "prod" {
		t.Errorf("result[0].Vars should inherit from original, got %v", result[0].Vars)
	}

	// Non-overridden entry should retain its fields
	if result[1].DirScope != "config" {
		t.Errorf("result[1].DirScope = %q, want %q", result[1].DirScope, "config")
	}
	if result[1].Reconcile != ReconcileAuthoritative {
		t.Errorf("result[1].Reconcile = %q, want %q", result[1].Reconcile, ReconcileAuthoritative)
	}
}

func TestFilesFor_InheritsPatches(t *testing.T) {
	patches := []string{"--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n"}
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{
					Path:    ".tagpr",
					Content: "original",
					Patches: patches,
				},
			},
		},
	}
	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			// Override content but don't set Patches — should inherit
			{Path: ".tagpr", Content: "overridden"},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Content != "overridden" {
		t.Errorf("Content = %q, want %q", result[0].Content, "overridden")
	}
	if len(result[0].Patches) != 1 {
		t.Fatalf("Patches should be inherited, got %d patches", len(result[0].Patches))
	}
	if result[0].Patches[0] != patches[0] {
		t.Errorf("Patches[0] = %q, want %q", result[0].Patches[0], patches[0])
	}
}

func TestFilesFor_OverridePatchesReplacesOriginal(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{
					Path:    ".tagpr",
					Content: "original",
					Patches: []string{"original-patch"},
				},
			},
		},
	}
	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			// Override provides its own patches — should NOT inherit original
			{Path: ".tagpr", Content: "overridden", Patches: []string{"override-patch"}},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if len(result[0].Patches) != 1 || result[0].Patches[0] != "override-patch" {
		t.Errorf("Patches should use override's patches, got %v", result[0].Patches)
	}
}

func TestFilesFor_InheritsContentAndSource(t *testing.T) {
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{
					Path:           ".goreleaser.yaml",
					Content:        "dummy message",
					Source:         "github://owner/repo/templates/.goreleaser.yaml",
					OriginalSource: "templates/.goreleaser.yaml",
				},
			},
		},
	}

	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			{Path: ".goreleaser.yaml", Vars: map[string]string{"Description": "overwrite message"}},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Content != "dummy message" {
		t.Errorf("Content = %q, want %q", result[0].Content, "dummy message")
	}
	if result[0].Source != "github://owner/repo/templates/.goreleaser.yaml" {
		t.Errorf("Source = %q, want %q", result[0].Source, "github://owner/repo/templates/.goreleaser.yaml")
	}
	if result[0].OriginalSource != "templates/.goreleaser.yaml" {
		t.Errorf("OriginalSource = %q, want %q", result[0].OriginalSource, "templates/.goreleaser.yaml")
	}
	if v := result[0].Vars["Description"]; v != "overwrite message" {
		t.Errorf("Vars.Description = %q, want %q", v, "overwrite message")
	}
}

func TestFilesFor_InheritsExecutable(t *testing.T) {
	execTrue := true
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "bin/tool", Content: "#!/bin/sh\necho hello", Executable: &execTrue},
			},
		},
	}
	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			// Content override but Executable is nil — should inherit from base.
			{Path: "bin/tool", Content: "#!/bin/sh\necho world"},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Executable == nil {
		t.Fatal("Executable should be inherited (non-nil), got nil")
	}
	if !*result[0].Executable {
		t.Errorf("Executable = false, want true (inherited from base)")
	}
}

func TestFilesFor_ExecutableNotInherited_WhenOverrideExplicitlySetsIt(t *testing.T) {
	execTrue := true
	execFalse := false
	fs := &FileSet{
		Spec: FileSetSpec{
			Files: []FileEntry{
				{Path: "bin/tool", Content: "#!/bin/sh\necho hello", Executable: &execTrue},
			},
		},
	}
	target := FileSetRepository{
		Name: "repo",
		Overrides: []FileEntry{
			// Override explicitly sets Executable: false — must NOT be overwritten.
			{Path: "bin/tool", Content: "#!/bin/sh\necho world", Executable: &execFalse},
		},
	}

	result := filesFor(t, fs, target)

	if len(result) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result))
	}
	if result[0].Executable == nil {
		t.Fatal("Executable should be non-nil")
	}
	if *result[0].Executable {
		t.Errorf("Executable = true, want false (override should take precedence)")
	}
}

func filesFor(t *testing.T, fs *FileSet, target FileSetRepository) []FileEntry {
	t.Helper()
	files, err := fs.FilesFor(target)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// dirFileSet has a file entry and a directory entry of two files, as parse
// leaves them with sources resolved.
func dirFileSet() *FileSet {
	return &FileSet{Spec: FileSetSpec{Files: []FileEntry{
		{Path: "README.md", Content: "readme"},
		{Path: "policy/a.rego", Content: "a", OriginalSource: "/base/a.rego", DirScope: "policy", Description: "Policies", Reconcile: ReconcileAuthoritative},
		{Path: "policy/b.rego", Content: "b", OriginalSource: "/base/b.rego", DirScope: "policy", Description: "Policies", Reconcile: ReconcileAuthoritative},
		{Path: "LICENSE", Content: "mit"},
	}}}
}

func paths(files []FileEntry) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestFilesFor_DirectoryFieldsApplyToEveryFile(t *testing.T) {
	got := filesFor(t, dirFileSet(), FileSetRepository{Name: "r", Overrides: []FileEntry{
		{Path: "policy", Description: "For r", Vars: map[string]string{"k": "v"}},
	}})
	if !reflect.DeepEqual(paths(got), []string{"README.md", "policy/a.rego", "policy/b.rego", "LICENSE"}) {
		t.Fatalf("paths = %v", paths(got))
	}
	for _, f := range got[1:3] {
		if f.Description != "For r" || f.Vars["k"] != "v" || f.Reconcile != ReconcileAuthoritative || f.DirScope != "policy" {
			t.Errorf("%s = %+v, want the override's fields and the rest from the entry", f.Path, f)
		}
	}
	if got[1].Content != "a" || got[1].OriginalSource != "/base/a.rego" || got[2].Content != "b" {
		t.Errorf("contents = %q, %q; want each file's own", got[1].Content, got[2].Content)
	}
	if got[0].Description != "" {
		t.Errorf("README.md got %q, want the override only on the directory", got[0].Description)
	}
}

func TestFilesFor_DirectorySourceReplacesTheDirectory(t *testing.T) {
	got := filesFor(t, dirFileSet(), FileSetRepository{Name: "r", Overrides: []FileEntry{
		// policy, source: ./alt/ with a.rego and c.rego, as parse resolves it
		{Path: "policy/a.rego", Content: "A2", OriginalSource: "/alt/a.rego", DirScope: "policy"},
		{Path: "policy/c.rego", Content: "c", OriginalSource: "/alt/c.rego", DirScope: "policy", Description: "Own"},
	}})
	if !reflect.DeepEqual(paths(got), []string{"README.md", "policy/a.rego", "policy/c.rego", "LICENSE"}) {
		t.Fatalf("paths = %v, want the directory replaced in its place", paths(got))
	}
	if got[1].Content != "A2" || got[1].OriginalSource != "/alt/a.rego" {
		t.Errorf("a.rego = %+v, want the override's file", got[1])
	}
	if got[1].Description != "Policies" || got[1].Reconcile != ReconcileAuthoritative {
		t.Errorf("a.rego = %+v, want description and reconcile from the entry", got[1])
	}
	if got[2].Description != "Own" {
		t.Errorf("c.rego description = %q, want the override's own", got[2].Description)
	}
}

// An empty file in the replacing directory stays empty rather than taking
// the content of a file it replaces.
func TestFilesFor_DirectorySourceKeepsEmptyFile(t *testing.T) {
	got := filesFor(t, dirFileSet(), FileSetRepository{Name: "r", Overrides: []FileEntry{
		{Path: "policy/a.rego", Content: "", OriginalSource: "/alt/a.rego", DirScope: "policy"},
	}})
	if got[1].Path != "policy/a.rego" || got[1].Content != "" || got[1].OriginalSource != "/alt/a.rego" {
		t.Errorf("a.rego = %+v, want the empty file from the override", got[1])
	}
}

func TestFilesFor_DirectoryFieldsThenFile(t *testing.T) {
	got := filesFor(t, dirFileSet(), FileSetRepository{Name: "r", Overrides: []FileEntry{
		{Path: "policy/b.rego", Content: "B2", Description: "Only b"},
		{Path: "policy", Description: "For r"},
	}})
	if got[1].Description != "For r" {
		t.Errorf("a.rego description = %q, want the directory override", got[1].Description)
	}
	if got[2].Content != "B2" || got[2].Description != "Only b" || got[2].DirScope != "policy" {
		t.Errorf("b.rego = %+v, want the file override on top of the directory override", got[2])
	}
}

func TestFilesFor_Errors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		overrides []FileEntry
		want      string
	}{
		{"no entry", []FileEntry{{Path: "missing.txt", Content: "x"}}, "override missing.txt for r matches no entry in files"},
		{"content for a directory", []FileEntry{{Path: "policy", Content: "x"}}, "gives a directory entry a single file"},
		{"directory for a file", []FileEntry{{Path: "LICENSE/x", Content: "x", DirScope: "LICENSE"}}, "has a directory source, but LICENSE is a file"},
		{"directory not in files", []FileEntry{{Path: "other/x", Content: "x", DirScope: "other"}}, "override other for r matches no entry in files"},
		{"file of a replaced directory", []FileEntry{
			{Path: "policy/c.rego", Content: "c", DirScope: "policy"},
			{Path: "policy/b.rego", Content: "B2"},
		}, "override policy/b.rego for r matches no file: its directory is replaced"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dirFileSet().FilesFor(FileSetRepository{Name: "r", Overrides: tt.overrides})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

// A replacing directory can bring a file that another entry also writes.
func TestFilesFor_DuplicateAfterReplace(t *testing.T) {
	fs := &FileSet{Spec: FileSetSpec{Files: []FileEntry{
		{Path: ".github/ci.yml", Content: "ci", DirScope: ".github"},
		{Path: ".github/CODEOWNERS", Content: "* @a"},
	}}}
	_, err := fs.FilesFor(FileSetRepository{Name: "r", Overrides: []FileEntry{
		{Path: ".github/ci.yml", Content: "ci", DirScope: ".github"},
		{Path: ".github/CODEOWNERS", Content: "* @b", DirScope: ".github"},
	}})
	if err == nil || !strings.Contains(err.Error(), `duplicate file path ".github/CODEOWNERS"`) {
		t.Errorf("err = %v, want a duplicate path error", err)
	}
}

// writeTree writes files (path -> content) under dir; a path ending in /
// is an empty directory.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const overrideFileSet = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: repo
      overrides:
%s
  files:
    - path: policy
      source: ./base/
      description: Policies
`

func parseOverride(t *testing.T, overrides string, opts ParseOptions) (*ParseResult, error) {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"base/a.rego": "a", "base/b.rego": "b",
		"alt/a.rego": "A2", "alt/c.rego": "c",
		"empty/": "",
	})
	path := filepath.Join(dir, "fs.yaml")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(overrideFileSet, overrides)), 0o644); err != nil {
		t.Fatal(err)
	}
	return ParseAll(path, opts)
}

func TestParseFileSet_DirectoryOverride(t *testing.T) {
	result, err := parseOverride(t, "        - path: policy\n          source: ./alt/", fileOpts())
	if err != nil {
		t.Fatal(err)
	}
	fs := result.FileSets[0]
	got := filesFor(t, fs, fs.Spec.Repositories[0])
	if !reflect.DeepEqual(paths(got), []string{"policy/a.rego", "policy/c.rego"}) {
		t.Errorf("paths = %v, want alt/ in place of base/", paths(got))
	}
	if got[0].Content != "A2" || got[0].Description != "Policies" {
		t.Errorf("a.rego = %+v, want alt/'s content and the entry's description", got[0])
	}
}

func TestParseFileSet_OverrideErrors(t *testing.T) {
	for _, tt := range []struct {
		name, overrides, want string
	}{
		{"no entry", "        - path: polcy\n          description: x", "override polcy for repo matches no entry in files"},
		{"listed twice", "        - path: policy\n          description: x\n        - path: policy\n          source: ./alt/", "override policy for repo is listed twice"},
		{"empty directory", "        - path: policy\n          source: ./empty/", "override policy for repo: source ./empty/ has no files"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOverride(t, tt.overrides, fileOpts())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

// With github:// sources left unresolved, a remote directory is one entry
// at its path, so a local override of one of its files cannot be matched;
// the check waits for the parse that resolves it.
func TestParseFileSet_OverridesNotCheckedWithRemoteSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"local.rego": "x"})
	path := filepath.Join(dir, "fs.yaml")
	doc := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - name: repo
      overrides:
        - path: policy/a.rego
          source: ./local.rego
  files:
    - path: policy
      source: github://owner/config/policy/
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAll(path, ParseOptions{Resolver: &SourceResolver{}, SkipRemoteSources: true}); err != nil {
		t.Errorf("unexpected error with remote sources skipped: %v", err)
	}
}

// Each resolved file keeps where its entry is written, before directories
// are expanded, and the source it came from.
func TestParseFileSet_Origins(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"base/a.rego": "a", "base/b.rego": "b", "alt/a.rego": "A2", "alt/c.rego": "c", "lic": "mit"})
	path := filepath.Join(dir, "fs.yaml")
	doc := `apiVersion: gh-infra/v1
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
          content: two
  files:
    - path: policy
      source: ./base/
    - path: LICENSE
      source: ./lic
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ParseAll(path, fileOpts())
	if err != nil {
		t.Fatal(err)
	}
	fs := result.FileSets[0]
	type origin struct {
		Path   string
		Origin FileOrigin
		Source string
	}
	collect := func(files []FileEntry) []origin {
		var out []origin
		for _, f := range files {
			out = append(out, origin{f.Path, f.Origin, f.Source})
		}
		return out
	}
	if got, want := collect(filesFor(t, fs, fs.Spec.Repositories[0])), []origin{
		{"policy/a.rego", FileOrigin{Index: 0, Dir: true}, "./base/"},
		{"policy/b.rego", FileOrigin{Index: 0, Dir: true}, "./base/"},
		{"LICENSE", FileOrigin{Index: 1}, "./lic"},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("one:\n got %+v\nwant %+v", got, want)
	}
	if got, want := collect(filesFor(t, fs, fs.Spec.Repositories[1])), []origin{
		{"policy/a.rego", FileOrigin{Override: true, Index: 0, Dir: true}, "./alt/"},
		{"policy/c.rego", FileOrigin{Override: true, Index: 0, Dir: true}, "./alt/"},
		// content of its own: written back to the override, not to ./lic
		{"LICENSE", FileOrigin{Override: true, Index: 1}, ""},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("two:\n got %+v\nwant %+v", got, want)
	}
	if got := filesFor(t, fs, fs.Spec.Repositories[1])[2].OriginalSource; got != "" {
		t.Errorf("LICENSE override OriginalSource = %q, want none", got)
	}
}

func TestParseFile_Origins(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"base/a": "a", "base/b": "b"})
	path := filepath.Join(dir, "file.yaml")
	doc := `apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: dir
      source: ./base/
    - path: note.txt
      content: x
`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ParseAll(path, fileOpts())
	if err != nil {
		t.Fatal(err)
	}
	var got []FileOrigin
	for _, f := range result.FileSets[0].Spec.Files {
		got = append(got, f.Origin)
	}
	if want := []FileOrigin{{Index: 0, Dir: true}, {Index: 0, Dir: true}, {Index: 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("origins = %+v, want %+v", got, want)
	}
}

func TestFilesFor_OriginsAndBasePatches(t *testing.T) {
	fs := dirFileSet()
	fs.Spec.Files[0].Patches = []string{"p"} // README.md
	for i := range fs.Spec.Files {
		fs.Spec.Files[i].Origin = FileOrigin{Index: i}
	}
	fs.Spec.Files[1].Origin = FileOrigin{Index: 1, Dir: true}
	fs.Spec.Files[2].Origin = FileOrigin{Index: 1, Dir: true}
	fs.Spec.Files[1].Source, fs.Spec.Files[2].Source = "./base/", "./base/"

	got := filesFor(t, fs, FileSetRepository{Name: "r", Overrides: []FileEntry{
		{Path: "policy", Description: "x", Origin: FileOrigin{Override: true, Index: 0}},
		{Path: "README.md", Content: "own", Origin: FileOrigin{Override: true, Index: 1}},
	}})
	if got[1].Origin != (FileOrigin{Index: 1, Dir: true}) || got[1].Source != "./base/" {
		t.Errorf("fields-only directory override: %+v, want the file's own origin and source", got[1])
	}
	if got[0].Origin != (FileOrigin{Override: true, Index: 1}) || !got[0].BaseHasPatches {
		t.Errorf("file override: %+v, want the override's origin and BaseHasPatches", got[0])
	}
	if got[3].BaseHasPatches {
		t.Errorf("LICENSE has no override, got BaseHasPatches")
	}
}
