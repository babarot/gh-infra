package infra

import (
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// printPlan prints repository and fileset changes grouped by repo name.
// FileSet changes for a repo are displayed after its repository changes.
func printPlan(p ui.Printer, repoChanges []repository.Change, fileChanges []fileset.Change, showDiff bool) {
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

		// Convert to unified display model and render.
		diffGroups := repoChangesToDiffGroups(rChanges)
		p.SetColumnWidth(ui.DiffGroupFieldWidth(diffGroups))
		ui.RenderDiffGroups(p, diffGroups)

		// Print fileset changes (aligned by file path width, independent of repo fields)
		if len(fChanges) > 0 {
			p.SetColumnWidth(filePathWidth(fChanges))
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
				if showDiff {
					printFileDiff(p, c)
				}
			}
		}

		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

const maxDiffLines = 500

// printFileDiff emits a colored unified diff block for a single file change.
// For creates, it diffs empty string against desired content.
// For deletes, it diffs current content against empty string.
// For updates, it diffs current against desired.
// Output is capped at maxDiffLines to prevent terminal flooding.
func printFileDiff(p ui.Printer, c fileset.Change) {
	var current, desired string
	switch c.Type {
	case fileset.ChangeCreate:
		current = ""
		desired = c.Desired
	case fileset.ChangeUpdate:
		current = c.Current
		desired = c.Desired
	case fileset.ChangeDelete:
		current = c.Current
		desired = ""
	default:
		return
	}
	diff := ui.GenerateDiff(current, desired, c.Path)
	if diff == "" {
		return
	}
	p.PrintDiffBlock(ui.TruncateDiff(diff, maxDiffLines))
}

// printApplyResults prints apply results grouped by repo name,
// mirroring the hierarchical structure of PrintPlan.
func printApplyResults(p ui.Printer, repoResults []repository.ApplyResult, fileResults []fileset.ApplyResult) {
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

		// Partition results into regular and groupable (Label, Milestone)
		var regularResults []repository.ApplyResult
		groupedResults := make(map[string][]repository.ApplyResult)
		var groupResultOrder []string
		for _, r := range repoByName[name] {
			if r.Change.Resource == manifest.ResourceLabel || r.Change.Resource == manifest.ResourceMilestone {
				if _, seen := groupedResults[r.Change.Resource]; !seen {
					groupResultOrder = append(groupResultOrder, r.Change.Resource)
				}
				groupedResults[r.Change.Resource] = append(groupedResults[r.Change.Resource], r)
			} else {
				regularResults = append(regularResults, r)
			}
		}
		for _, r := range regularResults {
			if r.Err != nil {
				p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: r.Change.Field, Detail: r.Err.Error()})
			} else {
				p.PrintResult(ui.ResultItem{Icon: ui.IconSuccess, Field: r.Change.Field, Detail: fmt.Sprintf("%sd", r.Change.Type)})
			}
		}
		for _, resource := range groupResultOrder {
			p.SubGroupHeader(ui.IconSuccess, strings.ToLower(resource)+"s")
			for _, r := range groupedResults[resource] {
				if r.Err != nil {
					p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: r.Change.Field, Detail: r.Err.Error(), Level: ui.IndentSub})
				} else {
					p.PrintResult(ui.ResultItem{Icon: ui.IconSuccess, Field: r.Change.Field, Detail: fmt.Sprintf("%sd", r.Change.Type), Level: ui.IndentSub})
				}
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

// changeGroup represents either a single regular change (resource=="")
// or a batch of same-resource Label/Milestone changes.
type changeGroup struct {
	resource string // "" for regular, "Label"/"Milestone" for grouped
	changes  []repository.Change
}

// isGroupableResource returns true for resources that should be grouped
// under a sub-header (e.g. labels, milestones).
func isGroupableResource(resource string) bool {
	return resource == manifest.ResourceLabel || resource == manifest.ResourceMilestone
}

// groupRepoChanges partitions changes into groups, collapsing consecutive
// Label or Milestone changes into a single group while preserving order.
func groupRepoChanges(changes []repository.Change) []changeGroup {
	var groups []changeGroup
	for _, c := range changes {
		if isGroupableResource(c.Resource) {
			// Append to last group if same resource, else start new group
			if n := len(groups); n > 0 && groups[n-1].resource == c.Resource {
				groups[n-1].changes = append(groups[n-1].changes, c)
			} else {
				groups = append(groups, changeGroup{resource: c.Resource, changes: []repository.Change{c}})
			}
		} else {
			groups = append(groups, changeGroup{changes: []repository.Change{c}})
		}
	}
	return groups
}

// groupIcon returns the aggregate icon for a group of changes.
// If all are creates → "+", all deletes → "-", otherwise "~".
func groupIcon(changes []repository.Change) string {
	allCreate, allDelete := true, true
	for _, c := range changes {
		if c.Type != repository.ChangeCreate {
			allCreate = false
		}
		if c.Type != repository.ChangeDelete {
			allDelete = false
		}
	}
	switch {
	case allCreate:
		return ui.IconAdd
	case allDelete:
		return ui.IconRemove
	default:
		return ui.IconChange
	}
}

// repoChangesToDiffGroups converts repository.Change slice to the unified
// DiffGroup model for rendering. It reuses the existing groupRepoChanges logic.
func repoChangesToDiffGroups(changes []repository.Change) []ui.DiffGroup {
	grouped := groupRepoChanges(changes)
	var result []ui.DiffGroup

	for _, g := range grouped {
		if g.resource != "" {
			// Grouped sub-resource (labels, milestones)
			dg := ui.DiffGroup{
				Header: strings.ToLower(g.resource) + "s",
				Icon:   groupIcon(g.changes),
			}
			for _, c := range g.changes {
				if len(c.Details) > 0 {
					for _, child := range c.Details {
						item := changeToDiffItem(child)
						item.Field = c.Field + "." + child.Field
						dg.Items = append(dg.Items, item)
					}
				} else {
					dg.Items = append(dg.Items, changeToDiffItem(c))
				}
			}
			result = append(result, dg)
		} else {
			c := g.changes[0]
			if len(c.Details) > 0 {
				var icon string
				switch c.Type {
				case repository.ChangeCreate:
					icon = ui.IconAdd
				case repository.ChangeDelete:
					icon = ui.IconRemove
				default:
					icon = ui.IconChange
				}
				header := repoChangeGroupHeader(c)
				dg := ui.DiffGroup{Header: header, Icon: icon}
				for _, child := range c.Details {
					dg.Items = append(dg.Items, changeToDiffItem(child))
				}
				result = append(result, dg)
			} else {
				result = append(result, ui.DiffGroup{
					Items: []ui.DiffItem{changeToDiffItem(c)},
				})
			}
		}
	}
	return result
}

func repoChangeGroupHeader(c repository.Change) string {
	if resource, key, ok := splitBracketResource(c.Resource); ok {
		switch resource {
		case manifest.ResourceRuleset:
			return fmt.Sprintf("ruleset %q", key)
		case manifest.ResourceBranchProtection:
			return fmt.Sprintf("branch protection %q", key)
		}
	}
	header := c.Field
	if s, ok := c.NewValue.(string); ok && s != "" {
		header = fmt.Sprintf("%s[%s]", c.Field, s)
	}
	return header
}

func splitBracketResource(resource string) (name, key string, ok bool) {
	prefix, rest, found := strings.Cut(resource, "[")
	if !found || !strings.HasSuffix(rest, "]") {
		return "", "", false
	}
	key = strings.TrimSuffix(rest, "]")
	if prefix == "" || key == "" {
		return "", "", false
	}
	return prefix, key, true
}

func changeToDiffItem(c repository.Change) ui.DiffItem {
	switch c.Type {
	case repository.ChangeCreate:
		return ui.DiffItem{Icon: ui.IconAdd, Field: c.Field, Value: c.NewValue}
	case repository.ChangeUpdate:
		return ui.DiffItem{Icon: ui.IconChange, Field: c.Field, Old: ui.FormatValue(c.OldValue), New: ui.FormatValue(c.NewValue)}
	case repository.ChangeDelete:
		return ui.DiffItem{Icon: ui.IconRemove, Field: c.Field, Value: c.OldValue}
	default:
		return ui.DiffItem{Field: c.Field}
	}
}

// repoFieldWidth returns the max field width across repo changes (for repo section alignment).
func repoFieldWidth(changes []repository.Change) int {
	w := 0
	for _, c := range changes {
		if len(c.Details) > 0 {
			for _, child := range c.Details {
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
	return w
}

// filePathWidth returns the max path width across file changes (for file section alignment).
func filePathWidth(changes []fileset.Change) int {
	w := 0
	for _, c := range changes {
		if len(c.Path) > w {
			w = len(c.Path)
		}
	}
	return w
}

// changeToItem converts a repository.Change to a ui.ChangeItem.
func changeToItem(c repository.Change, level ui.IndentLevel) ui.ChangeItem {
	switch c.Type {
	case repository.ChangeCreate:
		return ui.ChangeItem{Icon: ui.IconAdd, Field: c.Field, Value: c.NewValue, Level: level}
	case repository.ChangeUpdate:
		return ui.ChangeItem{Icon: ui.IconChange, Field: c.Field, Old: ui.FormatValue(c.OldValue), New: ui.FormatValue(c.NewValue), Level: level}
	case repository.ChangeDelete:
		return ui.ChangeItem{Icon: ui.IconRemove, Field: c.Field, Value: c.OldValue, Level: level}
	default:
		return ui.ChangeItem{Field: c.Field, Level: level}
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
