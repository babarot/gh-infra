package importer

import (
	"regexp"
	"strings"
)

var (
	templateExprPattern = regexp.MustCompile(`^\.(Repo\.(FullName|Owner|Name)|Vars\.[A-Za-z0-9_]+)$`)
)

// reverseTemplateContent rebuilds template source from rendered remote content.
// Supported template lines are treated as anchors: the matching remote line is
// replaced with the original template line, while the surrounding non-template
// lines come from the remote content. This lets literal-only drift be imported
// even when GitHub has added or removed extra lines.
func reverseTemplateContent(templateContent, remote string) (string, bool) {
	templateLines := splitKeepNewline(templateContent)
	remoteLines := splitKeepNewline(remote)

	var out strings.Builder
	remoteIdx := 0
	for _, templateLine := range templateLines {
		if !strings.Contains(templateLine, "<%") {
			continue
		}

		pattern, ok := compileTemplateLinePattern(templateLine)
		if !ok {
			return "", false
		}

		matched := -1
		for i := remoteIdx; i < len(remoteLines); i++ {
			if pattern.MatchString(remoteLines[i]) {
				matched = i
				break
			}
		}
		if matched < 0 {
			return "", false
		}

		for ; remoteIdx < matched; remoteIdx++ {
			out.WriteString(remoteLines[remoteIdx])
		}
		out.WriteString(templateLine)
		remoteIdx = matched + 1
	}

	for ; remoteIdx < len(remoteLines); remoteIdx++ {
		out.WriteString(remoteLines[remoteIdx])
	}

	return out.String(), true
}

func supportsTemplateReverse(templateContent string) bool {
	for _, line := range splitKeepNewline(templateContent) {
		if !strings.Contains(line, "<%") {
			continue
		}
		if _, ok := compileTemplateLinePattern(line); !ok {
			return false
		}
	}
	return true
}

func compileTemplateLinePattern(line string) (*regexp.Regexp, bool) {
	var pattern strings.Builder
	pattern.WriteString("^")

	rest := line
	seenPlaceholder := false
	for {
		start := strings.Index(rest, "<%")
		if start < 0 {
			pattern.WriteString(regexp.QuoteMeta(rest))
			break
		}

		pattern.WriteString(regexp.QuoteMeta(rest[:start]))
		end := strings.Index(rest[start+2:], "%>")
		if end < 0 {
			return nil, false
		}
		end += start + 2

		expr := strings.TrimSpace(rest[start+2 : end])
		if !templateExprPattern.MatchString(expr) {
			return nil, false
		}
		pattern.WriteString(".*?")
		seenPlaceholder = true
		rest = rest[end+2:]
	}

	if !seenPlaceholder {
		return nil, false
	}

	pattern.WriteString("$")
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, false
	}
	return re, true
}

func splitKeepNewline(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
