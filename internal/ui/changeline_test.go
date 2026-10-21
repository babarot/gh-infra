package ui

import (
	"reflect"
	"strings"
	"testing"
)

func TestLayoutChanges(t *testing.T) {
	desc := DiffItem{Icon: IconChange, Field: "description", Old: "Manage GitHub repository settings declaratively", New: "Declarative GitHub infrastructure as code"}
	vis := DiffItem{Icon: IconChange, Field: "visibility", Old: "private", New: "public"}
	home := DiffItem{Icon: IconAdd, Field: "homepage", Value: "https://acme.example.com/tools/internal/handbook"}
	topics := DiffItem{Icon: IconRemove, Field: "topics", Value: []string{"legacy"}}

	tests := []struct {
		name  string
		items []DiffItem
		level IndentLevel
		width int
		want  []ChangeLine
	}{
		{"fields line up and nothing is cut without a terminal",
			[]DiffItem{vis, desc}, IndentItem, 0,
			[]ChangeLine{
				{Indent: "      ", Icon: IconChange, Field: "visibility ", Arrow: true, Old: "private", New: "public"},
				{Indent: "      ", Icon: IconChange, Field: "description", Arrow: true, Old: desc.Old, New: desc.New},
			}},
		// prefix 6 + 2 + 11 + 2 = 21; 80 - 21 - 3 = 56 for the values,
		// a third (18) for the old one and 38 for the new one
		{"an update too wide for the terminal gives a third to the old value",
			[]DiffItem{desc}, IndentItem, 80,
			[]ChangeLine{{Indent: "      ", Icon: IconChange, Field: "description", Arrow: true,
				Old: "Manage GitHub rep…", New: "Declarative GitHub infrastructure as …"}}},
		{"an update that fits is not cut",
			[]DiffItem{vis}, IndentItem, 40,
			[]ChangeLine{{Indent: "      ", Icon: IconChange, Field: "visibility", Arrow: true, Old: "private", New: "public"}}},
		// prefix 6 + 2 + 8 + 2 = 18; the value gets all of 50 - 18 = 32
		{"a create's value is cut to what is left of the line",
			[]DiffItem{home}, IndentItem, 50,
			[]ChangeLine{{Indent: "      ", Icon: IconAdd, Field: "homepage", New: "https://acme.example.com/tools/…"}}},
		{"a delete's value is formatted",
			[]DiffItem{topics}, IndentSub, 0,
			[]ChangeLine{{Indent: "          ", Icon: IconRemove, Field: "topics", New: "[legacy]"}}},
		{"an item without an icon prints nothing but widens the field column",
			[]DiffItem{{Field: "a-long-field-name"}, vis}, IndentItem, 0,
			[]ChangeLine{{Indent: "      ", Icon: IconChange, Field: "visibility       ", Arrow: true, Old: "private", New: "public"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutChanges(tt.items, tt.level, tt.width); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// TestLayoutChanges_FitsTheTerminal checks that every line laid out for a
// terminal, printed, is no wider than the terminal.
func TestLayoutChanges_FitsTheTerminal(t *testing.T) {
	items := []DiffItem{
		{Icon: IconChange, Field: "description", Old: strings.Repeat("o", 70), New: strings.Repeat("n", 90)},
		{Icon: IconChange, Field: "topics", Old: "[]", New: strings.Repeat("t", 120)},
		{Icon: IconAdd, Field: "homepage", Value: strings.Repeat("h", 150)},
		{Icon: IconRemove, Field: "x", Value: strings.Repeat("r", 150)},
	}
	for _, width := range []int{60, 80, 100, 120} {
		for _, level := range []IndentLevel{IndentItem, IndentSub} {
			for _, l := range layoutChanges(items, level, width) {
				printed := l.Indent + l.Icon + " " + l.Field + "  " + l.New
				if l.Arrow {
					printed = l.Indent + l.Icon + " " + l.Field + "  " + l.Old + " → " + l.New
				}
				if n := len([]rune(printed)); n > width {
					t.Errorf("width %d: %d columns: %q", width, n, printed)
				}
			}
		}
	}
}
