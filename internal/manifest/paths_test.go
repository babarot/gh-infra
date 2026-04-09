package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths_DefaultToDot(t *testing.T) {
	paths, err := ResolvePaths(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	wd, _ := os.Getwd()
	if paths[0] != wd {
		t.Errorf("expected %s, got %s", wd, paths[0])
	}
}

func TestResolvePaths_DeduplicatesExact(t *testing.T) {
	dir := t.TempDir()
	paths, err := ResolvePaths([]string{dir, dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 deduplicated path, got %d", len(paths))
	}
}

func TestResolvePaths_DetectsContainment(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ResolvePaths([]string{parent, child})
	if err == nil {
		t.Fatal("expected error for overlapping paths")
	}
}

func TestResolvePaths_DisjointPaths(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	paths, err := ResolvePaths([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
}

func TestParseAllMultiplePaths(t *testing.T) {
	// Create two separate directories with different manifests.
	reposDir := t.TempDir()
	filesDir := t.TempDir()

	repoYAML := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
  owner: org
spec:
  visibility: public
`
	fileSetYAML := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - my-repo
  files:
    - path: .editorconfig
      content: "root = true"
`
	if err := os.WriteFile(filepath.Join(reposDir, "repo.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "fileset.yaml"), []byte(fileSetYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Parse each directory and merge — same flow as plan/apply/validate.
	paths, err := ResolvePaths([]string{reposDir, filesDir})
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}

	merged := &ParseResult{}
	for _, p := range paths {
		result, err := ParseAll(p, ParseOptions{Resolver: &SourceResolver{}})
		if err != nil {
			t.Fatalf("ParseAll(%s): %v", p, err)
		}
		merged.Merge(result)
	}

	if len(merged.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(merged.Repositories))
	}
	if len(merged.FileSets) != 1 {
		t.Errorf("expected 1 fileset, got %d", len(merged.FileSets))
	}
	if merged.Repositories[0].Metadata.Name != "my-repo" {
		t.Errorf("repo name = %q, want %q", merged.Repositories[0].Metadata.Name, "my-repo")
	}
	if merged.FileSets[0].Metadata.Owner != "org" {
		t.Errorf("fileset owner = %q, want %q", merged.FileSets[0].Metadata.Owner, "org")
	}
}
