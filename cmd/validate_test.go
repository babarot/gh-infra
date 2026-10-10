package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const prFileSetYAML = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
  files:
    - path: a.txt
      content: a
  via: pull_request
`

// The PR branch check spans path arguments, not just documents in one file.
func TestRunValidate_RejectsSharedPRBranchAcrossPaths(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"ci.yaml", "hooks.yaml"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(prFileSetYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}

	err := runValidate(paths, false)

	if err == nil || !strings.Contains(err.Error(), `from branch "gh-infra/sync-org-repo"`) {
		t.Fatalf("expected shared PR branch error, got %v", err)
	}
}
