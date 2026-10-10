package fileset

import (
	"bytes"
	"maps"
	"strings"
	"text/template"
)

// Template delimiters — <% %> avoids conflicts with Go template {{ }},
// GitHub Actions ${{ }}, GoReleaser {{ .Version }}, Helm {{ .Release }}, etc.
const (
	delimLeft  = "<%"
	delimRight = "%>"
)

// TemplateContext is the data available to templates.
type TemplateContext struct {
	Repo   RepoContext
	Vars   map[string]string
	Source SourceContext
}

// SourceContext provides source provenance metadata to templates.
type SourceContext struct {
	URL string
}

// RepoContext provides repository metadata to templates.
type RepoContext struct {
	Name     string // "gomi"
	Owner    string // "babarot"
	FullName string // "babarot/gomi"
}

// RenderTemplate applies template rendering to file content using <% %> delimiters.
// vars values are themselves templates (expanded with .Repo context first).
func RenderTemplate(content string, repo string, vars map[string]string) (string, error) {
	repoCtx := buildRepoContext(repo)

	// Pass 1: Expand vars values (they can reference .Repo)
	expandedVars := make(map[string]string, len(vars))
	for k, v := range vars {
		rendered, err := execTemplate(v, TemplateContext{Repo: repoCtx})
		if err != nil {
			return "", err
		}
		expandedVars[k] = rendered
	}

	// Pass 2: Expand content (can reference .Repo and .Vars)
	ctx := TemplateContext{Repo: repoCtx, Vars: expandedVars}
	return execTemplate(content, ctx)
}

// RenderCommitMessage renders a commit or PR message template with Repo and Source context.
// Vars are not available in commit messages — use .Repo and .Source fields only.
func RenderCommitMessage(msg string, repo string, sourceURL string) (string, error) {
	repoCtx := buildRepoContext(repo)
	ctx := TemplateContext{
		Repo:   repoCtx,
		Source: SourceContext{URL: sourceURL},
	}
	return execTemplate(msg, ctx)
}

// HasTemplate returns true if the content uses <% %> template syntax or has vars.
func HasTemplate(content string, vars map[string]string) bool {
	if len(vars) > 0 {
		return true
	}
	return strings.Contains(content, delimLeft)
}

func buildRepoContext(repo string) RepoContext {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		return RepoContext{
			Owner:    parts[0],
			Name:     parts[1],
			FullName: repo,
		}
	}
	return RepoContext{Name: repo, FullName: repo}
}

func execTemplate(text string, ctx TemplateContext) (string, error) {
	tmpl, err := template.New("").
		Delims(delimLeft, delimRight).
		Option("missingkey=error").
		Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// copyVars creates a shallow copy of a string map to avoid data races.
func copyVars(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	maps.Copy(cp, m)
	return cp
}
