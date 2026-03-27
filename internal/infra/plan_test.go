package infra

import (
	"testing"

	"github.com/babarot/gh-infra/internal/ui"
)

func TestPlanResult_Printer_WithEngine(t *testing.T) {
	p := ui.NewStandardPrinter()
	r := &PlanResult{
		engine: &engine{printer: p},
	}
	if r.Printer() != p {
		t.Error("expected Printer() to return engine's printer")
	}
}

func TestPlanResult_Printer_NilEngine(t *testing.T) {
	r := &PlanResult{}
	p := r.Printer()
	if p == nil {
		t.Fatal("expected non-nil fallback printer")
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"no duplicates", []string{"a", "b", "c"}, 3},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, 3},
		{"all same", []string{"x", "x", "x"}, 1},
		{"empty", []string{}, 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueStrings(tt.in)
			if len(got) != tt.want {
				t.Errorf("uniqueStrings(%v) = %d items, want %d", tt.in, len(got), tt.want)
			}
		})
	}
}

func TestUniqueStrings_PreservesOrder(t *testing.T) {
	got := uniqueStrings([]string{"c", "a", "b", "a", "c"})
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
