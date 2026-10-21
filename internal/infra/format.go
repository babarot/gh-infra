package infra

import (
	"encoding/json"
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
	printPlanWithNotes(p, repoChanges, fileChanges, nil, nil, showDiff)
}

// printPlanWithNotes is printPlan with notes shown under each repository,
// such as what its create leaves to the next plan. unseen tells what apply
// does to a repository that the body does not show; nil leaves it out.
func printPlanWithNotes(p ui.Printer, repoChanges []repository.Change, fileChanges []fileset.Change, notes map[string][]string, unseen unseenFunc, showDiff bool) {
	blocks := planBlocks(repoChanges, fileChanges, notes, unseen, showDiff)
	ui.RenderPlanBlocks(p, ui.RewritePlanBlocks(blocks))
}

// unseenFunc returns what apply does to the repository name with the given
// changes that the plan body does not show.
type unseenFunc func(name string, repoChanges []repository.Change, fileChanges []fileset.Change) ui.Unseen

// unseenOf is the unseenFunc of a plan of repos: the spec values apply
// sends that the body summarizes, the file contents, and how the files are
// delivered, each as JSON. A part that cannot be read is the repository's
// name instead, so it always differs.
func unseenOf(repos []*manifest.Repository) unseenFunc {
	specs := make(map[string]manifest.RepositorySpec, len(repos))
	for _, r := range repos {
		specs[r.Metadata.FullName()] = r.Spec
	}
	return func(name string, repoChanges []repository.Change, fileChanges []fileset.Change) ui.Unseen {
		asJSON := func(v any, err error) string {
			b, jerr := json.Marshal(v)
			if err != nil || jerr != nil {
				return name
			}
			return string(b)
		}
		return ui.Unseen{
			Settings: asJSON(repository.ApplyInputs(specs[name], repoChanges), nil),
			Files:    asJSON(fileContents(fileChanges), nil),
			Delivery: asJSON(fileDeliveries(name, fileChanges)),
		}
	}
}

// fileContents is what changes write to a repository, without its name.
func fileContents(changes []fileset.Change) any {
	type content struct {
		Path, Type, Current, Desired, Mode, CurrentMode string
	}
	out := make([]content, len(changes))
	for i, c := range changes {
		out[i] = content{c.Path, string(c.Type), c.Current, c.Desired, c.Mode, c.CurrentMode}
	}
	return out
}

// fileDeliveries is how apply delivers changes to the repository name: per
// FileSet, the files it delivers and how, built as apply builds it
// (fileSetApplyArgs, and the via planned for the repository).
func fileDeliveries(name string, changes []fileset.Change) (any, error) {
	type delivery struct {
		fileset.Delivery
		NoCommits bool
		Paths     []string
	}
	var out []delivery
	index := make(map[*manifest.FileSet]int)
	for _, c := range changes {
		if i, ok := index[c.FileSet]; ok {
			out[i].Paths = append(out[i].Paths, c.Path)
			continue
		}
		index[c.FileSet] = len(out)
		opts := fileset.ApplyOptions{}
		if c.FileSet != nil {
			_, opts = fileSetApplyArgs(c.FileSet, changes)
		}
		if c.Via != "" {
			opts.Via = c.Via
		}
		d, err := fileset.DeliveryFor(opts, name)
		if err != nil {
			return nil, err
		}
		out = append(out, delivery{d, c.NoCommits, []string{c.Path}})
	}
	return out, nil
}

// planBlocks builds a block of the plan body per repository with changes,
// in plan order. FileSet changes for a repository come after its
// repository changes.
func planBlocks(repoChanges []repository.Change, fileChanges []fileset.Change, notes map[string][]string, unseen unseenFunc, showDiff bool) []ui.PlanBlock {
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

	var blocks []ui.PlanBlock
	for _, name := range repoNames {
		rChanges := repoByName[name]
		fChanges := fileByTarget[name]

		b := ui.PlanBlock{
			Names:  []string{name},
			Icon:   ui.IconChange,
			Right:  changeCounts(rChanges, fChanges),
			Groups: repoChangesToDiffGroups(rChanges),
			Notes:  notes[name],
		}
		for _, c := range rChanges {
			if c.Type == repository.ChangeCreate && c.Field == "repository" {
				b.Icon, b.Right = ui.IconAdd, "will be created"
				break
			}
			if c.Type == repository.ChangeDelete && c.Field == "repository" {
				b.Icon, b.Right = ui.IconRemove, "will be destroyed"
				break
			}
		}
		if len(fChanges) > 0 {
			b.Files = fileBlock(fChanges, showDiff)
		}
		if unseen != nil {
			b.Unseen = unseen(name, rChanges, fChanges)
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// fileBlock holds the FileSet changes of one repository.
func fileBlock(changes []fileset.Change, showDiff bool) *ui.FileBlock {
	f := &ui.FileBlock{Via: changes[0].Via}
	unsigned := false
	for _, c := range changes {
		added, removed := fileset.DiffStat(c.Current, c.Desired)
		f.Items = append(f.Items, fileChangeToItem(c, added, removed))
		diff := ""
		if showDiff {
			diff = fileDiff(c)
		}
		f.Diffs = append(f.Diffs, diff)
		unsigned = unsigned || c.ModeChanged()
	}
	if changes[0].NoCommits {
		f.Notes = append(f.Notes, fileset.NoCommitsNote)
	}
	if unsigned {
		f.Notes = append(f.Notes, "Changing file modes needs the Git Data API, so this commit will not be signed")
	}
	return f
}

// changeCounts describes a repository's changes for its header, such as
// "2 to create · 5 to update". It counts as the plan summary does, so the
// headers add up to the summary.
func changeCounts(repoChanges []repository.Change, fileChanges []fileset.Change) string {
	rc, ru, rd := repository.CountChanges(repoChanges)
	fc, fu, fd := fileset.CountChanges(fileChanges)
	var parts []string
	for _, n := range []struct {
		count int
		verb  string
	}{{rc + fc, "create"}, {ru + fu, "update"}, {rd + fd, "destroy"}} {
		if n.count > 0 {
			parts = append(parts, fmt.Sprintf("%d to %s", n.count, n.verb))
		}
	}
	return strings.Join(parts, " · ")
}

// plannedRepos returns the repositories with changes, in plan order.
func plannedRepos(repoChanges []repository.Change, fileChanges []fileset.Change) []string {
	var names []string
	for _, c := range repoChanges {
		if c.Type != repository.ChangeNoOp {
			names = append(names, c.Name)
		}
	}
	for _, c := range fileChanges {
		if c.Type != fileset.ChangeNoOp {
			names = append(names, c.Target)
		}
	}
	return uniqueStrings(names)
}

const maxDiffLines = 500

// printFileDiff emits a colored unified diff block for a single file change.
func printFileDiff(p ui.Printer, c fileset.Change) {
	if diff := fileDiff(c); diff != "" {
		p.PrintDiffBlock(diff)
	}
}

// fileDiff returns the unified diff for a single file change, capped at
// maxDiffLines to prevent flooding, or "" when there is nothing to show.
// For creates, it diffs empty string against desired content.
// For deletes, it diffs current content against empty string.
// For updates, it diffs current against desired.
func fileDiff(c fileset.Change) string {
	var current, desired string
	switch c.Type {
	case fileset.ChangeCreate:
		desired = c.Desired
	case fileset.ChangeUpdate:
		current = c.Current
		desired = c.Desired
	case fileset.ChangeDelete:
		current = c.Current
	default:
		return ""
	}
	diff := ui.GenerateDiff(current, desired, c.Path)
	if diff == "" {
		return ""
	}
	return ui.TruncateDiff(diff, maxDiffLines)
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
			switch {
			case r.Err != nil:
				p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: r.Change.Path, Detail: r.Err.Error()})
			case r.Skipped:
				// No commit was made, so there is no delivery method to report.
				p.PrintResult(ui.ResultItem{Icon: ui.IconSuccess, Field: r.Change.Path, Detail: "skipped (no content change on GitHub)"})
				continue
			default:
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

// printApplyFollowUps prints, after the spinner, what of an apply needs a
// hand: each failed change with its error, by repository, and the pull
// requests opened. The spinner's rows already showed every repository's
// result, so nothing is printed when nothing failed and no pull request
// was opened.
func printApplyFollowUps(p ui.Printer, repoResults []repository.ApplyResult, fileResults []fileset.ApplyResult) {
	type failure struct{ field, err string }
	var names []string // repositories to mention, in result order
	seen := make(map[string]bool)
	failures := make(map[string][]failure)
	pullRequests := make(map[string]string)
	note := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, r := range repoResults {
		if r.Err != nil {
			note(r.Change.Name)
			failures[r.Change.Name] = append(failures[r.Change.Name], failure{r.Change.Field, r.Err.Error()})
		}
	}
	for _, r := range fileResults {
		switch {
		case r.Err != nil:
			note(r.Change.Target)
			failures[r.Change.Target] = append(failures[r.Change.Target], failure{r.Change.Path, r.Err.Error()})
		case r.PRURL != "":
			note(r.Change.Target)
			pullRequests[r.Change.Target] = r.PRURL
		}
	}
	if len(names) == 0 {
		return
	}

	p.Separator()
	for _, name := range names {
		fs := failures[name]
		if len(fs) == 0 {
			continue
		}
		w := 0
		for _, f := range fs {
			w = max(w, len(f.field))
		}
		p.SetColumnWidth(w)
		p.GroupHeader(ui.IconError, name)
		for _, f := range fs {
			p.PrintResult(ui.ResultItem{Icon: ui.IconError, Field: f.field, Detail: f.err})
		}
		p.GroupEnd()
	}
	p.SetColumnWidth(0)
	var links []ui.PullRequestLink
	for _, name := range names {
		if url := pullRequests[name]; url != "" {
			links = append(links, ui.PullRequestLink{Name: name, URL: url})
		}
	}
	p.PullRequests(links)
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
				// A header that is the change's own field (features,
				// merge_strategy) is part of its children's path; a ruleset
				// or branch protection header names the item instead.
				dg := ui.DiffGroup{Header: header, Icon: icon, Path: header == c.Field}
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

// fileChangeToItem converts a fileset.FileChange to a ui.FileItem.
func fileChangeToItem(c fileset.Change, added, removed int) ui.FileItem {
	item := changeTypeToItem(c, added, removed)
	if c.ModeChanged() {
		item.OldMode = c.CurrentMode
		item.NewMode = c.Mode
	}
	return item
}

func changeTypeToItem(c fileset.Change, added, removed int) ui.FileItem {
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
