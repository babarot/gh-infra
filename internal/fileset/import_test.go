package fileset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyImport_UpdatesRepositoryOverrideInlineContent(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "fileset.yaml")
	input := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot
spec:
  repositories:
    - name: repo-a
    - name: repo-b
      overrides:
        - path: config.yml
          content: |
            old override
  files:
    - path: config.yml
      content: |
        base content
`
	if err := os.WriteFile(manifestPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	changes := []ImportChange{
		{
			Path:         "config.yml",
			WriteMode:    ImportWriteInline,
			ManifestPath: manifestPath,
			DocIndex:     0,
			YAMLPath:     "$.spec.repositories[1].overrides[0].content",
			Desired:      "new override\n",
		},
	}

	if err := ApplyImport(changes, map[string][]byte{manifestPath: []byte(input)}); err != nil {
		t.Fatalf("ApplyImport returned error: %v", err)
	}

	gotBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(gotBytes)

	if !strings.Contains(got, "new override") {
		t.Fatalf("expected override content to be updated, got:\n%s", got)
	}
	if strings.Contains(got, "old override") {
		t.Fatalf("expected old override content to be removed, got:\n%s", got)
	}
	if !strings.Contains(got, "base content") {
		t.Fatalf("expected base content to stay unchanged, got:\n%s", got)
	}
}
