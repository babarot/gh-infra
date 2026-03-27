package repository

import (
	"fmt"
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
// FormatValue (in ui package)
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
// CountChanges
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
	creates, updates, deletes := CountChanges(changes)
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
	creates, updates, deletes := CountChanges(nil)
	if creates != 0 || updates != 0 || deletes != 0 {
		t.Errorf("expected all zeros, got %d/%d/%d", creates, updates, deletes)
	}
}

// ---------------------------------------------------------------------------
// CountApplyResults
// ---------------------------------------------------------------------------

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
