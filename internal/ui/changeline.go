package ui

import (
	"fmt"
	"unicode/utf8"
)

// ChangeLine is a settings change line laid out: what goes in each column.
// layoutChanges decides all of it; the printer only styles and writes it.
type ChangeLine struct {
	Indent string // leading spaces of the line's level
	Icon   string // IconAdd, IconChange or IconRemove
	Field  string // the field name, padded to the block's field column
	Arrow  bool   // an update: Old, an arrow and New
	Old    string // an update's old value, cut to fit
	OldPad int    // spaces after Old that bring the arrow to the block's old column
	New    string // an update's new value, or a create's or delete's value, cut to fit
	// Aligned is set on an update whose arrow is at the block's old column.
	Aligned bool
}

// alignWidth is the terminal width the old column's cap is worked out for
// when there is no terminal, so CI logs line up the rows a terminal of that
// width would.
const alignWidth = 100

// layoutChanges lays out items, a block of change lines whose field columns
// line up, at level for a terminal width columns wide. Without a terminal
// (width 0) no value is cut. Items with no create, update or delete icon
// print nothing, but still count toward the field column.
//
// The arrows of the block's updates line up where that cuts no value: an
// update's old value is padded to the block's old column when it is at most
// a third of what the line leaves for the values (the share
// truncateChangeValues gives an old value) and the padded line still fits.
// Other lines are laid out as if nothing were aligned.
func layoutChanges(items []DiffItem, level IndentLevel, width int) []ChangeLine {
	return layoutChangesAligned(items, level, width, true)
}

func layoutChangesAligned(items []DiffItem, level IndentLevel, width int, align bool) []ChangeLine {
	ind := Indent(level)
	fw := itemFieldWidth(items)
	// what comes before a value: the indent, the icon and a space, the field
	// column and two spaces
	prefix := len(ind) + 2 + fw + 2

	capWidth := width
	if capWidth == 0 {
		capWidth = alignWidth
	}
	oldCap := (capWidth - prefix - 3) / 3
	oldColumn := 0
	for _, item := range items {
		if n := utf8.RuneCountInString(item.Old); item.Icon == IconChange && n <= oldCap {
			oldColumn = max(oldColumn, n)
		}
	}

	var lines []ChangeLine
	for _, item := range items {
		l := ChangeLine{Indent: ind, Icon: item.Icon, Field: fmt.Sprintf("%-*s", fw, item.Field)}
		switch item.Icon {
		case IconChange:
			l.Arrow, l.Old, l.New = true, item.Old, item.New
			oldLen := utf8.RuneCountInString(item.Old)
			fits := width == 0 || prefix+oldColumn+3+utf8.RuneCountInString(item.New) <= width
			switch {
			case align && oldLen <= oldCap && fits:
				l.Aligned, l.OldPad = true, oldColumn-oldLen
			case width > 0:
				l.Old, l.New = truncateChangeValues(item.Old, item.New, width, prefix)
			}
		case IconAdd, IconRemove:
			l.New = FormatValue(item.Value)
			if width > 0 {
				l.New = fitValue(l.New, width-prefix)
			}
		default:
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// fitValue cuts s to n columns, ending it with an ellipsis, when it is
// wider.
func fitValue(s string, n int) string {
	if utf8.RuneCountInString(s) <= n || n <= 1 {
		return s
	}
	return truncateRunes(s, n-1) + IconEllipsis
}
