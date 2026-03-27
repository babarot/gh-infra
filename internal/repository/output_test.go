package repository

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/ui"
)

func init() {
	ui.DisableStyles()
}

// ---------------------------------------------------------------------------
// HasChanges
// ---------------------------------------------------------------------------

func TestHasChanges_WithChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp},
		{Type: ChangeUpdate, Field: "description"},
	}
	if !HasChanges(changes) {
		t.Error("expected true when non-noop changes exist")
	}
}

func TestHasChanges_WithoutChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp},
		{Type: ChangeNoOp},
	}
	if HasChanges(changes) {
		t.Error("expected false when only noop changes")
	}
}

func TestHasChanges_Empty(t *testing.T) {
	if HasChanges(nil) {
		t.Error("expected false for nil slice")
	}
	if HasChanges([]Change{}) {
		t.Error("expected false for empty slice")
	}
}

// ---------------------------------------------------------------------------
// FormatValue (now in ui package)
// ---------------------------------------------------------------------------

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"string slice", []string{"a", "b", "c"}, "[a, b, c]"},
		{"empty string slice", []string{}, "[]"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ui.FormatValue(tt.val)
			if got != tt.want {
				t.Errorf("FormatValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// countChanges
// ---------------------------------------------------------------------------

func TestCountChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeCreate},
		{Type: ChangeCreate},
		{Type: ChangeUpdate},
		{Type: ChangeDelete},
		{Type: ChangeNoOp},
		{Type: ChangeCreate},
	}
	creates, updates, deletes := countChanges(changes)
	if creates != 3 {
		t.Errorf("creates = %d, want 3", creates)
	}
	if updates != 1 {
		t.Errorf("updates = %d, want 1", updates)
	}
	if deletes != 1 {
		t.Errorf("deletes = %d, want 1", deletes)
	}
}

func TestCountChanges_Empty(t *testing.T) {
	creates, updates, deletes := countChanges(nil)
	if creates != 0 || updates != 0 || deletes != 0 {
		t.Errorf("expected all zeros, got %d/%d/%d", creates, updates, deletes)
	}
}

// ---------------------------------------------------------------------------
// groupByName
// ---------------------------------------------------------------------------

func TestGroupByName(t *testing.T) {
	changes := []Change{
		{Type: ChangeUpdate, Name: "org/repo1", Field: "description"},
		{Type: ChangeNoOp, Name: "org/repo1", Field: "homepage"},
		{Type: ChangeCreate, Name: "org/repo2", Field: "visibility"},
		{Type: ChangeDelete, Name: "org/repo1", Field: "topics"},
	}
	groups := groupByName(changes)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// First group should be org/repo1 with 2 changes (noop skipped)
	if groups[0].name != "org/repo1" {
		t.Errorf("group[0].name = %q, want org/repo1", groups[0].name)
	}
	if len(groups[0].changes) != 2 {
		t.Errorf("group[0] changes = %d, want 2", len(groups[0].changes))
	}

	// Second group should be org/repo2 with 1 change
	if groups[1].name != "org/repo2" {
		t.Errorf("group[1].name = %q, want org/repo2", groups[1].name)
	}
	if len(groups[1].changes) != 1 {
		t.Errorf("group[1] changes = %d, want 1", len(groups[1].changes))
	}
}

func TestGroupByName_AllNoOp(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp, Name: "org/repo1"},
	}
	groups := groupByName(changes)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for all noop, got %d", len(groups))
	}
}

// ---------------------------------------------------------------------------
// PrintApplyResults
// ---------------------------------------------------------------------------

func TestPrintApplyResults_Success(t *testing.T) {
	results := []ApplyResult{
		{
			Change: Change{Type: ChangeUpdate, Name: "org/repo1", Field: "description"},
			Err:    nil,
		},
		{
			Change: Change{Type: ChangeCreate, Name: "org/repo1", Field: "homepage"},
			Err:    nil,
		},
	}

	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	PrintApplyResults(p, results)
	out := buf.String()

	if count := strings.Count(out, "✓"); count != 2 {
		t.Errorf("expected 2 check marks, got %d in:\n%s", count, out)
	}
	// No summary line (handled by cmd layer now)
	if strings.Contains(out, "Apply complete!") {
		t.Errorf("expected no summary in output:\n%s", out)
	}
}

func TestPrintApplyResults_Errors(t *testing.T) {
	results := []ApplyResult{
		{
			Change: Change{Type: ChangeUpdate, Name: "org/repo1", Field: "description"},
			Err:    fmt.Errorf("permission denied"),
		},
	}

	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	PrintApplyResults(p, results)
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected cross mark in output:\n%s", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected error message in output:\n%s", out)
	}
}

func TestPrintApplyResults_Mixed(t *testing.T) {
	results := []ApplyResult{
		{
			Change: Change{Type: ChangeUpdate, Name: "org/repo1", Field: "description"},
			Err:    nil,
		},
		{
			Change: Change{Type: ChangeCreate, Name: "org/repo1", Field: "homepage"},
			Err:    fmt.Errorf("api error"),
		},
		{
			Change: Change{Type: ChangeDelete, Name: "org/repo2", Field: "topics"},
			Err:    nil,
		},
	}

	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)

	PrintApplyResults(p, results)
	out := buf.String()

	if count := strings.Count(out, "✓"); count != 2 {
		t.Errorf("expected 2 check marks, got %d in:\n%s", count, out)
	}
	if count := strings.Count(out, "✗"); count != 1 {
		t.Errorf("expected 1 cross mark, got %d in:\n%s", count, out)
	}
}

func TestCountApplyResults(t *testing.T) {
	results := []ApplyResult{
		{Err: nil},
		{Err: fmt.Errorf("fail")},
		{Err: nil},
	}
	s, f := CountApplyResults(results)
	if s != 2 || f != 1 {
		t.Errorf("CountApplyResults = (%d, %d), want (2, 1)", s, f)
	}
}
