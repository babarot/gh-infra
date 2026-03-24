package cmd

import (
	"fmt"

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

		// Set unified column width for this repo group
		p.SetColumnWidth(computeColumnWidth(rChanges, fChanges))

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

		// Print repository changes
		for _, c := range rChanges {
			if len(c.Children) > 0 {
				icon := "~"
				if c.Type == repository.ChangeCreate {
					icon = "+"
				} else if c.Type == repository.ChangeDelete {
					icon = "-"
				}
				header := c.Field
				if s, ok := c.NewValue.(string); ok && s != "" {
					header = fmt.Sprintf("%s[%s]", c.Field, s)
				}
				p.SubGroupHeader(icon, header)
				for _, child := range c.Children {
					switch child.Type {
					case repository.ChangeCreate:
						p.SubItemCreate(child.Field, child.NewValue)
					case repository.ChangeUpdate:
						p.SubItemUpdate(child.Field, ui.FormatValue(child.OldValue), ui.FormatValue(child.NewValue))
					case repository.ChangeDelete:
						p.SubItemDelete(child.Field, child.OldValue)
					}
				}
			} else {
				switch c.Type {
				case repository.ChangeCreate:
					p.ItemCreate(c.Field, c.NewValue)
				case repository.ChangeUpdate:
					p.ItemUpdate(c.Field, ui.FormatValue(c.OldValue), ui.FormatValue(c.NewValue))
				case repository.ChangeDelete:
					p.ItemDelete(c.Field, c.OldValue)
				}
			}
		}

		// Print fileset changes (inline under same repo group)
		if len(fChanges) > 0 {
			label := fmt.Sprintf("%d file", len(fChanges))
			if len(fChanges) != 1 {
				label += "s"
			}
			p.SubGroupHeader("~", fmt.Sprintf("FileSet: %s", ui.Bold.Render(label)))
			for _, c := range fChanges {
				switch c.Type {
				case fileset.FileCreate:
					p.FileCreate(c.Path)
				case fileset.FileUpdate:
					p.FileUpdate(c.Path)
				case fileset.FileDrift:
					p.FileDrift(c.Path, c.OnDrift)
				case fileset.FileSkip:
					p.FileSkip(c.Path)
				}
			}
		}

		p.GroupEnd()
	}

	// Reset column width
	p.SetColumnWidth(0)
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
