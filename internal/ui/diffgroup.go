package ui

// DiffGroup represents a block of related changes for rendering.
// When Header is empty, items are rendered at IndentItem level (bare fields).
// When Header is set, a SubGroupHeader is printed and items render at IndentSub.
type DiffGroup struct {
	Header string     // e.g. "features", "branch_protection[main]", "labels", or ""
	Icon   string     // aggregate icon for the header (IconAdd, IconChange, IconRemove)
	Items  []DiffItem // individual field changes within this group
	// Path is set when Header is part of the items' field path (features,
	// merge_strategy), not the name of what they belong to (a ruleset,
	// labels). Only such a group can be put on one line with its child.
	Path bool
}

// DiffItem represents a single field-level change within a DiffGroup.
type DiffItem struct {
	Icon  string // IconAdd, IconChange, IconRemove
	Field string // leaf field name (not full dotted path)
	Value any    // for create/delete
	Old   string // for update (pre-formatted)
	New   string // for update (pre-formatted)
	// Flattened is set on the item FlattenSingleChild made of a group and
	// its only child, such as features.wiki.
	Flattened bool
}

// RenderDiffGroups renders a sequence of DiffGroups through the printer.
// Groups with a Header get a SubGroupHeader; bare groups render items directly.
//
// Field columns are aligned within a block: a run of consecutive bare groups,
// or one group with a Header. A long field name then widens only its own
// block, instead of pushing every value in the repository to the right where
// it would be cut at the terminal width. Flattened items, whose names are
// long (merge_strategy.allow_squash_merge), make a block of their own.
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
		for len(bare) > 0 {
			n := 1
			for n < len(bare) && bare[n].Flattened == bare[0].Flattened {
				n++
			}
			w := itemFieldWidth(bare[:n])
			for _, item := range bare[:n] {
				p.PrintChange(itemToChangeItem(item, IndentItem, w))
			}
			bare = bare[n:]
		}
	}
}

// FlattenSingleChild puts each Path group with a single item on one line:
// "features" with only "wiki" becomes the bare item "features.wiki".
func FlattenSingleChild(groups []DiffGroup) []DiffGroup {
	out := make([]DiffGroup, 0, len(groups))
	for _, g := range groups {
		if g.Path && len(g.Items) == 1 {
			item := g.Items[0]
			item.Field = g.Header + "." + item.Field
			item.Flattened = true
			g = DiffGroup{Items: []DiffItem{item}}
		}
		out = append(out, g)
	}
	return out
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
