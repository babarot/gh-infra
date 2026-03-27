package repository

import "fmt"

type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeUpdate ChangeType = "update"
	ChangeDelete ChangeType = "delete"
	ChangeNoOp   ChangeType = "noop"
)

// Change represents a single field-level change.
type Change struct {
	Type     ChangeType
	Resource string // "Repository", "BranchProtection", "Secret", "Variable"
	Name     string // "babarot/my-project"
	Field    string // "description", "topics", etc.
	OldValue any
	NewValue any
	Children []Change // Sub-field details for hierarchical display
}

func (c Change) String() string {
	switch c.Type {
	case ChangeCreate:
		return fmt.Sprintf("+ %s", c.Field)
	case ChangeDelete:
		return fmt.Sprintf("- %s", c.Field)
	case ChangeUpdate:
		return fmt.Sprintf("~ %s: %v → %v", c.Field, c.OldValue, c.NewValue)
	default:
		return ""
	}
}

// Result holds all changes for a plan run.
type Result struct {
	Changes []Change
}

func (r *Result) HasChanges() bool {
	for _, c := range r.Changes {
		if c.Type != ChangeNoOp {
			return true
		}
	}
	return false
}

func (r *Result) Summary() (creates, updates, deletes int) {
	for _, c := range r.Changes {
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

// ReverseChanges flips changes so they can be presented in the opposite direction.
// This is used by import flows, where repository.Diff reports local -> GitHub and
// the UI needs to show GitHub -> local.
func ReverseChanges(changes []Change) []Change {
	reversed := make([]Change, len(changes))
	for i, c := range changes {
		if len(c.Children) > 0 {
			c.Children = ReverseChanges(c.Children)
		} else {
			c.OldValue, c.NewValue = c.NewValue, c.OldValue
		}

		switch c.Type {
		case ChangeCreate:
			c.Type = ChangeDelete
		case ChangeDelete:
			c.Type = ChangeCreate
		}

		reversed[i] = c
	}
	return reversed
}
