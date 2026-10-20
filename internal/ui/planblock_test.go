package ui

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func wikiBlock(name, newValue string) PlanBlock {
	return PlanBlock{
		Names: []string{name}, Icon: IconChange, Right: "1 to update",
		Groups: []DiffGroup{{Items: []DiffItem{{Icon: IconChange, Field: "features.wiki", Old: "true", New: newValue}}}},
	}
}

func TestMergeIdentical(t *testing.T) {
	withUnseen := func(b PlanBlock, u Unseen) PlanBlock { b.Unseen = u; return b }
	withRight := func(b PlanBlock, right string) PlanBlock { b.Right = right; return b }
	withIcon := func(b PlanBlock, icon string) PlanBlock { b.Icon = icon; return b }
	names := func(blocks []PlanBlock) [][]string {
		var out [][]string
		for _, b := range blocks {
			out = append(out, b.Names)
		}
		return out
	}

	tests := []struct {
		name       string
		in         []PlanBlock
		wantNames  [][]string
		wantDiffer [][]UnseenPart
	}{
		{"same body merges into the first, which keeps its place",
			[]PlanBlock{wikiBlock("o/a", "false"), wikiBlock("o/b", "true"), wikiBlock("o/c", "false")},
			[][]string{{"o/a", "o/c"}, {"o/b"}}, [][]UnseenPart{nil, nil}},
		{"a different header count does not merge",
			[]PlanBlock{wikiBlock("o/a", "false"), withRight(wikiBlock("o/b", "false"), "2 to update")},
			[][]string{{"o/a"}, {"o/b"}}, [][]UnseenPart{nil, nil}},
		{"created and destroyed repositories do not merge",
			[]PlanBlock{withIcon(wikiBlock("o/a", "false"), IconAdd), withIcon(wikiBlock("o/b", "false"), IconAdd),
				withIcon(wikiBlock("o/c", "false"), IconRemove), withIcon(wikiBlock("o/d", "false"), IconRemove)},
			[][]string{{"o/a"}, {"o/b"}, {"o/c"}, {"o/d"}}, [][]UnseenPart{nil, nil, nil, nil}},
		{"what the body does not show does not stop a merge, but is noted per part",
			[]PlanBlock{
				withUnseen(wikiBlock("o/a", "false"), Unseen{Settings: "s", Files: "1", Delivery: "d"}),
				withUnseen(wikiBlock("o/b", "false"), Unseen{Settings: "s", Files: "2", Delivery: "d"}),
				withUnseen(wikiBlock("o/c", "false"), Unseen{Settings: "t", Files: "1", Delivery: "d"}),
			},
			[][]string{{"o/a", "o/b", "o/c"}}, [][]UnseenPart{{UnseenSettings, UnseenFiles}}},
		{"the same unseen parts are not noted",
			[]PlanBlock{withUnseen(wikiBlock("o/a", "false"), Unseen{Files: "1"}), withUnseen(wikiBlock("o/b", "false"), Unseen{Files: "1"})},
			[][]string{{"o/a", "o/b"}}, [][]UnseenPart{nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeIdentical(tt.in)
			if !reflect.DeepEqual(names(got), tt.wantNames) {
				t.Errorf("names = %v, want %v", names(got), tt.wantNames)
			}
			for i, b := range got {
				if i < len(tt.wantDiffer) && !reflect.DeepEqual(b.Differ, tt.wantDiffer[i]) {
					t.Errorf("block %d Differ = %v, want %v", i, b.Differ, tt.wantDiffer[i])
				}
			}
			if tt.in[0].Names[0] != got[0].Names[0] || len(tt.in[0].Names) != 1 {
				t.Errorf("the input block was changed: %v", tt.in[0].Names)
			}
		})
	}
}

func TestRepoGroupHeader(t *testing.T) {
	var names []string
	for _, n := range []string{"auth", "billing", "catalog", "checkout", "gateway", "inventory", "a-repository-with-a-name-longer-than-the-width-of-a-narrow-terminal", "x"} {
		names = append(names, "acme/svc-"+n)
	}
	for _, notes := range [][]string{nil, {"file contents"}} {
		var buf bytes.Buffer
		NewStandardPrinterWith(&buf, &buf).RepoGroupHeader(IconChange, names, "1 to update each", notes)
		lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")

		if !strings.HasPrefix(lines[0], "  ~ 8 repositories ") || !strings.HasSuffix(lines[0], "1 to update each") {
			t.Errorf("first line = %q", lines[0])
		}
		nameLines := lines[1:]
		if notes != nil {
			last := nameLines[len(nameLines)-1]
			if last != "    ! differs per repository: file contents" {
				t.Errorf("note line = %q", last)
			}
			nameLines = nameLines[:len(nameLines)-1]
		}
		var got []string
		for _, l := range nameLines {
			if !strings.HasPrefix(l, "    ") {
				t.Errorf("name line %q is not indented by 4", l)
			}
			if len(l) > repoNamesWidth && len(strings.Fields(l)) > 1 {
				t.Errorf("name line is %d columns wide: %q", len(l), l)
			}
			got = append(got, strings.Fields(l)...)
		}
		if !reflect.DeepEqual(got, names) {
			t.Errorf("names printed = %v, want each once in order: %v", got, names)
		}
	}
}

func TestWrapNames(t *testing.T) {
	tests := []struct {
		width int
		want  [][]string
	}{
		{100, [][]string{{"o/aa", "o/bb", "o/cc"}}},
		{10, [][]string{{"o/aa", "o/bb"}, {"o/cc"}}},
		{3, [][]string{{"o/aa"}, {"o/bb"}, {"o/cc"}}},
	}
	for _, tt := range tests {
		if got := wrapNames([]string{"o/aa", "o/bb", "o/cc"}, tt.width); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("width %d: got %v, want %v", tt.width, got, tt.want)
		}
	}
}
