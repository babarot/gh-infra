package fileset

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/babarot/gh-infra/internal/manifest"
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
	return renderMessage(msg, repo, sourceURL, nil)
}

// renderMessage renders commit_message, pr_title or pr_body with .Repo,
// .Source and .Files.
func renderMessage(msg, repo, sourceURL string, files []FileContext) (string, error) {
	return execTemplate(msg, messageContext{
		Repo:   buildRepoContext(repo),
		Source: SourceContext{URL: sourceURL},
		Files:  files,
	})
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

func execTemplate(text string, ctx any) (string, error) {
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

// CheckMessages renders the FileSet's commit_message, pr_title and pr_body,
// and the descriptions of its files, for target, so a template error is
// reported by plan and validate rather than partway through apply (e.g.
// after the PR branch was committed to). changes are the target's planned
// changes of the FileSet; .Files is built from them as apply builds it, or,
// when none has anything to do (and for validate, which plans nothing),
// from every file of the FileSet as if each were created. The source URL is
// empty, as apply is the only command that has it.
func CheckMessages(fs *manifest.FileSet, target manifest.FileSetRepository, changes []Change) error {
	repo := fs.RepoFullName(target.Name)
	files, err := buildFiles(changes, repo, "")
	if err == nil && len(files) == 0 {
		var entries []manifest.FileEntry
		if entries, err = fs.FilesFor(target); err == nil {
			files, err = entryFiles(entries, repo)
		}
	}
	if err != nil {
		return fmt.Errorf("%s: %w", repo, err)
	}
	for _, m := range []struct{ field, text string }{
		{"commit_message", fs.Spec.CommitMessage},
		{"pr_title", fs.Spec.PRTitle},
		{"pr_body", fs.Spec.PRBody},
	} {
		if !HasTemplate(m.text, nil) {
			continue
		}
		if _, err := renderMessage(m.text, repo, "", files); err != nil {
			return fmt.Errorf("%s for %s: %w", m.field, repo, err)
		}
	}
	return nil
}
