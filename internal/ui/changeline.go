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
	New    string // an update's new value, or a create's or delete's value, cut to fit
}

// layoutChanges lays out items, a block of change lines whose field columns
// line up, at level for a terminal width columns wide. Without a terminal
// (width 0) no value is cut. Items with no create, update or delete icon
// print nothing, but still count toward the field column.
func layoutChanges(items []DiffItem, level IndentLevel, width int) []ChangeLine {
	ind := Indent(level)
	fw := itemFieldWidth(items)
	// what comes before a value: the indent, the icon and a space, the field
	// column and two spaces
	prefix := len(ind) + 2 + fw + 2
	var lines []ChangeLine
	for _, item := range items {
		l := ChangeLine{Indent: ind, Icon: item.Icon, Field: fmt.Sprintf("%-*s", fw, item.Field)}
		switch item.Icon {
		case IconChange:
			l.Arrow, l.Old, l.New = true, item.Old, item.New
			if width > 0 {
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
