package fileset

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
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
