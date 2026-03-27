package yamlpatch

import (
	"strings"
	"testing"
)

func TestReplaceLiteralContent_Basic(t *testing.T) {
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

	result, err := ReplaceLiteralContent([]byte(input), 0, "$.spec.files[0].content", newContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(result)

	if !strings.Contains(output, "new content") {
		t.Errorf("output should contain new content, got:\n%s", output)
	}

	if strings.Contains(output, "old content here") {
		t.Errorf("output should not contain old content, got:\n%s", output)
	}

	if !strings.Contains(output, "source: ./templates/release.yml") {
		t.Errorf("output should preserve other entries, got:\n%s", output)
	}

	if !strings.Contains(output, "owner: babarot") {
		t.Errorf("output should preserve metadata, got:\n%s", output)
	}
}

func TestReplaceLiteralContent_SecondEntry(t *testing.T) {
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

	result, err := ReplaceLiteralContent([]byte(input), 0, "$.spec.files[1].content", newContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(result)

	if !strings.Contains(output, "first content") {
		t.Errorf("first entry should be unchanged, got:\n%s", output)
	}

	if !strings.Contains(output, "replaced second") {
		t.Errorf("second entry should be replaced, got:\n%s", output)
	}
}

func TestReplaceLiteralContent_InvalidDocIndex(t *testing.T) {
	input := `kind: FileSet
spec:
  files:
    - path: test.yaml
      content: |
        test
`
	_, err := ReplaceLiteralContent([]byte(input), 5, "$.spec.files[0].content", "new")
	if err == nil {
		t.Fatal("expected error for invalid doc index")
	}
}
