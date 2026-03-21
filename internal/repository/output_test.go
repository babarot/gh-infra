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
// HasRealChanges
// ---------------------------------------------------------------------------

func TestHasRealChanges_WithChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp},
		{Type: ChangeUpdate, Field: "description"},
	}
	if !HasRealChanges(changes) {
		t.Error("expected true when non-noop changes exist")
	}
}

func TestHasRealChanges_WithoutChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp},
		{Type: ChangeNoOp},
	}
	if HasRealChanges(changes) {
		t.Error("expected false when only noop changes")
	}
}

func TestHasRealChanges_Empty(t *testing.T) {
	if HasRealChanges(nil) {
		t.Error("expected false for nil slice")
	}
	if HasRealChanges([]Change{}) {
		t.Error("expected false for empty slice")
	}
}

// ---------------------------------------------------------------------------
// formatValue
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
			got := formatValue(tt.val)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.val, got, tt.want)
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
// PrintPlan
// ---------------------------------------------------------------------------

func TestPrintPlan_WithChanges(t *testing.T) {
	changes := []Change{
		{Type: ChangeCreate, Name: "org/repo1", Field: "description", NewValue: "new desc"},
		{Type: ChangeUpdate, Name: "org/repo1", Field: "visibility", OldValue: "public", NewValue: "private"},
		{Type: ChangeDelete, Name: "org/repo2", Field: "homepage", OldValue: "https://old.example.com"},
	}

	var buf bytes.Buffer
	PrintPlan(&buf, changes)
	out := buf.String()

	// Verify summary line
	if !strings.Contains(out, "1 to create") {
		t.Errorf("expected '1 to create' in output:\n%s", out)
	}
	if !strings.Contains(out, "1 to update") {
		t.Errorf("expected '1 to update' in output:\n%s", out)
	}
	if !strings.Contains(out, "1 to destroy") {
		t.Errorf("expected '1 to destroy' in output:\n%s", out)
	}

	// Verify create line has +
	if !strings.Contains(out, "+ description:") {
		t.Errorf("expected '+ description:' in output:\n%s", out)
	}

	// Verify update line has ~
	if !strings.Contains(out, "~ visibility:") {
		t.Errorf("expected '~ visibility:' in output:\n%s", out)
	}

	// Verify delete line has -
	if !strings.Contains(out, "- homepage:") {
		t.Errorf("expected '- homepage:' in output:\n%s", out)
	}

	// Verify footer
	if !strings.Contains(out, "gh infra apply") {
		t.Errorf("expected 'gh infra apply' in output:\n%s", out)
	}
}

func TestPrintPlan_EmptyChanges(t *testing.T) {
	var buf bytes.Buffer
	PrintPlan(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty changes, got %q", buf.String())
	}

	buf.Reset()
	PrintPlan(&buf, []Change{})
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty slice, got %q", buf.String())
	}
}

func TestPrintPlan_OnlyNoOp(t *testing.T) {
	changes := []Change{
		{Type: ChangeNoOp, Name: "org/repo1", Field: "description"},
	}
	var buf bytes.Buffer
	PrintPlan(&buf, changes)
	out := buf.String()

	// Should still output the summary line (0 to create, etc.) because len(changes) > 0
	if !strings.Contains(out, "0 to create") {
		t.Errorf("expected '0 to create' in output:\n%s", out)
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
	PrintApplyResults(&buf, results)
	out := buf.String()

	if count := strings.Count(out, "✓"); count != 2 {
		t.Errorf("expected 2 check marks, got %d in:\n%s", count, out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("expected '2' succeeded in output:\n%s", out)
	}
	if !strings.Contains(out, "Apply complete!") {
		t.Errorf("expected 'Apply complete!' in output:\n%s", out)
	}
	// No "failed" text
	if strings.Contains(out, "failed") {
		t.Errorf("expected no 'failed' text in output:\n%s", out)
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
	PrintApplyResults(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected cross mark in output:\n%s", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected error message in output:\n%s", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected '1 failed' in output:\n%s", out)
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
	PrintApplyResults(&buf, results)
	out := buf.String()

	if count := strings.Count(out, "✓"); count != 2 {
		t.Errorf("expected 2 check marks, got %d in:\n%s", count, out)
	}
	if count := strings.Count(out, "✗"); count != 1 {
		t.Errorf("expected 1 cross mark, got %d in:\n%s", count, out)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("expected '2' succeeded in output:\n%s", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected '1 failed' in output:\n%s", out)
	}
}
