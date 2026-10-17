package ui

// DiffGroup represents a block of related changes for rendering.
// When Header is empty, items are rendered at IndentItem level (bare fields).
// When Header is set, a SubGroupHeader is printed and items render at IndentSub.
type DiffGroup struct {
	Header string     // e.g. "features", "branch_protection[main]", "labels", or ""
	Icon   string     // aggregate icon for the header (IconAdd, IconChange, IconRemove)
	Items  []DiffItem // individual field changes within this group
}

// DiffItem represents a single field-level change within a DiffGroup.
type DiffItem struct {
	Icon  string // IconAdd, IconChange, IconRemove
	Field string // leaf field name (not full dotted path)
	Value any    // for create/delete
	Old   string // for update (pre-formatted)
	New   string // for update (pre-formatted)
}

// RenderDiffGroups renders a sequence of DiffGroups through the printer.
// Groups with a Header get a SubGroupHeader; bare groups render items directly.
//
// Field columns are aligned within a block: a run of consecutive bare groups,
// or one group with a Header. A long field name then widens only its own
// block, instead of pushing every value in the repository to the right where
// it would be cut at the terminal width.
func RenderDiffGroups(p Printer, groups []DiffGroup) {
	for i := 0; i < len(groups); {
		if g := groups[i]; g.Header != "" {
			p.SubGroupHeader(g.Icon, g.Header)
			w := itemFieldWidth(g.Items)
			for _, item := range g.Items {
				p.PrintChange(itemToChangeItem(item, IndentSub, w))
			}
			i++
			continue
		}
		var bare []DiffItem
		for ; i < len(groups) && groups[i].Header == ""; i++ {
			bare = append(bare, groups[i].Items...)
		}
		w := itemFieldWidth(bare)
		for _, item := range bare {
			p.PrintChange(itemToChangeItem(item, IndentItem, w))
		}
	}
}

// DiffGroupFieldWidth returns the maximum field name width across all items.
func DiffGroupFieldWidth(groups []DiffGroup) int {
	w := 0
	for _, g := range groups {
		w = max(w, itemFieldWidth(g.Items))
	}
	return w
}

// itemFieldWidth returns the maximum field name width of items.
func itemFieldWidth(items []DiffItem) int {
	w := 0
	for _, item := range items {
		w = max(w, len(item.Field))
	}
	return w
}

func itemToChangeItem(item DiffItem, level IndentLevel, width int) ChangeItem {
	return ChangeItem{
		Icon:  item.Icon,
		Field: item.Field,
		Value: item.Value,
		Old:   item.Old,
		New:   item.New,
		Level: level,
		Width: width,
	}
}
