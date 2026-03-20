package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/babarot/gh-infra/internal/apply"
	"github.com/babarot/gh-infra/internal/plan"
)

// PrintPlan prints the plan result in a human-readable format.
func PrintPlan(w io.Writer, changes []plan.Change) {
	if len(changes) == 0 {
		fmt.Fprintln(w, "No changes. Infrastructure is up-to-date.")
		return
	}

	creates, updates, deletes := countChanges(changes)
	fmt.Fprintf(w, "\nPlan: %d to create, %d to update, %d to destroy\n\n", creates, updates, deletes)

	// Group by repo name
	grouped := groupByName(changes)
	for _, group := range grouped {
		if len(group.changes) == 0 {
			continue
		}
		fmt.Fprintf(w, "  ~ %s\n", group.name)
		for _, c := range group.changes {
			printChange(w, c)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, strings.Repeat("─", 50))
	fmt.Fprintln(w, "To apply these changes, run: gh infra apply")
}

// PrintApplyResults prints the results of an apply operation.
func PrintApplyResults(w io.Writer, results []apply.ApplyResult) {
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(w, "  ✗ %s  %s: %v\n", r.Change.Name, r.Change.Field, r.Err)
		} else {
			fmt.Fprintf(w, "  ✓ %s  %s %sd\n", r.Change.Name, r.Change.Field, r.Change.Type)
		}
	}

	succeeded := 0
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	fmt.Fprintf(w, "\nApply complete! %d changes applied", succeeded)
	if failed > 0 {
		fmt.Fprintf(w, ", %d failed", failed)
	}
	fmt.Fprintln(w, ".")
}

func printChange(w io.Writer, c plan.Change) {
	switch c.Type {
	case plan.ChangeCreate:
		fmt.Fprintf(w, "      + %s: %v\n", c.Field, c.NewValue)
	case plan.ChangeUpdate:
		fmt.Fprintf(w, "      ~ %s: %v → %v\n", c.Field, formatValue(c.OldValue), formatValue(c.NewValue))
	case plan.ChangeDelete:
		fmt.Fprintf(w, "      - %s: %v\n", c.Field, c.OldValue)
	}
}

func formatValue(v any) string {
	switch val := v.(type) {
	case []string:
		return "[" + strings.Join(val, ", ") + "]"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

type changeGroup struct {
	name    string
	changes []plan.Change
}

func groupByName(changes []plan.Change) []changeGroup {
	seen := make(map[string]int)
	var groups []changeGroup

	for _, c := range changes {
		if c.Type == plan.ChangeNoOp {
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

func countChanges(changes []plan.Change) (creates, updates, deletes int) {
	for _, c := range changes {
		switch c.Type {
		case plan.ChangeCreate:
			creates++
		case plan.ChangeUpdate:
			updates++
		case plan.ChangeDelete:
			deletes++
		}
	}
	return
}
