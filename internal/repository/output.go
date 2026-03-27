package repository

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/ui"
)

// HasChanges returns true if there are any non-noop changes.
func HasChanges(changes []Change) bool {
	for _, c := range changes {
		if c.Type != ChangeNoOp {
			return true
		}
	}
	return false
}

// CountChanges returns the number of creates, updates, and deletes.
func CountChanges(changes []Change) (creates, updates, deletes int) {
	return countChanges(changes)
}

// PrintApplyResults prints individual apply result lines (no summary).
func PrintApplyResults(p ui.Printer, results []ApplyResult) {
	for _, r := range results {
		if r.Err != nil {
			p.Error(r.Change.Name, fmt.Sprintf("%s: %s", r.Change.Field, r.Err.Error()))
		} else {
			p.Success(r.Change.Name, fmt.Sprintf("%s %sd", r.Change.Field, r.Change.Type))
		}
	}
}

// CountApplyResults returns succeeded and failed counts.
func CountApplyResults(results []ApplyResult) (succeeded, failed int) {
	for _, r := range results {
		if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	return
}

type changeGroup struct {
	name    string
	changes []Change
}

func groupByName(changes []Change) []changeGroup {
	seen := make(map[string]int)
	var groups []changeGroup

	for _, c := range changes {
		if c.Type == ChangeNoOp {
			continue
		}
		idx, ok := seen[c.Name]
		if !ok {
			idx = len(groups)
			seen[c.Name] = idx
			groups = append(groups, changeGroup{name: c.Name})
		}
		groups[idx].changes = append(groups[idx].changes, c)
	}
	return groups
}

func countChanges(changes []Change) (creates, updates, deletes int) {
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
