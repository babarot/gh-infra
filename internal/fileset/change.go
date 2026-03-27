package fileset

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// HasChanges returns true if any file changes are non-noop.
func HasChanges(changes []Change) bool {
	for _, c := range changes {
		if c.Type != ChangeNoOp {
			return true
		}
	}
	return false
}

// CountChanges returns create, update, delete counts.
func CountChanges(changes []Change) (creates, updates, deletes int) {
	for _, c := range changes {
		switch c.Type {
		case ChangeCreate:
			creates++
		case ChangeUpdate:
			updates++
		case ChangeDelete:
			deletes++
		}
	}
	return
}

// DiffStat counts added and removed lines between two strings using a proper
// sequence diff algorithm. This correctly handles line moves (reordering) as
// additions and removals, matching what unified diff displays.
func DiffStat(current, desired string) (added, removed int) {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:       difflib.SplitLines(current),
		B:       difflib.SplitLines(desired),
		Context: 0, // no context lines — only +/- lines
	})
	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}
