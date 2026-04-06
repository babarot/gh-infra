package fileset

import (
	"fmt"
	"strings"
)

type SegmentKind string

const (
	SegmentLiteral     SegmentKind = "literal"
	SegmentPlaceholder SegmentKind = "placeholder"
)

type RenderedSegment struct {
	Kind         SegmentKind
	Expr         string
	SourceText   string
	RenderedText string
}

type RenderedLine struct {
	Source         string
	Rendered       string
	Segments       []RenderedSegment
	HasPlaceholder bool
}

type RenderedTemplate struct {
	Source string
	Text   string
	Lines  []RenderedLine
}

// RenderTemplateWithTrace renders a template using the same repo/vars semantics
// as RenderTemplate, but returns per-line segment trace for simple substitution
// syntax used by import reverse mapping.
func RenderTemplateWithTrace(content string, repo string, vars map[string]string) (RenderedTemplate, error) {
	repoCtx := buildRepoContext(repo)

	expandedVars := make(map[string]string, len(vars))
	for k, v := range vars {
		rendered, err := execTemplate(v, TemplateContext{Repo: repoCtx})
		if err != nil {
			return RenderedTemplate{}, err
		}
		expandedVars[k] = rendered
	}

	sourceLines := SplitLinesKeepNewline(content)
	lines := make([]RenderedLine, 0, len(sourceLines))
	var rendered strings.Builder

	for _, sourceLine := range sourceLines {
		line, err := renderLineWithTrace(sourceLine, repoCtx, expandedVars)
		if err != nil {
			return RenderedTemplate{}, err
		}
		lines = append(lines, line)
		rendered.WriteString(line.Rendered)
	}

	return RenderedTemplate{
		Source: content,
		Text:   rendered.String(),
		Lines:  lines,
	}, nil
}

func renderLineWithTrace(line string, repo RepoContext, vars map[string]string) (RenderedLine, error) {
	var segments []RenderedSegment
	var rendered strings.Builder
	rest := line
	hasPlaceholder := false

	for {
		start := strings.Index(rest, delimLeft)
		if start < 0 {
			if rest != "" {
				segments = append(segments, RenderedSegment{
					Kind:         SegmentLiteral,
					SourceText:   rest,
					RenderedText: rest,
				})
				rendered.WriteString(rest)
			}
			break
		}

		if start > 0 {
			literal := rest[:start]
			segments = append(segments, RenderedSegment{
				Kind:         SegmentLiteral,
				SourceText:   literal,
				RenderedText: literal,
			})
			rendered.WriteString(literal)
		}

		end := strings.Index(rest[start+len(delimLeft):], delimRight)
		if end < 0 {
			return RenderedLine{}, fmt.Errorf("unterminated template expression")
		}
		end += start + len(delimLeft)

		sourceText := rest[start : end+len(delimRight)]
		expr := strings.TrimSpace(rest[start+len(delimLeft) : end])
		renderedText, err := resolveTemplateExpr(expr, repo, vars)
		if err != nil {
			return RenderedLine{}, err
		}

		segments = append(segments, RenderedSegment{
			Kind:         SegmentPlaceholder,
			Expr:         expr,
			SourceText:   sourceText,
			RenderedText: renderedText,
		})
		rendered.WriteString(renderedText)
		hasPlaceholder = true
		rest = rest[end+len(delimRight):]
	}

	return RenderedLine{
		Source:         line,
		Rendered:       rendered.String(),
		Segments:       segments,
		HasPlaceholder: hasPlaceholder,
	}, nil
}

func resolveTemplateExpr(expr string, repo RepoContext, vars map[string]string) (string, error) {
	switch expr {
	case ".Repo.Name":
		return repo.Name, nil
	case ".Repo.Owner":
		return repo.Owner, nil
	case ".Repo.FullName":
		return repo.FullName, nil
	default:
		if key, found := strings.CutPrefix(expr, ".Vars."); found {
			v, ok := vars[key]
			if !ok {
				return "", fmt.Errorf("unsupported template expression: %s", expr)
			}
			return v, nil
		}
		return "", fmt.Errorf("unsupported template expression: %s", expr)
	}
}

func SplitLinesKeepNewline(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
