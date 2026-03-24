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
