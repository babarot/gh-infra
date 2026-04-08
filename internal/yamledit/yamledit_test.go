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

func TestSetLiteral_SpecialCharsInContent(t *testing.T) {
	data := []byte(`spec:
  files:
    - path: CODEOWNERS
      content: |
        old owner
    - path: ci.yml
      content: |
        name: CI
`)

	// Content with * at start of line (YAML alias char) and / at start
	newContent := "* @babarot @team-platform\n/docs/ @babarot\n"
	updated, err := SetLiteral(data, 0, "$.spec.files[0].content", newContent)
	if err != nil {
		t.Fatalf("SetLiteral error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "* @babarot") {
		t.Errorf("expected '* @babarot' in output:\n%s", result)
	}
	if !strings.Contains(result, "/docs/ @babarot") {
		t.Errorf("expected '/docs/ @babarot' in output:\n%s", result)
	}
	// Second file should be untouched
	if !strings.Contains(result, "name: CI") {
		t.Errorf("second file content lost:\n%s", result)
	}

	// Verify result is valid YAML
	_, err = Set([]byte(result), 0, "$.spec.files[1].content", "name: CI updated\n")
	if err != nil {
		t.Fatalf("result YAML is unparseable for subsequent edit: %v", err)
	}
}

func TestSetLiteral_StringNodeToLiteralBlock(t *testing.T) {
	// Simulates the WritePatch flow: patches[0] starts as a quoted StringNode
	// (from Merge) and SetLiteral converts it to a literal block scalar.
	data := []byte(`spec:
  files:
    - path: ci.yml
      content: |
        name: CI
        on: [push]
      patches:
        - "--- a/ci.yml\n+++ b/ci.yml\n"
    - path: other.yml
      content: |
        name: Other
`)

	// Replace the quoted string with a multiline unified diff
	newPatch := "--- a/ci.yml\n+++ b/ci.yml\n@@ -2 +2 @@\n-on: [push]\n+on: [push, pull_request]\n"
	updated, err := SetLiteral(data, 0, "$.spec.files[0].patches[0]", newPatch)
	if err != nil {
		t.Fatalf("SetLiteral error: %v", err)
	}

	result := string(updated)
	t.Logf("Result:\n%s", result)

	if !strings.Contains(result, "+on: [push, pull_request]") {
		t.Errorf("expected patch content with + line in output:\n%s", result)
	}
	if !strings.Contains(result, "-on: [push]") {
		t.Errorf("expected patch content with - line in output:\n%s", result)
	}
	// Other file should be untouched
	if !strings.Contains(result, "name: Other") {
		t.Errorf("second file content lost:\n%s", result)
	}

	// Verify result parses for subsequent edits
	_, err = SetLiteral([]byte(result), 0, "$.spec.files[1].content", "name: Other updated\n")
	if err != nil {
		t.Fatalf("result YAML is unparseable for subsequent edit: %v", err)
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

func TestDelete_PreservesCommentsOnSiblingNodes(t *testing.T) {
	data := []byte(`spec:
  actions:
    # keep this
    enabled: true
    fork_pr_approval: first_time_contributors
`)

	updated, err := Delete(data, 0, "$.spec.actions.fork_pr_approval")
	if err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	result := string(updated)
	if strings.Contains(result, "fork_pr_approval") {
		t.Fatalf("expected target field to be deleted:\n%s", result)
	}
	if !strings.Contains(result, "# keep this") {
		t.Fatalf("expected sibling comment to be preserved:\n%s", result)
	}
	if !strings.Contains(result, "enabled: true") {
		t.Fatalf("expected sibling field to remain:\n%s", result)
	}
}

func TestMerge_PreservesFlowStyleSiblingMap(t *testing.T) {
	data := []byte(`spec:
  actions: {enabled: true, allowed_actions: all}
  description: hello
`)

	updated, err := Merge(data, 0, "$.spec", map[string]any{
		"homepage": "https://example.com",
	})
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "actions: {enabled: true, allowed_actions: all}") {
		t.Fatalf("expected untouched flow-style map to be preserved:\n%s", result)
	}
	if !strings.Contains(result, "homepage: https://example.com") {
		t.Fatalf("expected merged field to be added:\n%s", result)
	}
}

func TestSet_ReplacesFieldWithHeadComment(t *testing.T) {
	data := []byte(`spec:
  # field comment
  description: old
  visibility: private
`)

	updated, err := Set(data, 0, "$.spec.description", "new")
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	result := string(updated)
	if !strings.Contains(result, "# field comment") {
		t.Fatalf("expected head comment to be preserved:\n%s", result)
	}
	if !strings.Contains(result, "description: new") {
		t.Fatalf("expected target field to be replaced:\n%s", result)
	}
	if !strings.Contains(result, "visibility: private") {
		t.Fatalf("expected sibling field to remain:\n%s", result)
	}
}
