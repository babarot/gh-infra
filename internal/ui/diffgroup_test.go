package ui

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRenderDiffGroups_BareItems(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	groups := []DiffGroup{
		{Items: []DiffItem{{Icon: IconChange, Field: "description", Old: `""`, New: "My repo"}}},
		{Items: []DiffItem{{Icon: IconChange, Field: "visibility", Old: "private", New: "public"}}},
	}
	p.SetColumnWidth(DiffGroupFieldWidth(groups))
	RenderDiffGroups(p, groups)

	out := buf.String()
	if !strings.Contains(out, "description") || !strings.Contains(out, "visibility") {
		t.Errorf("expected bare items, got:\n%s", out)
	}
}

func TestRenderDiffGroups_GroupWithHeader(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	groups := []DiffGroup{
		{
			Header: "features",
			Icon:   IconChange,
			Items: []DiffItem{
				{Icon: IconChange, Field: "issues", Old: "false", New: "true"},
				{Icon: IconChange, Field: "wiki", Old: "true", New: "false"},
			},
		},
	}
	p.SetColumnWidth(DiffGroupFieldWidth(groups))
	RenderDiffGroups(p, groups)

	out := buf.String()
	if !strings.Contains(out, "features") {
		t.Errorf("expected 'features' header, got:\n%s", out)
	}
	if !strings.Contains(out, "issues") || !strings.Contains(out, "wiki") {
		t.Errorf("expected child items, got:\n%s", out)
	}
}

func TestRenderDiffGroups_Mixed(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	groups := []DiffGroup{
		{Items: []DiffItem{{Icon: IconChange, Field: "description", Old: `""`, New: "test"}}},
		{
			Header: "labels",
			Icon:   IconAdd,
			Items: []DiffItem{
				{Icon: IconAdd, Field: "kind/bug", Value: "#d73a4a"},
			},
		},
	}
	p.SetColumnWidth(DiffGroupFieldWidth(groups))
	RenderDiffGroups(p, groups)

	out := buf.String()
	if !strings.Contains(out, "description") {
		t.Errorf("expected bare item")
	}
	if !strings.Contains(out, "labels") {
		t.Errorf("expected labels header")
	}
	if !strings.Contains(out, "kind/bug") {
		t.Errorf("expected label item")
	}
}

func TestRenderDiffGroups_WidthPerBlock(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	// A long field in one group must not widen the others: the top-level
	// fields would be pushed right and their values cut at the terminal width.
	groups := []DiffGroup{
		{Items: []DiffItem{{Icon: IconChange, Field: "description", Old: "old", New: "new"}}},
		{Items: []DiffItem{{Icon: IconChange, Field: "topics", Old: "[a]", New: "[a, b]"}}},
		{Header: `ruleset "main"`, Icon: IconChange, Items: []DiffItem{
			{Icon: IconChange, Field: "required_approving_review_count", Old: "0", New: "1"},
			{Icon: IconAdd, Field: "required_status_checks", Value: "[test]"},
		}},
		{Header: "labels", Icon: IconAdd, Items: []DiffItem{
			{Icon: IconAdd, Field: "kind/bug.color", Value: "d73a4a"},
		}},
		{Items: []DiffItem{{Icon: IconChange, Field: "visibility", Old: "private", New: "public"}}},
	}
	p.SetColumnWidth(DiffGroupFieldWidth(groups)) // as callers do; the blocks must not use it
	RenderDiffGroups(p, groups)

	want := strings.Join([]string{
		"      ~ description  old → new",
		"      ~ topics       [a] → [a, b]",
		`      ~ ruleset "main"`,
		"          ~ required_approving_review_count  0 → 1",
		"          + required_status_checks           [test]",
		"      + labels",
		"          + kind/bug.color  d73a4a",
		"      ~ visibility  private → public",
	}, "\n") + "\n"
	if got := buf.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRenderDiffGroups_Empty(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	RenderDiffGroups(p, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty groups")
	}
}

func TestDiffGroupFieldWidth(t *testing.T) {
	groups := []DiffGroup{
		{Items: []DiffItem{{Field: "ab"}, {Field: "abcdef"}}},
		{Header: "x", Items: []DiffItem{{Field: "abc"}}},
	}
	if w := DiffGroupFieldWidth(groups); w != 6 {
		t.Errorf("expected width 6, got %d", w)
	}
}

func TestDiffGroupFieldWidth_Empty(t *testing.T) {
	if w := DiffGroupFieldWidth(nil); w != 0 {
		t.Errorf("expected 0, got %d", w)
	}
}

func TestFlattenSingleChild(t *testing.T) {
	wiki := DiffItem{Icon: IconChange, Field: "wiki", Old: "true", New: "false"}
	issues := DiffItem{Icon: IconAdd, Field: "issues", Value: true}
	bare := DiffGroup{Items: []DiffItem{{Icon: IconChange, Field: "description", Old: "a", New: "b"}}}
	tests := []struct {
		name string
		in   DiffGroup
		want DiffGroup
	}{
		{"path group with one child goes on one line",
			DiffGroup{Header: "features", Icon: IconChange, Path: true, Items: []DiffItem{wiki}},
			DiffGroup{Items: []DiffItem{{Icon: IconChange, Field: "features.wiki", Old: "true", New: "false", Flattened: true}}}},
		{"the child's icon is kept",
			DiffGroup{Header: "features", Icon: IconChange, Path: true, Items: []DiffItem{issues}},
			DiffGroup{Items: []DiffItem{{Icon: IconAdd, Field: "features.issues", Value: true, Flattened: true}}}},
		{"path group with two children stays",
			DiffGroup{Header: "features", Icon: IconChange, Path: true, Items: []DiffItem{wiki, issues}},
			DiffGroup{Header: "features", Icon: IconChange, Path: true, Items: []DiffItem{wiki, issues}}},
		{"named group with one child stays",
			DiffGroup{Header: `ruleset "main"`, Icon: IconChange, Items: []DiffItem{wiki}},
			DiffGroup{Header: `ruleset "main"`, Icon: IconChange, Items: []DiffItem{wiki}}},
		{"bare group stays", bare, bare},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FlattenSingleChild([]DiffGroup{tt.in})
			if len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRenderDiffGroups_FlattenedItemsAlignApart(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	// A flattened name is long; aligning it with the plain fields would push
	// their values right, where they would be cut at the terminal width.
	groups := FlattenSingleChild([]DiffGroup{
		{Items: []DiffItem{{Icon: IconChange, Field: "description", Old: "old", New: "new"}}},
		{Header: "features", Icon: IconChange, Path: true, Items: []DiffItem{{Icon: IconChange, Field: "wiki", Old: "true", New: "false"}}},
		{Header: "merge_strategy", Icon: IconChange, Path: true, Items: []DiffItem{{Icon: IconChange, Field: "allow_squash_merge", Old: "false", New: "true"}}},
		{Items: []DiffItem{{Icon: IconChange, Field: "visibility", Old: "private", New: "public"}}},
	})
	RenderDiffGroups(p, groups)

	want := strings.Join([]string{
		"      ~ description  old → new",
		"      ~ features.wiki                      true → false",
		"      ~ merge_strategy.allow_squash_merge  false → true",
		"      ~ visibility  private → public",
	}, "\n") + "\n"
	if got := buf.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
