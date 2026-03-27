package infra

import (
	"fmt"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// PrintPlan prints repository and fileset changes grouped by repo name.
// FileSet changes for a repo are displayed after its repository changes.
func PrintPlan(p ui.Printer, repoChanges []repository.Change, fileChanges []fileset.Change) {
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
		if c.Type == fileset.ChangeNoOp {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			repoNames = append(repoNames, c.Target)
		}
	}

	// Index changes by repo name
	fileByTarget := make(map[string][]fileset.Change)
	for _, c := range fileChanges {
		if c.Type == fileset.ChangeNoOp {
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
		p.SetColumnWidth(ComputeColumnWidth(rChanges, fChanges))

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
				var icon string
				switch c.Type {
				case repository.ChangeCreate:
					icon = ui.IconAdd
				case repository.ChangeDelete:
					icon = ui.IconRemove
				default:
					icon = ui.IconChange
				}
				header := c.Field
				if s, ok := c.NewValue.(string); ok && s != "" {
					header = fmt.Sprintf("%s[%s]", c.Field, s)
				}
				p.SubGroupHeader(icon, header)
				for _, child := range c.Children {
					p.PrintChange(changeToItem(child, true))
				}
			} else {
				p.PrintChange(changeToItem(c, false))
			}
		}

		// Print fileset changes (inline under same repo group)
		if len(fChanges) > 0 {
			label := fmt.Sprintf("%d file", len(fChanges))
			if len(fChanges) != 1 {
				label += "s"
			}
			// Show delivery method if available
			via := fChanges[0].Via
			if via != "" {
				label += ", via " + ui.Cyan.Render(via)
			}
			p.SubGroupHeader(ui.IconChange, fmt.Sprintf("FileSet: %s", ui.Bold.Render(label)))
			for _, c := range fChanges {
				added, removed := fileset.DiffStat(c.Current, c.Desired)
				p.PrintFileChange(fileChangeToItem(c, added, removed))
			}
		}

		p.GroupEnd()
	}

	// Reset column width
	p.SetColumnWidth(0)
}

// PrintApplyResults prints apply results grouped by repo name,
// mirroring the hierarchical structure of PrintPlan.
func PrintApplyResults(p ui.Printer, repoResults []repository.ApplyResult, fileResults []fileset.ApplyResult) {
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
	fileByTarget := make(map[string][]fileset.ApplyResult)
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
				p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: r.Change.Field, Detail: r.Err.Error()})
			} else {
				p.PrintResult(ui.ResultItem{Icon: ui.IconSuccess, Field: r.Change.Field, Detail: fmt.Sprintf("%sd", r.Change.Type)})
			}
		}

		var prURL string
		var commitStrategy string
		for _, r := range fileByTarget[name] {
			if r.Err != nil {
				p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: r.Change.Path, Detail: r.Err.Error()})
			} else {
				p.PrintResult(ui.ResultItem{Icon: ui.IconSuccess, Field: r.Change.Path, Detail: fmt.Sprintf("%sd", r.Change.Type)})
			}
			if r.Via != "" {
				commitStrategy = r.Via
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

// ComputeColumnWidth returns the max field/path width across both repo and file changes.
func ComputeColumnWidth(rChanges []repository.Change, fChanges []fileset.Change) int {
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

// changeToItem converts a repository.Change to a ui.ChangeItem.
func changeToItem(c repository.Change, sub bool) ui.ChangeItem {
	switch c.Type {
	case repository.ChangeCreate:
		return ui.ChangeItem{Icon: ui.IconAdd, Field: c.Field, Value: c.NewValue, Sub: sub}
	case repository.ChangeUpdate:
		return ui.ChangeItem{Icon: ui.IconChange, Field: c.Field, Old: ui.FormatValue(c.OldValue), New: ui.FormatValue(c.NewValue), Sub: sub}
	case repository.ChangeDelete:
		return ui.ChangeItem{Icon: ui.IconRemove, Field: c.Field, Value: c.OldValue, Sub: sub}
	default:
		return ui.ChangeItem{Field: c.Field, Sub: sub}
	}
}

// fileChangeToItem converts a fileset.FileChange to a ui.FileItem.
func fileChangeToItem(c fileset.Change, added, removed int) ui.FileItem {
	switch c.Type {
	case fileset.ChangeCreate:
		return ui.FileItem{Icon: ui.IconAdd, Path: c.Path, Added: added}
	case fileset.ChangeUpdate:
		return ui.FileItem{Icon: ui.IconChange, Path: c.Path, Added: added, Removed: removed}
	case fileset.ChangeDelete:
		return ui.FileItem{Icon: ui.IconRemove, Path: c.Path, Removed: removed}
	default:
		return ui.FileItem{Path: c.Path}
	}
}
