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
		if c.Type == fileset.FileNoOp {
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

		// Determine action type for this repo group
		isNew := false
		isDestroy := false
		for _, c := range rChanges {
			if c.Type == repository.ChangeCreate && c.Field == "repository" {
				isNew = true
				break
			}
			if c.Type == repository.ChangeDelete && c.Field == "repository" {
				isDestroy = true
				break
			}
		}

		// Print action header (Terraform-style)
		if isNew {
			p.ActionHeader(name, "will be created")
			p.GroupHeader(ui.IconAdd, name)
		} else if isDestroy {
			p.ActionHeader(name, "will be destroyed")
			p.GroupHeader(ui.IconRemove, name)
		} else {
			p.ActionHeader(name, "will be updated")
			p.GroupHeader(ui.IconChange, name)
		}

		// Print repository changes
		for _, c := range rChanges {
			if len(c.Children) > 0 {
				icon := ui.IconChange
				if c.Type == repository.ChangeCreate {
					icon = ui.IconAdd
				} else if c.Type == repository.ChangeDelete {
					icon = ui.IconRemove
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
			// Show apply method if available
			strategy := fChanges[0].OnApply
			if strategy != "" {
				label += ", " + strategy
			}
			p.SubGroupHeader(ui.IconChange, fmt.Sprintf("FileSet: %s", ui.Bold.Render(label)))
			for _, c := range fChanges {
				switch c.Type {
				case fileset.FileCreate:
					p.FileCreate(c.Path)
				case fileset.FileUpdate:
					p.FileUpdate(c.Path)
				case fileset.FileDelete:
					p.FileDelete(c.Path)
				}
			}
		}

		p.GroupEnd()
	}

	// Reset column width
	p.SetColumnWidth(0)
}

// printUnifiedApplyResults prints apply results grouped by repo name,
// mirroring the hierarchical structure of printUnifiedPlan.
func printUnifiedApplyResults(p ui.Printer, repoResults []repository.ApplyResult, fileResults []fileset.FileApplyResult) {
	// Build ordered list of unique repo names (preserving appearance order)
	seen := make(map[string]bool)
	var repoNames []string
	for _, r := range repoResults {
		name := r.Change.Name
		if !seen[name] {
			seen[name] = true
			repoNames = append(repoNames, name)
		}
	}
	for _, r := range fileResults {
		name := r.Change.Target
		if !seen[name] {
			seen[name] = true
			repoNames = append(repoNames, name)
		}
	}

	// Index results by repo name
	repoByName := make(map[string][]repository.ApplyResult)
	for _, r := range repoResults {
		repoByName[r.Change.Name] = append(repoByName[r.Change.Name], r)
	}
	fileByTarget := make(map[string][]fileset.FileApplyResult)
	for _, r := range fileResults {
		fileByTarget[r.Change.Target] = append(fileByTarget[r.Change.Target], r)
	}

	for _, name := range repoNames {
		// Compute column width for this group
		w := 0
		for _, r := range repoByName[name] {
			if len(r.Change.Field) > w {
				w = len(r.Change.Field)
			}
		}
		for _, r := range fileByTarget[name] {
			if len(r.Change.Path) > w {
				w = len(r.Change.Path)
			}
		}
		p.SetColumnWidth(w)

		// Determine header icon based on whether any result in this group failed
		icon := ui.IconSuccess
		for _, r := range repoByName[name] {
			if r.Err != nil {
				icon = ui.IconError
				break
			}
		}
		if icon != ui.IconError {
			for _, r := range fileByTarget[name] {
				if r.Err != nil {
					icon = ui.IconError
					break
				}
			}
		}
		p.GroupHeader(icon, name)

		for _, r := range repoByName[name] {
			if r.Err != nil {
				p.ResultError(r.Change.Field, r.Err.Error())
			} else {
				p.ResultSuccess(r.Change.Field, fmt.Sprintf("%sd", r.Change.Type))
			}
		}

		var prURL string
		var commitStrategy string
		for _, r := range fileByTarget[name] {
			if r.Err != nil {
				p.ResultError(r.Change.Path, r.Err.Error())
			} else {
				p.ResultSuccess(r.Change.Path, fmt.Sprintf("%sd", r.Change.Type))
			}
			if r.OnApply != "" {
				commitStrategy = r.OnApply
			}
			if r.PRURL != "" {
				prURL = r.PRURL
			}
		}
		if commitStrategy != "" {
			label := "via " + commitStrategy
			if prURL != "" {
				label += " → " + prURL
			}
			p.Detail(label)
		}

		p.GroupEnd()
	}

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
