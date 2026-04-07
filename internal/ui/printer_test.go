package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func init() {
	DisableStyles()
}

// ---------------------------------------------------------------------------
// FormatValue
// ---------------------------------------------------------------------------

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, "<nil>"},
		{"string slice", []string{"a", "b"}, "[a, b]"},
		{"empty slice", []string{}, "[]"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"string", "hello", "hello"},
		{"empty string", "", `""`},
		{"int", 42, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatValue(tt.val); got != tt.want {
				t.Errorf("FormatValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatDuration
// ---------------------------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1.0s"},
		{2500 * time.Millisecond, "2.5s"},
		{100 * time.Millisecond, "100ms"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatDuration(tt.d); got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatDiffStat
// ---------------------------------------------------------------------------

func TestFormatDiffStat(t *testing.T) {
	tests := []struct {
		name           string
		added, removed int
		wantPlus       bool
		wantMinus      bool
		wantEmpty      bool
	}{
		{"both zero", 0, 0, false, false, true},
		{"added only", 5, 0, true, false, false},
		{"removed only", 0, 3, false, true, false},
		{"both", 2, 1, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDiffStat(tt.added, tt.removed)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty, got %q", got)
			}
			if tt.wantPlus && !strings.Contains(got, "+") {
				t.Errorf("expected + in %q", got)
			}
			if tt.wantMinus && !strings.Contains(got, "-") {
				t.Errorf("expected - in %q", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// renderIcon
// ---------------------------------------------------------------------------

func TestRenderIcon(t *testing.T) {
	// With styles disabled, renderIcon returns the icon as-is
	tests := []string{IconAdd, IconChange, IconRemove, IconSuccess, IconError}
	for _, icon := range tests {
		got := renderIcon(icon)
		if !strings.Contains(got, icon) {
			t.Errorf("renderIcon(%q) = %q, expected to contain icon", icon, got)
		}
	}
}

// ---------------------------------------------------------------------------
// PrintChange
// ---------------------------------------------------------------------------

func TestPrintChange_Create(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintChange(ChangeItem{Icon: IconAdd, Field: "description", Value: "hello"})
	out := buf.String()

	if !strings.Contains(out, "+") {
		t.Errorf("expected + icon, got:\n%s", out)
	}
	if !strings.Contains(out, "description") {
		t.Errorf("expected field name, got:\n%s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected value, got:\n%s", out)
	}
}

func TestPrintChange_Update(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintChange(ChangeItem{Icon: IconChange, Field: "visibility", Old: "private", New: "public"})
	out := buf.String()

	if !strings.Contains(out, "~") {
		t.Errorf("expected ~ icon, got:\n%s", out)
	}
	if !strings.Contains(out, "private") || !strings.Contains(out, "public") {
		t.Errorf("expected old/new values, got:\n%s", out)
	}
}

func TestPrintChange_Delete(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintChange(ChangeItem{Icon: IconRemove, Field: "homepage", Value: "https://old.com"})
	out := buf.String()

	if !strings.Contains(out, "-") {
		t.Errorf("expected - icon, got:\n%s", out)
	}
}

func TestPrintChange_SubIndent(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintChange(ChangeItem{Icon: IconAdd, Field: "issues", Value: true, Sub: true})
	out := buf.String()

	// Sub-level should have more leading spaces than top-level
	if !strings.HasPrefix(out, "          ") {
		t.Errorf("expected 10-space indent for Sub, got:\n%q", out)
	}
}

// ---------------------------------------------------------------------------
// PrintFileChange
// ---------------------------------------------------------------------------

func TestPrintFileChange(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintFileChange(FileItem{Icon: IconAdd, Path: "ci.yml", Added: 10})
	out := buf.String()

	if !strings.Contains(out, "ci.yml") {
		t.Errorf("expected path, got:\n%s", out)
	}
	if !strings.Contains(out, "+10") {
		t.Errorf("expected +10 stat, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// PrintResult
// ---------------------------------------------------------------------------

func TestPrintResult_Success(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintResult(ResultItem{Icon: IconSuccess, Field: "description", Detail: "updated"})
	out := buf.String()

	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓, got:\n%s", out)
	}
	if !strings.Contains(out, "description") {
		t.Errorf("expected field, got:\n%s", out)
	}
}

func TestPrintResult_Error(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)

	p.PrintResult(ResultItem{Icon: IconError, Field: "visibility", Detail: "forbidden"})
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗, got:\n%s", out)
	}
	if !strings.Contains(out, "forbidden") {
		t.Errorf("expected error detail, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// StandardPrinter output methods
// ---------------------------------------------------------------------------

func TestPhase(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Phase("Loading...")
	if !strings.Contains(buf.String(), "Loading...") {
		t.Errorf("expected phase message, got: %q", buf.String())
	}
}

func TestMessage(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Message("No changes.")
	if !strings.Contains(buf.String(), "No changes.") {
		t.Errorf("expected message, got: %q", buf.String())
	}
}

func TestSummary(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Summary("Plan: 1 to create")
	if !strings.Contains(buf.String(), "Plan: 1 to create") {
		t.Errorf("expected summary, got: %q", buf.String())
	}
}

func TestErrorMessage(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	p := NewStandardPrinterWith(&outBuf, &errBuf)
	p.ErrorMessage(fmt.Errorf("something broke"))
	if !strings.Contains(errBuf.String(), "something broke") {
		t.Errorf("expected error on stderr, got: %q", errBuf.String())
	}
}

func TestLegend(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Legend(true, true, false)
	out := buf.String()
	if !strings.Contains(out, "create") {
		t.Errorf("expected 'create' in legend, got:\n%s", out)
	}
	if !strings.Contains(out, "update") {
		t.Errorf("expected 'update' in legend, got:\n%s", out)
	}
	if strings.Contains(out, "destroy") {
		t.Errorf("expected no 'destroy' in legend, got:\n%s", out)
	}
}

func TestGroupHeaderAndEnd(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.GroupHeader(IconChange, "org/repo")
	p.GroupEnd()
	out := buf.String()
	if !strings.Contains(out, "org/repo") {
		t.Errorf("expected repo name, got:\n%s", out)
	}
}

func TestSuccess(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Success("org/repo", "description updated")
	out := buf.String()
	if !strings.Contains(out, "✓") || !strings.Contains(out, "org/repo") {
		t.Errorf("expected success output, got:\n%s", out)
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	p := NewStandardPrinterWith(&buf, &buf)
	p.Error("org/repo", "permission denied")
	out := buf.String()
	if !strings.Contains(out, "✗") || !strings.Contains(out, "permission denied") {
		t.Errorf("expected error output, got:\n%s", out)
	}
}

func TestWarning(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	p := NewStandardPrinterWith(&outBuf, &errBuf)
	p.Warning("deprecation", "field X is deprecated")
	if !strings.Contains(errBuf.String(), "deprecated") {
		t.Errorf("expected warning on stderr, got: %q", errBuf.String())
	}
}

func TestSetColumnWidth(t *testing.T) {
	p := NewStandardPrinterWith(&bytes.Buffer{}, &bytes.Buffer{})
	p.SetColumnWidth(20)
	if p.itemWidth() != 24 { // +4 for top-level
		t.Errorf("itemWidth = %d, want 24", p.itemWidth())
	}
	if p.subItemWidth() != 20 {
		t.Errorf("subItemWidth = %d, want 20", p.subItemWidth())
	}
	p.SetColumnWidth(0)
	if p.itemWidth() != 30 { // default
		t.Errorf("default itemWidth = %d, want 30", p.itemWidth())
	}
}
