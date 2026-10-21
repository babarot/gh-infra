package ui

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
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
		// Without a terminal the old column's cap is worked out for 100
		// columns: (100 - 21 - 3) / 3 = 25, so the description's old value
		// (47) is left out and visibility aligns on its own.
		{"fields line up and nothing is cut without a terminal",
			[]DiffItem{vis, desc}, IndentItem, 0,
			[]ChangeLine{
				{Indent: "      ", Icon: IconChange, Field: "visibility ", Arrow: true, Old: "private", New: "public", Aligned: true},
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
			// the cap is (40 - 20 - 3) / 3 = 5, below private's 7
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
			[]ChangeLine{{Indent: "      ", Icon: IconChange, Field: "visibility       ", Arrow: true, Old: "private", New: "public", Aligned: true}}},
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

func TestLayoutChanges_AlignsArrows(t *testing.T) {
	desc := DiffItem{Icon: IconChange, Field: "description", Old: "gh infra", New: "Declarative GitHub infrastructure"}
	home := DiffItem{Icon: IconChange, Field: "homepage", Old: `""`, New: "https://babarot.github.io/gh-infra/introduction/start"}
	long := DiffItem{Icon: IconChange, Field: "topics", Old: strings.Repeat("o", 40), New: "[go]"}
	add := DiffItem{Icon: IconAdd, Field: "visibility", Value: "public"}

	tests := []struct {
		name  string
		items []DiffItem
		width int
		// per line: whether it is aligned and the spaces after its old value
		aligned []bool
		pad     []int
	}{
		{"old values within the cap line up", []DiffItem{desc, home}, 0, []bool{true, true}, []int{0, 6}},
		// prefix 21, old column 8: 21 + 8 + 3 + 52 = 84 > 80
		{"a line that would not fit keeps its place", []DiffItem{desc, home}, 80, []bool{true, false}, []int{0, 0}},
		// cap (100 - 21 - 3) / 3 = 25 < 40
		{"an old value over the cap keeps its place", []DiffItem{desc, long}, 100, []bool{true, false}, []int{0, 0}},
		{"a create has no arrow to align", []DiffItem{desc, add}, 100, []bool{true, false}, []int{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := layoutChanges(tt.items, IndentItem, tt.width)
			for i, l := range lines {
				if l.Aligned != tt.aligned[i] || l.OldPad != tt.pad[i] {
					t.Errorf("line %d (%s): aligned %v, pad %d; want %v, %d", i, l.Field, l.Aligned, l.OldPad, tt.aligned[i], tt.pad[i])
				}
			}
		})
	}
}

// TestLayoutChanges_AligningCutsNothing checks over generated blocks that
// aligning the arrows only adds spaces: every value reads as it would
// unaligned, the aligned arrows of a block share a column, and every line
// fits the terminal.
func TestLayoutChanges_AligningCutsNothing(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	word := func(max int) string { return strings.Repeat("x", r.IntN(max+1)) }
	icons := []string{IconChange, IconChange, IconChange, IconAdd, IconRemove}
	padded := 0 // aligned lines that got spaces, which is what could go wrong
	for range 2000 {
		var items []DiffItem
		for range 1 + r.IntN(6) {
			item := DiffItem{Icon: icons[r.IntN(len(icons))], Field: "f" + word(20)}
			if item.Icon == IconChange {
				item.Old, item.New = word(30), word(90)
			} else {
				item.Value = word(90)
			}
			items = append(items, item)
		}
		width := []int{0, 60, 80, 100, 120}[r.IntN(5)]
		level := []IndentLevel{IndentItem, IndentSub}[r.IntN(2)]

		got := layoutChanges(items, level, width)
		plain := layoutChangesAligned(items, level, width, false)
		column := -1
		for i, l := range got {
			if l.Old != plain[i].Old || l.New != plain[i].New {
				t.Fatalf("width %d: aligning changed %q -> %q to %q -> %q", width, plain[i].Old, plain[i].New, l.Old, l.New)
			}
			if !l.Aligned {
				if l.OldPad != 0 {
					t.Fatalf("an unaligned line is padded: %+v", l)
				}
				continue
			}
			if l.OldPad > 0 {
				padded++
			}
			arrow := len(l.Indent) + 2 + utf8.RuneCountInString(l.Field) + 2 + utf8.RuneCountInString(l.Old) + l.OldPad
			if column >= 0 && arrow != column {
				t.Fatalf("width %d: arrows at %d and %d in one block", width, column, arrow)
			}
			column = arrow
			if n := arrow + 3 + utf8.RuneCountInString(l.New); width > 0 && n > width {
				t.Fatalf("width %d: aligned line is %d columns", width, n)
			}
		}
	}
	if padded < 300 {
		t.Fatalf("only %d padded lines generated; the generator should align more", padded)
	}
}
