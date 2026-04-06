package fileset

import (
	"strings"
	"testing"
)

func TestRenderTemplate_RepoName(t *testing.T) {
	result, err := RenderTemplate("BINARY = <% .Repo.Name %>", "babarot/gomi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "BINARY = gomi" {
		t.Errorf("got %q, want %q", result, "BINARY = gomi")
	}
}

func TestRenderTemplate_RepoOwner(t *testing.T) {
	result, err := RenderTemplate("OWNER = <% .Repo.Owner %>", "babarot/gomi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "OWNER = babarot" {
		t.Errorf("got %q, want %q", result, "OWNER = babarot")
	}
}

func TestRenderTemplate_RepoFullName(t *testing.T) {
	result, err := RenderTemplate("MODULE = github.com/<% .Repo.FullName %>", "babarot/gomi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "MODULE = github.com/babarot/gomi" {
		t.Errorf("got %q, want %q", result, "MODULE = github.com/babarot/gomi")
	}
}

func TestRenderTemplate_Vars(t *testing.T) {
	vars := map[string]string{"binary": "my-tool"}
	result, err := RenderTemplate("BINARY = <% .Vars.binary %>", "owner/repo", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "BINARY = my-tool" {
		t.Errorf("got %q, want %q", result, "BINARY = my-tool")
	}
}

func TestRenderTemplate_VarsWithRepoTemplate(t *testing.T) {
	vars := map[string]string{"binary": "<% .Repo.Name %>"}
	result, err := RenderTemplate("BINARY = <% .Vars.binary %>", "babarot/gomi", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "BINARY = gomi" {
		t.Errorf("got %q, want %q", result, "BINARY = gomi")
	}
}

func TestRenderTemplate_NoTemplate(t *testing.T) {
	content := "just plain text without any templates"
	result, err := RenderTemplate(content, "owner/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != content {
		t.Errorf("got %q, want %q", result, content)
	}
}

func TestRenderTemplate_MissingKey(t *testing.T) {
	_, err := RenderTemplate("<% .Vars.nonexistent %>", "owner/repo", nil)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRenderTemplate_GitHubActionsPreserved(t *testing.T) {
	content := `name: CI
on:
  push:
    branches: [main]
jobs:
  build:
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo "Building <% .Repo.Name %>"
      - run: echo "Ref is ${{ github.ref }}"
        env:
          TOKEN: ${{ secrets.TOKEN }}`

	result, err := RenderTemplate(content, "babarot/gomi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Building gomi") {
		t.Errorf("expected .Repo.Name to be expanded, got:\n%s", result)
	}
	if !strings.Contains(result, "${{ github.ref }}") {
		t.Errorf("expected ${{ github.ref }} to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "${{ secrets.TOKEN }}") {
		t.Errorf("expected ${{ secrets.TOKEN }} to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "${{ matrix.os }}") {
		t.Errorf("expected ${{ matrix.os }} to be preserved, got:\n%s", result)
	}
}

func TestRenderTemplate_GoReleaserPreserved(t *testing.T) {
	content := `main: ./cmd/<% .Repo.Name %>/
ldflags:
  - -X main.version=v{{ .Version }}
  - -X main.revision={{ .ShortCommit }}`

	result, err := RenderTemplate(content, "babarot/gomi", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "main: ./cmd/gomi/") {
		t.Errorf("expected .Repo.Name to be expanded, got:\n%s", result)
	}
	if !strings.Contains(result, "{{ .Version }}") {
		t.Errorf("expected GoReleaser {{ .Version }} to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "{{ .ShortCommit }}") {
		t.Errorf("expected GoReleaser {{ .ShortCommit }} to be preserved, got:\n%s", result)
	}
}

func TestHasTemplate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		vars    map[string]string
		want    bool
	}{
		{"gh-infra template", "<% .Repo.Name %>", nil, true},
		{"plain text", "plain text", nil, false},
		{"with vars map", "plain text", map[string]string{"k": "v"}, true},
		{"empty", "", nil, false},
		{"goreleaser template", "{{ .Version }}", nil, false},
		{"github actions", "${{ github.ref }}", nil, false},
		{"helm template", "{{ .Release.Name }}", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasTemplate(tt.content, tt.vars)
			if got != tt.want {
				t.Errorf("HasTemplate(%q, %v) = %v, want %v", tt.content, tt.vars, got, tt.want)
			}
		})
	}
}

func TestCopyVars(t *testing.T) {
	original := map[string]string{"a": "1", "b": "2"}
	copied := copyVars(original)
	copied["a"] = "modified"
	if original["a"] != "1" {
		t.Error("copyVars did not create an independent copy")
	}
}

func TestCopyVars_Nil(t *testing.T) {
	if copyVars(nil) != nil {
		t.Error("copyVars(nil) should return nil")
	}
}

func TestRenderTemplateWithTrace(t *testing.T) {
	rendered, err := RenderTemplateWithTrace("module github.com/<% .Repo.FullName %>\nGO=<% .Vars.go_version %>\n", "babarot/gomi", map[string]string{
		"go_version": "1.26.1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rendered.Text != "module github.com/babarot/gomi\nGO=1.26.1\n" {
		t.Fatalf("Text = %q", rendered.Text)
	}
	if len(rendered.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(rendered.Lines))
	}
	if !rendered.Lines[0].HasPlaceholder || !rendered.Lines[1].HasPlaceholder {
		t.Fatal("expected both lines to be placeholder-backed")
	}
	if got := rendered.Lines[0].Segments[1].Expr; got != ".Repo.FullName" {
		t.Fatalf("first placeholder expr = %q", got)
	}
	if got := rendered.Lines[1].Segments[1].Expr; got != ".Vars.go_version" {
		t.Fatalf("second placeholder expr = %q", got)
	}
}

func TestRenderTemplateWithTrace_UnsupportedSyntax(t *testing.T) {
	_, err := RenderTemplateWithTrace("<% if .Repo.Name %>ok<% end %>\n", "babarot/gomi", nil)
	if err == nil {
		t.Fatal("expected error for unsupported syntax")
	}
}
