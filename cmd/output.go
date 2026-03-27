package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/importer"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

const maxImportDisplayPathWidth = 44

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

		printRepoChanges(p, rChanges)

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
				switch c.Type {
				case fileset.FileCreate:
					p.FileCreate(c.Path, added)
				case fileset.FileUpdate:
					p.FileUpdate(c.Path, added, removed)
				case fileset.FileDelete:
					p.FileDelete(c.Path, removed)
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

func computeImportColumnWidth(rChanges []repository.Change, fChanges []importer.ImportChange) int {
	w := 0
	for _, c := range rChanges {
		if len(c.Children) > 0 {
			for _, child := range c.Children {
				if len(child.Field) > w {
					w = len(child.Field)
				}
			}
		} else if len(c.Field) > w {
			w = len(c.Field)
		}
	}
	for _, c := range fChanges {
		if displayPath := importDisplayPath(c); len(displayPath) > w {
			w = len(displayPath)
		}
	}
	return w
}

func printRepoChanges(p ui.Printer, changes []repository.Change) {
	for _, c := range changes {
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
				switch child.Type {
				case repository.ChangeCreate:
					p.SubItemCreate(child.Field, child.NewValue)
				case repository.ChangeUpdate:
					p.SubItemUpdate(child.Field, ui.FormatValue(child.OldValue), ui.FormatValue(child.NewValue))
				case repository.ChangeDelete:
					p.SubItemDelete(child.Field, child.OldValue)
				}
			}
			continue
		}

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

// printUnifiedImportPlan prints import changes grouped by repo, similar to printUnifiedPlan.
// It shows file diffs with +N -M stats to indicate what will be written locally.
func printUnifiedImportPlan(p ui.Printer, repoChanges []repository.Change, importChanges []importer.ImportChange) {
	// Collect repo names from changes
	seen := make(map[string]bool)
	var repoNames []string
	for _, c := range repoChanges {
		if !seen[c.Name] {
			seen[c.Name] = true
			repoNames = append(repoNames, c.Name)
		}
	}
	for _, c := range importChanges {
		if !shouldDisplayImportChange(c) {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			repoNames = append(repoNames, c.Target)
		}
	}

	// Index changes by target
	fileByTarget := make(map[string][]importer.ImportChange)
	for _, c := range importChanges {
		if !shouldDisplayImportChange(c) {
			continue
		}
		fileByTarget[c.Target] = append(fileByTarget[c.Target], c)
	}
	repoByName := make(map[string][]repository.Change)
	for _, c := range repoChanges {
		repoByName[c.Name] = append(repoByName[c.Name], c)
	}

	for _, name := range repoNames {
		rChanges := repoByName[name]
		fChanges := fileByTarget[name]

		p.SetColumnWidth(computeImportColumnWidth(rChanges, fChanges))

		p.ActionHeader(name, "will be imported")
		p.GroupHeader(ui.IconChange, name)

		printRepoChanges(p, rChanges)

		// Print file changes with diff stats
		if len(fChanges) > 0 {
			label := fmt.Sprintf("%d file", len(fChanges))
			if len(fChanges) != 1 {
				label += "s"
			}
			p.SubGroupHeader(ui.IconChange, fmt.Sprintf("FileSet: %s", ui.Bold.Render(label)))
			for _, c := range fChanges {
				displayPath := importDisplayPath(c)
				if c.WriteMode == importer.ImportSkip {
					p.FileWarning(displayPath, c.Reason)
					continue
				}

				added, removed := fileset.DiffStat(c.Current, c.Desired)
				switch c.Type {
				case fileset.FileCreate:
					p.FileCreate(displayPath, added)
				case fileset.FileUpdate:
					p.FileUpdate(displayPath, added, removed)
				case fileset.FileDelete:
					p.FileDelete(displayPath, removed)
				}
				if c.Type == fileset.FileNoOp {
					for _, w := range c.Warnings {
						p.FileWarning(displayPath, w)
					}
				}
			}
		}

		p.GroupEnd()
	}

	p.SetColumnWidth(0)
}

func shouldDisplayImportChange(c importer.ImportChange) bool {
	if c.WriteMode == importer.ImportSkip {
		return true
	}
	if c.Type != fileset.FileNoOp {
		return true
	}
	return len(c.Warnings) > 0
}

func importDisplayPath(c importer.ImportChange) string {
	path := c.Path
	if c.LocalTarget != "" {
		path = relativizePath(c.LocalTarget)
	}
	return shortenPath(path, maxImportDisplayPathWidth)
}

func relativizePath(path string) string {
	if path == "" || !filepath.IsAbs(path) {
		return path
	}
	if rel, err := filepath.Rel(".", path); err == nil {
		return rel
	}
	return path
}

func shortenPath(path string, max int) string {
	if max <= 0 || len(path) <= max {
		return path
	}

	clean := filepath.ToSlash(path)
	parts := strings.Split(clean, "/")
	if len(parts) <= 2 {
		if max <= 3 {
			return clean[:max]
		}
		return clean[:max-3] + "..."
	}

	first := parts[0]
	last := parts[len(parts)-1]
	candidate := first + "/.../" + last
	if len(candidate) <= max {
		return candidate
	}

	if max <= 3 {
		return "..."
	}
	tailWidth := max - len(".../")
	if tailWidth <= 0 {
		return "..."
	}
	if len(last) <= tailWidth {
		return ".../" + last
	}
	return ".../" + last[len(last)-tailWidth:]
}
