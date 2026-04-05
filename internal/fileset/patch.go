package fileset

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/pmezard/go-difflib/difflib"
)

// ApplyPatches applies a sequence of unified diff patches to content.
// Each patch is parsed and applied in order; the output of one becomes the input of the next.
func ApplyPatches(content string, patches []string) (string, error) {
	for i, p := range patches {
		files, _, err := gitdiff.Parse(strings.NewReader(p))
		if err != nil {
			return "", fmt.Errorf("patches[%d]: %w", i, wrapPatchError("parse", err))
		}
		if len(files) == 0 {
			continue
		}
		var out bytes.Buffer
		if err := gitdiff.Apply(&out, strings.NewReader(content), files[0]); err != nil {
			return "", fmt.Errorf("patches[%d]: %w", i, wrapPatchError("apply", err))
		}
		content = out.String()
	}
	return content, nil
}

// GeneratePatch generates a unified diff patch between base and desired content.
// The patch can be applied to baseContent to produce desiredContent.
// Returns an empty string if the contents are identical.
func GeneratePatch(baseContent, desiredContent, filePath string) (string, error) {
	baseNorm := ensureTrailingNewline(baseContent)
	desiredNorm := ensureTrailingNewline(desiredContent)

	if baseNorm == desiredNorm {
		return "", nil
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        splitLines(baseNorm),
		B:        splitLines(desiredNorm),
		FromFile: "a/" + filePath,
		ToFile:   "b/" + filePath,
		Context:  3,
	})
	if err != nil {
		return "", fmt.Errorf("generate diff for %s: %w", filePath, err)
	}

	if diff == "" {
		return "", nil
	}

	// Round-trip validation: apply the generated patch to base and verify it matches desired.
	result, err := ApplyPatches(baseNorm, []string{diff})
	if err != nil {
		return "", fmt.Errorf("round-trip validation failed for %s: generated patch cannot be applied: %w", filePath, err)
	}
	if strings.TrimRight(result, "\n") != strings.TrimRight(desiredNorm, "\n") {
		return "", fmt.Errorf("round-trip validation failed for %s: applied patch does not produce expected content", filePath)
	}

	return diff, nil
}

// ensureTrailingNewline appends a newline if the string doesn't end with one.
func ensureTrailingNewline(s string) string {
	if s == "" || s[len(s)-1] != '\n' {
		return s + "\n"
	}
	return s
}

// splitLines splits a string into lines, each ending with "\n".
// Unlike difflib.SplitLines, this does not produce a trailing empty element
// when the string ends with a newline, which avoids phantom empty context
// lines in the generated unified diff.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	// Remove trailing empty element caused by trailing newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// wrapPatchError translates low-level gitdiff errors into user-friendly messages.
func wrapPatchError(phase string, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "fragment header miscounts lines"):
		return fmt.Errorf("invalid hunk header: line counts in @@ ... @@ do not match the actual number of diff lines.\n"+
			"  Hint: verify the numbers after @@ (e.g. @@ -3,5 +3,5 @@) equal the lines in that hunk.\n"+
			"  Underlying error: %w", err)
	case strings.Contains(msg, "conflict"):
		return fmt.Errorf("patch context does not match the source content.\n"+
			"  Hint: the context lines (lines without +/-) in the patch must exactly match the source file.\n"+
			"  Underlying error: %w", err)
	default:
		return fmt.Errorf("%s failed: %w", phase, err)
	}
}
