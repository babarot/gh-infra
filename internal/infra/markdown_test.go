package infra

import (
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

func renderMarkdown(t *testing.T, doc Document) string {
	t.Helper()
	var b strings.Builder
	if err := doc.WriteMarkdown(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWriteMarkdown(t *testing.T) {
	out := renderMarkdown(t, testPlanResult().PlanDocument(false))

	for _, want := range []string{
		"### Plan: 3 to create, 3 to update, 1 to destroy\n",
		"This plan is incomplete",
		"<details><summary><code>o/a</code>: 2 to create, 3 to update</summary>",
		"<details><summary><code>o/b</code>: 1 to destroy</summary>",
		"| ~ | Repository |  | <code>description</code> | <code>old</code> | <code>new</code> |",
		"| ~ | Repository |  | <code>features.wiki</code> | <code>true</code> | <code>false</code> |",
		"| + | Ruleset | <code>main</code> | <code>enforcement</code> |  | <code>active</code> |",
		"| + | Secret | <code>TOKEN</code> |  |  | <code>(new)</code> |",
		"| ~ | File | <code>a.sh</code> | +1 -1, mode 100644 -&gt; 100755 |  |  |",
		"| - | Label | <code>old-label</code> |  | <code>#ffffff</code> |  |",
		"#### Warnings\n\n- label_sync is deprecated\n- <code>o/a</code>: Changing file modes",
		"#### Errors\n\n- <code>o/d</code>: fetch secrets: forbidden (HTTP 403)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown does not contain %q\n---\n%s", want, out)
		}
	}

	// Repositories appear in plan order, and every details block is closed.
	if strings.Index(out, "<code>o/a</code>:") > strings.Index(out, "<code>o/c</code>:") {
		t.Error("o/a should come before o/c")
	}
	if strings.Count(out, "<details>") != strings.Count(out, "</details>") {
		t.Errorf("unbalanced details blocks\n%s", out)
	}
	if strings.Contains(out, "Diff of") {
		t.Error("diffs should only appear with showDiff")
	}
}

func TestWriteMarkdown_NoChanges(t *testing.T) {
	out := renderMarkdown(t, (&PlanResult{}).PlanDocument(false))
	if out != "### Plan: no changes\n" {
		t.Errorf("markdown = %q", out)
	}
}

func TestWriteMarkdown_EscapesValues(t *testing.T) {
	result := &PlanResult{
		RepoChanges: []repository.Change{{
			Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: "o/a",
			Field: "description", OldValue: "a | b <script>", NewValue: "line1\nline2 `x`",
		}},
	}
	out := renderMarkdown(t, result.PlanDocument(false))
	if !strings.Contains(out, "<code>a &#124; b &lt;script&gt;</code>") {
		t.Errorf("pipe and HTML not escaped:\n%s", out)
	}
	if !strings.Contains(out, "<code>line1<br>line2 `x`</code>") {
		t.Errorf("newline not turned into <br>:\n%s", out)
	}
}

func TestWriteMarkdown_DiffFence(t *testing.T) {
	result := &PlanResult{
		FileChanges: []fileset.Change{{
			Type: fileset.ChangeUpdate, Target: "o/a", Path: "README.md",
			Current: "old\n", Desired: "````\ncode\n````\n",
		}},
	}
	out := renderMarkdown(t, result.PlanDocument(true))
	if !strings.Contains(out, "<details><summary>Diff of <code>README.md</code></summary>\n\n`````diff\n") {
		t.Errorf("diff block should use a fence longer than the content's backticks:\n%s", out)
	}
	if !strings.Contains(out, "+````\n") || !strings.HasSuffix(strings.TrimSpace(out), "</details>") {
		t.Errorf("unexpected diff block:\n%s", out)
	}
}

func TestCellTruncatesLongValues(t *testing.T) {
	got := cell(strings.Repeat("a", 500))
	if n := len([]rune(got)); n != maxCellRunes {
		t.Errorf("cell length = %d, want %d", n, maxCellRunes)
	}
}
