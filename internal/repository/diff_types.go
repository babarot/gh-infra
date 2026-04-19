package repository

import "fmt"

type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeUpdate ChangeType = "update"
	ChangeDelete ChangeType = "delete"
	ChangeNoOp   ChangeType = "noop"
)

// Change represents a single planned change.
type Change struct {
	Type     ChangeType // create, update, delete, noop
	Resource string     // "Repository", "BranchProtection", "Secret", "Variable"
	Name     string     // "babarot/my-project"
	Field    string     // "description", "topics", etc.
	OldValue any
	NewValue any

	// Details are display-only plan rendering details. They must not be
	// interpreted as apply units.
	Details []Change

	// Children are the concrete child changes to apply for grouped
	// changes. Most resource-level changes should leave this empty and apply
	// via the parent change.
	Children []Change
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
