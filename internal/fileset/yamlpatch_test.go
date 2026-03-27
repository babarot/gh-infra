package fileset

import (
	"strings"
	"testing"
)

func TestReplaceInlineContent_Basic(t *testing.T) {
	input := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot
spec:
  files:
    - path: .github/pr-labeler.yaml
      content: |
        old content here
        second line
    - path: .github/release.yml
      source: ./templates/release.yml
`
	newContent := "new content\nreplaced line\n"

	result, err := ReplaceInlineContent([]byte(input), 0, 0, newContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(result)

	// Should contain the new content
	if !strings.Contains(output, "new content") {
		t.Errorf("output should contain new content, got:\n%s", output)
	}

	// Should NOT contain the old content
	if strings.Contains(output, "old content here") {
		t.Errorf("output should not contain old content, got:\n%s", output)
	}

	// Should preserve other entries
	if !strings.Contains(output, "source: ./templates/release.yml") {
		t.Errorf("output should preserve other entries, got:\n%s", output)
	}

	// Should preserve metadata
	if !strings.Contains(output, "owner: babarot") {
		t.Errorf("output should preserve metadata, got:\n%s", output)
	}
}

func TestReplaceInlineContent_SecondEntry(t *testing.T) {
	input := `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot
spec:
  files:
    - path: first.yaml
      content: |
        first content
    - path: second.yaml
      content: |
        second content
`
	newContent := "replaced second\n"

	result, err := ReplaceInlineContent([]byte(input), 0, 1, newContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(result)

	// Should keep the first entry unchanged
	if !strings.Contains(output, "first content") {
		t.Errorf("first entry should be unchanged, got:\n%s", output)
	}

	// Should replace the second entry
	if !strings.Contains(output, "replaced second") {
		t.Errorf("second entry should be replaced, got:\n%s", output)
	}
}

func TestReplaceInlineContent_InvalidDocIndex(t *testing.T) {
	input := `kind: FileSet
spec:
  files:
    - path: test.yaml
      content: |
        test
`
	_, err := ReplaceInlineContent([]byte(input), 5, 0, "new")
	if err == nil {
		t.Fatal("expected error for invalid doc index")
	}
}
