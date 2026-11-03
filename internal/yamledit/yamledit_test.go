package yamledit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	goyaml "github.com/goccy/go-yaml"
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

// SetLiteral writes content exactly, at the node the path names and
// nowhere else, wherever the node is and whatever it held.
func TestSetLiteral_RoundTrip(t *testing.T) {
	docs := map[string]struct {
		src  string
		doc  int
		path []any // keys and indexes to the value
	}{
		"plain":            {"files:\n  - path: a\n    content: mit\n  - path: b\n    content: mit\n", 0, []any{"files", 1, "content"}},
		"literal":          {"files:\n  - path: a\n    content: |\n      mit\n  - path: b\n    content: |\n      mit\n", 0, []any{"files", 1, "content"}},
		"literal at end":   {"a:\n  content: |\n    mit\n", 0, []any{"a", "content"}},
		"first key":        {"files:\n  - content: mit\n    path: a\n", 0, []any{"files", 0, "content"}},
		"comment":          {"files:\n  - content: mit # keep\n    path: a\n", 0, []any{"files", 0, "content"}},
		"second document":  {"x: |\n  mit\n---\nx: |\n  mit\n", 1, []any{"x"}},
		"no final newline": {"a:\n  content: mit", 0, []any{"a", "content"}},
		"sequence item":    {"patches:\n  - old\n  - other\n", 0, []any{"patches", 0}},
		"flow":             {"file: {content: old, path: a}\n", 0, []any{"file", "content"}},
		"next line":        {"a:\n  content:\n    mit\n  n: 1\n", 0, []any{"a", "content"}},
		"folded":           {"a:\n  content: >\n    mit\n  n: 1\n", 0, []any{"a", "content"}},
		"strip":            {"a:\n  content: |-\n    mit\n    x\n  n: 1\n", 0, []any{"a", "content"}},
		"quoted":           {"a:\n  content: \"mit\"\n", 0, []any{"a", "content"}},
		"unicode key":      {"説明: mit\nn: 1\n", 0, []any{"説明"}},
		"trailing spaces":  {"a:\n  content: |\n    old  \n    x \n\n  n: 1\n", 0, []any{"a", "content"}},
		"spaces line":      {"content: |\n  old\n    \nnext: keep\n", 0, []any{"content"}},
		"keep at end":      {"a:\n  content: |+\n    old\n\n\n", 0, []any{"a", "content"}},
		"spaces at end":    {"content: |\n  old\n    ", 0, []any{"content"}},
		"unicode space":    {"content: old\u00a0\nn: 1\n", 0, []any{"content"}},
	}
	contents := []string{
		"apache\n", "apache", "a\nb", "a\nb\n", "a\nb\n\n", "x\n\n\n", "  indented\nx\n", "\tx\ny\n", "x\n\n  y\n",
		"key: value\n# not a comment\n", "\n", "", "a\r\nb\r\n", "a\tb", "bell\a\n", "é\nü\n", " lead", "123", "a: b", "- x\n- y\n", "a\nb ", "a \nb \n", "a\x00b", "a\u0080b\n", "a\u00a0b\nc\n", "? x", ".inf", "null", "true", "~",
	}
	for name, d := range docs {
		for _, content := range contents {
			yamlPath := "$"
			for _, p := range d.path {
				if k, ok := p.(string); ok {
					yamlPath += "." + k
				} else {
					yamlPath += fmt.Sprintf("[%d]", p)
				}
			}
			out, err := SetLiteral([]byte(d.src), d.doc, yamlPath, content)
			if err != nil {
				t.Errorf("%s, %q: %v", name, content, err)
				continue
			}
			want := decodeAll(t, []byte(d.src))
			setValue(t, want[d.doc], d.path, content)
			if got := decodeAll(t, out); !reflect.DeepEqual(got, want) {
				t.Errorf("%s, %q:\n got %#v\nwant %#v\n%s", name, content, got, want, out)
			}
		}
	}
}

func decodeAll(t *testing.T, data []byte) []any {
	t.Helper()
	dec := goyaml.NewDecoder(bytes.NewReader(data))
	var docs []any
	for {
		var v any
		if err := dec.Decode(&v); errors.Is(err, io.EOF) {
			return docs
		} else if err != nil {
			t.Fatalf("decode: %v\n%s", err, data)
		}
		docs = append(docs, v)
	}
}

// setValue sets the value at path (keys and indexes) in v to content.
func setValue(t *testing.T, v any, path []any, content string) {
	t.Helper()
	for i, p := range path {
		switch p := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("%v: not a mapping", path[:i])
			}
			if i == len(path)-1 {
				m[p] = content
				return
			}
			v = m[p]
		case int:
			s, ok := v.([]any)
			if !ok {
				t.Fatalf("%v: not a sequence", path[:i])
			}
			if i == len(path)-1 {
				s[p] = content
				return
			}
			v = s[p]
		}
	}
}

// A comment after the replaced value stays on its line.
func TestSetLiteral_KeepsTrailingComment(t *testing.T) {
	for _, tt := range []struct{ src, content, want string }{
		{"files:\n  - content: mit # keep\n    path: a\n", "apache\n", "files:\n  - content: | # keep\n      apache\n    path: a\n"},
		{"a:\n  content: | # keep\n    mit\n  b: 1\n", "apache\n", "a:\n  content: | # keep\n    apache\n  b: 1\n"},
		{"a:\n  content: mit # keep\n", "\n", "a:\n  content: \"\\n\" # keep\n"},
	} {
		path := "$.a.content"
		if strings.HasPrefix(tt.src, "files") {
			path = "$.files[0].content"
		}
		out, err := SetLiteral([]byte(tt.src), 0, path, tt.content)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != tt.want {
			t.Errorf("got\n%s\nwant\n%s", out, tt.want)
		}
	}
}

// Everything but the value stays byte for byte, and an anchor on the value
// is kept, so its aliases read the new value.
func TestSetLiteral_KeepsTheRest(t *testing.T) {
	src := "name: x   # spaced comment\na:\n  content: &c mit\nb: *c\n---\nother: |+\n  keep\n\n"
	out, err := SetLiteral([]byte(src), 0, "$.a.content", "l1\nl2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "name: x   # spaced comment\na:\n  content: &c |\n    l1\n    l2\nb: *c\n---\nother: |+\n  keep\n\n"
	if string(out) != want {
		t.Errorf("got\n%q\nwant\n%q", out, want)
	}
	docs := decodeAll(t, out)
	if m, ok := docs[0].(map[string]any); !ok || m["b"] != "l1\nl2\n" {
		t.Errorf("alias b = %v, want the new value", docs[0])
	}
}

// Blank lines after a block scalar stay, as empty lines, so the layout of
// the file does not change.
func TestSetLiteral_KeepsBlankLinesAfterBlock(t *testing.T) {
	out, err := SetLiteral([]byte("a:\n  content: |\n    old\n\n  n: 1\n"), 0, "$.a.content", "new\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "a:\n  content: |\n    new\n    x\n\n  n: 1\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// An alias is not replaced: the value belongs to the anchor, which other
// aliases read too.
func TestSetLiteral_RejectsAlias(t *testing.T) {
	src := "b: &an mit\na:\n  content: *an\n"
	if _, err := SetLiteral([]byte(src), 0, "$.a.content", "new\n"); err == nil || !strings.Contains(err.Error(), "is an alias") {
		t.Errorf("err = %v, want an alias error", err)
	}
}

// Characters YAML does not allow written as they are (C0 and C1 control
// characters) are escaped in a quoted scalar, even where goccy/go-yaml
// would read them back.
func TestSetLiteral_EscapesControlCharacters(t *testing.T) {
	for _, content := range []string{"a\u0080b\n", "a\u009fb\nc\n", "a\x00b", "a\x1bb\nc\n"} {
		out, err := SetLiteral([]byte("a: x\n"), 0, "$.a", content)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsFunc(string(out), func(r rune) bool { return r != '\n' && (r < 0x20 || r >= 0x7f && r <= 0x9f) }) {
			t.Errorf("%q: output has a raw control character: %q", content, out)
		}
	}
}
