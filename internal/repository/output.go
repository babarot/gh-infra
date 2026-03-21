package repository

import (
	"fmt"
	"io"
	"strings"

	"github.com/babarot/gh-infra/internal/ui"
)

// HasRealChanges returns true if there are any non-noop changes.
func HasRealChanges(changes []Change) bool {
	for _, c := range changes {
		if c.Type != ChangeNoOp {
			return true
		}
	}
	return false
}

// PrintPlan prints the plan result in a human-readable format.
func PrintPlan(w io.Writer, changes []Change) {
	if len(changes) == 0 {
		return
	}

	creates, updates, deletes := countChanges(changes)
	fmt.Fprintf(w, "\nPlan: %d to create, %d to update, %d to destroy\n\n", creates, updates, deletes)

	grouped := groupByName(changes)
	for _, group := range grouped {
		if len(group.changes) == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s %s\n", ui.Yellow.Render("~"), ui.Bold.Render(group.name))
		for _, c := range group.changes {
			printChange(w, c)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, ui.Dim.Render(strings.Repeat("─", 50)))
	fmt.Fprintf(w, "To apply these changes, run: %s\n", ui.Bold.Render("gh infra apply"))
}

// PrintApplyResults prints the results of an apply operation.
func PrintApplyResults(w io.Writer, results []ApplyResult) {
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(w, "  %s %s  %s: %v\n",
				ui.Red.Render("✗"), ui.Bold.Render(r.Change.Name), r.Change.Field, r.Err)
		} else {
			fmt.Fprintf(w, "  %s %s  %s %sd\n",
				ui.Green.Render("✓"), ui.Bold.Render(r.Change.Name), r.Change.Field, r.Change.Type)
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
	fmt.Fprintf(w, "\nApply complete! %s changes applied",
		ui.Green.Render(fmt.Sprintf("%d", succeeded)))
	if failed > 0 {
		fmt.Fprintf(w, ", %s failed", ui.Red.Render(fmt.Sprintf("%d", failed)))
	}
	fmt.Fprintln(w, ".")
}

func printChange(w io.Writer, c Change) {
	switch c.Type {
	case ChangeCreate:
		fmt.Fprintf(w, "      %s %s: %s\n",
			ui.Green.Render("+"), c.Field, ui.Green.Render(fmt.Sprintf("%v", c.NewValue)))
	case ChangeUpdate:
		fmt.Fprintf(w, "      %s %s: %s → %s\n",
			ui.Yellow.Render("~"), c.Field,
			ui.Dim.Render(formatValue(c.OldValue)),
			ui.Bold.Render(formatValue(c.NewValue)))
	case ChangeDelete:
		fmt.Fprintf(w, "      %s %s: %s\n",
			ui.Red.Render("-"), c.Field, ui.Red.Render(fmt.Sprintf("%v", c.OldValue)))
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
