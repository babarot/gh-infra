package cmd

import (
	"fmt"
	"io"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// printUnifiedPlan prints repository and fileset changes grouped by repo name.
// FileSet changes for a repo are displayed after its repository changes.
func printUnifiedPlan(p ui.Printer, repoChanges []repository.Change, fileChanges []fileset.FileChange) {
	// Build ordered list of unique repo names (preserving appearance order)
	seen := make(map[string]bool)
	var repoNames []string
	for _, c := range repoChanges {
		if c.Type == repository.ChangeNoOp {
			continue
		}
		if !seen[c.Name] {
			seen[c.Name] = true
			repoNames = append(repoNames, c.Name)
		}
	}
	for _, c := range fileChanges {
		if c.Type == fileset.FileNoOp || c.Type == fileset.FileSkip {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			repoNames = append(repoNames, c.Target)
		}
	}

	// Index changes by repo name
	fileByTarget := make(map[string][]fileset.FileChange)
	for _, c := range fileChanges {
		if c.Type == fileset.FileNoOp {
			continue
		}
		fileByTarget[c.Target] = append(fileByTarget[c.Target], c)
	}
	repoByName := make(map[string][]repository.Change)
	for _, c := range repoChanges {
		if c.Type == repository.ChangeNoOp {
			continue
		}
		repoByName[c.Name] = append(repoByName[c.Name], c)
	}

	for _, name := range repoNames {
		rChanges := repoByName[name]
		fChanges := fileByTarget[name]

		// Compute unified column width from both repo fields and file paths
		colWidth := computeColumnWidth(rChanges, fChanges)

		// Determine header icon
		isNew := false
		for _, c := range rChanges {
			if c.Type == repository.ChangeCreate && c.Field == "repository" {
				isNew = true
				break
			}
		}
		if isNew {
			p.GroupHeader("+", name+"  "+ui.Green.Render("(new)"))
		} else {
			p.GroupHeader("~", name)
		}

		out := p.OutWriter()

		// Print repository changes
		// Items are at indent 6 (4 less than subItems at 10), so add 4 to align columns
		itemWidth := colWidth + 4
		for _, c := range rChanges {
			if len(c.Children) > 0 {
				icon := changeIcon(c.Type)
				header := c.Field
				if s, ok := c.NewValue.(string); ok && s != "" {
					header = fmt.Sprintf("%s[%s]", c.Field, s)
				}
				fmt.Fprintf(out, "      %s %s\n", styledIcon(icon), ui.Bold.Render(header))
				for _, child := range c.Children {
					printSubItem(out, child.Type, child.Field, child.OldValue, child.NewValue, colWidth)
				}
			} else {
				printItem(out, c.Type, c.Field, c.OldValue, c.NewValue, itemWidth)
			}
		}

		// Print fileset changes (inline under same repo group)
		if len(fChanges) > 0 {
			label := fmt.Sprintf("%d file", len(fChanges))
			if len(fChanges) != 1 {
				label += "s"
			}
			fmt.Fprintf(out, "      %s %s\n", styledIcon("~"), ui.Bold.Render(fmt.Sprintf("FileSet: %s", label)))
			for _, c := range fChanges {
				printFileItem(out, c, colWidth)
			}
		}

		p.GroupEnd()
	}
}

// computeColumnWidth returns the max field/path width across both repo and file changes.
func computeColumnWidth(rChanges []repository.Change, fChanges []fileset.FileChange) int {
	w := 0
	for _, c := range rChanges {
		if len(c.Children) > 0 {
			for _, child := range c.Children {
				if len(child.Field) > w {
					w = len(child.Field)
				}
			}
		} else {
			if len(c.Field) > w {
				w = len(c.Field)
			}
		}
	}
	for _, c := range fChanges {
		if len(c.Path) > w {
			w = len(c.Path)
		}
	}
	return w
}

func changeIcon(t repository.ChangeType) string {
	switch t {
	case repository.ChangeCreate:
		return "+"
	case repository.ChangeDelete:
		return "-"
	default:
		return "~"
	}
}

func styledIcon(icon string) string {
	switch icon {
	case "+":
		return ui.Green.Render("+")
	case "-":
		return ui.Red.Render("-")
	default:
		return ui.Yellow.Render(icon)
	}
}

func printItem(out io.Writer, t repository.ChangeType, field string, oldVal, newVal any, w int) {
	switch t {
	case repository.ChangeCreate:
		fmt.Fprintf(out, "      %s %-*s  %s\n",
			ui.Green.Render("+"), w, field, ui.Green.Render(fmt.Sprintf("%v", newVal)))
	case repository.ChangeUpdate:
		fmt.Fprintf(out, "      %s %-*s  %s %s %s\n",
			ui.Yellow.Render("~"), w, field, ui.Dim.Render(ui.FormatValue(oldVal)), ui.Dim.Render("→"), ui.Bold.Render(ui.FormatValue(newVal)))
	case repository.ChangeDelete:
		fmt.Fprintf(out, "      %s %-*s  %s\n",
			ui.Red.Render("-"), w, field, ui.Red.Render(fmt.Sprintf("%v", oldVal)))
	}
}

func printSubItem(out io.Writer, t repository.ChangeType, field string, oldVal, newVal any, w int) {
	switch t {
	case repository.ChangeCreate:
		fmt.Fprintf(out, "          %s %-*s  %s\n",
			ui.Green.Render("+"), w, field, ui.Green.Render(fmt.Sprintf("%v", newVal)))
	case repository.ChangeUpdate:
		fmt.Fprintf(out, "          %s %-*s  %s %s %s\n",
			ui.Yellow.Render("~"), w, field, ui.Dim.Render(ui.FormatValue(oldVal)), ui.Dim.Render("→"), ui.Bold.Render(ui.FormatValue(newVal)))
	case repository.ChangeDelete:
		fmt.Fprintf(out, "          %s %-*s  %s\n",
			ui.Red.Render("-"), w, field, ui.Red.Render(fmt.Sprintf("%v", oldVal)))
	}
}

func printFileItem(out io.Writer, c fileset.FileChange, w int) {
	switch c.Type {
	case fileset.FileCreate:
		fmt.Fprintf(out, "          %s %-*s  %s\n",
			ui.Green.Render("+"), w, c.Path, ui.Green.Render("(new file)"))
	case fileset.FileUpdate:
		fmt.Fprintf(out, "          %s %-*s  %s\n",
			ui.Yellow.Render("~"), w, c.Path, ui.Yellow.Render("(content changed)"))
	case fileset.FileDrift:
		fmt.Fprintf(out, "          %s %-*s  %s  on_drift: %s\n",
			ui.Yellow.Render("⚠"), w, c.Path, ui.Yellow.Render("[drift]"), c.OnDrift)
	case fileset.FileSkip:
		fmt.Fprintf(out, "          %s %-*s  %s  on_drift: skip\n",
			ui.Dim.Render("-"), w, c.Path, ui.Dim.Render("[drift]"))
	}
}
