package importer

import (
	"strings"
	"unicode"

	"github.com/babarot/gh-infra/internal/fileset"
)

func prepareTemplateReverse(templateContent, repo string, vars map[string]string) (fileset.RenderedTemplate, bool) {
	trace, err := fileset.RenderTemplateWithTrace(templateContent, repo, vars)
	if err != nil {
		return fileset.RenderedTemplate{}, false
	}
	return trace, true
}

// reverseRenderedTemplate rebuilds template source from rendered remote content.
// Placeholder-rendered spans are used as anchors; remote literal text around them
// is preserved and placeholder source text is reinserted.
func reverseRenderedTemplate(trace fileset.RenderedTemplate, remote string) (string, bool) {
	remoteLines := fileset.SplitLinesKeepNewline(remote)
	var out strings.Builder
	remoteIdx := 0

	for _, line := range trace.Lines {
		if !line.HasPlaceholder {
			continue
		}

		reconstructed, matchedIdx, ok := reverseTemplateLine(line, remoteLines, remoteIdx)
		if !ok {
			return "", false
		}

		for ; remoteIdx < matchedIdx; remoteIdx++ {
			out.WriteString(remoteLines[remoteIdx])
		}
		out.WriteString(reconstructed)
		remoteIdx = matchedIdx + 1
	}

	for ; remoteIdx < len(remoteLines); remoteIdx++ {
		out.WriteString(remoteLines[remoteIdx])
	}

	return out.String(), true
}

func reverseTemplateLine(line fileset.RenderedLine, remoteLines []string, start int) (string, int, bool) {
	for idx := start; idx < len(remoteLines); idx++ {
		if reconstructed, ok := reconstructLineFromRemote(line, remoteLines[idx]); ok {
			return reconstructed, idx, true
		}
	}
	return "", -1, false
}

func reconstructLineFromRemote(line fileset.RenderedLine, remote string) (string, bool) {
	firstPlaceholder, lastPlaceholder := placeholderBounds(line.Segments)
	if firstPlaceholder < 0 {
		return "", false
	}

	var out strings.Builder
	pos := 0

	for i, seg := range line.Segments {
		switch seg.Kind {
		case fileset.SegmentLiteral:
			if seg.RenderedText == "" {
				continue
			}
			if isFlexibleEdgeLiteral(i, firstPlaceholder, lastPlaceholder, seg.RenderedText) {
				continue
			}
			if !strings.HasPrefix(remote[pos:], seg.RenderedText) {
				return "", false
			}
			out.WriteString(seg.SourceText)
			pos += len(seg.RenderedText)

		case fileset.SegmentPlaceholder:
			idx := strings.Index(remote[pos:], seg.RenderedText)
			if idx < 0 {
				return "", false
			}
			if idx > 0 && i != firstPlaceholder {
				return "", false
			}
			idx += pos
			out.WriteString(remote[pos:idx])
			out.WriteString(seg.SourceText)
			pos = idx + len(seg.RenderedText)
		}
	}

	if hasRigidTrailingLiteral(line.Segments, lastPlaceholder) && pos != len(remote) {
		return "", false
	}

	out.WriteString(remote[pos:])
	return out.String(), true
}

func placeholderBounds(segments []fileset.RenderedSegment) (int, int) {
	first := -1
	last := -1
	for i, seg := range segments {
		if seg.Kind != fileset.SegmentPlaceholder {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	return first, last
}

func isFlexibleEdgeLiteral(index, firstPlaceholder, lastPlaceholder int, literal string) bool {
	if index != firstPlaceholder-1 && index != lastPlaceholder+1 {
		return false
	}
	return isFlexibleLiteral(literal)
}

func hasRigidTrailingLiteral(segments []fileset.RenderedSegment, lastPlaceholder int) bool {
	idx := lastPlaceholder + 1
	if idx < 0 || idx >= len(segments) {
		return false
	}
	seg := segments[idx]
	return seg.Kind == fileset.SegmentLiteral && seg.RenderedText != "" && !isFlexibleLiteral(seg.RenderedText)
}

// isFlexibleLiteral allows punctuation-only edge literals to drift around a
// placeholder while still preserving the placeholder itself, e.g.
// "/<% .Repo.Name %>" -> "<% .Repo.Name %>*". Alphanumeric literals stay rigid.
func isFlexibleLiteral(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsSpace(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
