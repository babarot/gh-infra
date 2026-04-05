package yamledit

import (
	"strings"
	"testing"
)

func TestSet_SingleDoc(t *testing.T) {
	data := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-repo
  owner: my-org
spec:
  description: old desc
  visibility: private
`)

	type spec struct {
		Description string `yaml:"description"`
		Visibility  string `yaml:"visibility"`
	}

	updated, err := Set(data, 0, "$.spec", spec{
		Description: "new desc",
		Visibility:  "public",
	})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "new desc") {
		t.Errorf("expected 'new desc' in output:\n%s", result)
	}
	if !strings.Contains(result, "public") {
		t.Errorf("expected 'public' in output:\n%s", result)
	}
	// Metadata should be preserved
	if !strings.Contains(result, "my-repo") {
		t.Errorf("expected 'my-repo' preserved in output:\n%s", result)
	}
}

func TestSet_MultiDoc_TargetOnly(t *testing.T) {
	data := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-a
  owner: my-org
spec:
  description: first
---
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-b
  owner: my-org
spec:
  description: second
`)

	type spec struct {
		Description string `yaml:"description"`
	}

	// Replace only in second document (index 1)
	updated, err := Set(data, 1, "$.spec", spec{Description: "updated-second"})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	result := string(updated)
	// First doc should be unchanged
	if !strings.Contains(result, "first") {
		t.Errorf("first doc should be unchanged:\n%s", result)
	}
	// Second doc should be updated
	if !strings.Contains(result, "updated-second") {
		t.Errorf("expected 'updated-second' in output:\n%s", result)
	}
}

func TestSet_InvalidPathSyntax(t *testing.T) {
	data := []byte(`kind: Repository
spec:
  description: test
`)
	type spec struct {
		Description string `yaml:"description"`
	}

	// Invalid path syntax should return an error
	_, err := Set(data, 0, "[[invalid", spec{Description: "x"})
	if err == nil {
		t.Error("expected error for invalid path syntax")
	}
}

func TestSet_DocIndexOutOfRange(t *testing.T) {
	data := []byte(`kind: Repository
spec:
  description: test
`)
	type spec struct {
		Description string `yaml:"description"`
	}

	_, err := Set(data, 5, "$.spec", spec{Description: "x"})
	if err == nil {
		t.Error("expected error for out-of-range doc index")
	}
}

func TestSetLiteral_LiteralBlock(t *testing.T) {
	data := []byte(`kind: File
spec:
  content: |
    old content
    line 2
`)

	updated, err := SetLiteral(data, 0, "$.spec.content", "new content\nline 2\n")
	if err != nil {
		t.Fatalf("SetLiteral error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "new content") {
		t.Errorf("expected 'new content' in output:\n%s", result)
	}
}

func TestDelete_PrunesEmptyParents(t *testing.T) {
	data := []byte(`spec:
  actions:
    selected_actions:
      patterns_allowed:
        - foo/bar@v1
`)

	updated, err := Delete(data, 0, "$.spec.actions.selected_actions.patterns_allowed")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	result := string(updated)
	if strings.Contains(result, "patterns_allowed") {
		t.Fatalf("expected patterns_allowed to be deleted:\n%s", result)
	}
	if strings.Contains(result, "selected_actions") {
		t.Fatalf("expected selected_actions to be pruned:\n%s", result)
	}
	if strings.Contains(result, "actions:") {
		t.Fatalf("expected actions to be pruned when empty:\n%s", result)
	}
}

func TestExists(t *testing.T) {
	data := []byte(`spec:
  repositories:
    - name: repo-a
      spec:
        description: hello
`)

	ok, err := Exists(data, 0, "$.spec.repositories[0].spec.description")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !ok {
		t.Fatal("expected path to exist")
	}

	ok, err = Exists(data, 0, "$.spec.repositories[0].spec.visibility")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if ok {
		t.Fatal("expected path to be missing")
	}
}

func TestMerge_PreservesCommentsAndUntouchedSiblings(t *testing.T) {
	data := []byte(`spec:
  # keep this comment
  topics: [go, cli]
  features:
    issues: true
    wiki: false
`)

	updated, err := Merge(data, 0, "$.spec.features", map[string]any{
		"issues": false,
	})
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "# keep this comment") {
		t.Fatalf("expected comment to be preserved:\n%s", result)
	}
	if !strings.Contains(result, "topics: [go, cli]") {
		t.Fatalf("expected untouched flow-style topics to be preserved:\n%s", result)
	}
	if !strings.Contains(result, "issues: false") {
		t.Fatalf("expected merged field to be updated:\n%s", result)
	}
	if !strings.Contains(result, "wiki: false") {
		t.Fatalf("expected untouched sibling field to remain:\n%s", result)
	}
}

func TestSetAndExists_Integration(t *testing.T) {
	data := []byte("spec:\n  description: old\n")

	updated, err := Set(data, 0, "$.spec.description", "new")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if !strings.Contains(string(updated), "description: new") {
		t.Fatalf("expected Set to update content:\n%s", string(updated))
	}

	ok, err := Exists(updated, 0, "$.spec.description")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if !ok {
		t.Fatal("expected Exists to find updated path")
	}
}
