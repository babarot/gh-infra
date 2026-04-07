package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
	goyaml "github.com/goccy/go-yaml"
)

// ChangeItem represents a single field-level change for PrintChange.
type ChangeItem struct {
	Icon  string // IconAdd, IconChange, IconRemove
	Field string
	Value any    // for create/delete: the value; for update: ignored
	Old   string // for update only
	New   string // for update only
	Sub   bool   // true = sub-level indent (10), false = top-level (6)
}

// FileItem represents a file-level change for PrintFileChange.
type FileItem struct {
	Icon    string // IconAdd, IconChange, IconRemove, IconWarning
	Path    string
	Added   int
	Removed int
	Reason  string // skip reason (if set, line is dimmed and reason replaces diff stat)
}

// ResultItem represents an apply result for PrintResult.
type ResultItem struct {
	Icon   string // IconSuccess, IconError, IconWarning
	Field  string
	Detail string
}

// Printer is the interface for all user-facing output.
type Printer interface {
	// stderr: progress/status
	Phase(msg string)
	Progress(msg string)
	BlankLine() // empty line to stderr

	// stdout: structured output
	Separator()
	Legend(creates, updates, deletes bool)
	ActionHeader(name, action string) // e.g. "# babarot/repo will be created"
	GroupHeader(icon, name string)
	GroupEnd()
	SetColumnWidth(w int) // set field/path column width for alignment
	SubGroupHeader(icon, name string)
	PrintChange(item ChangeItem)
	PrintFileChange(item FileItem)
	PrintResult(item ResultItem)
	Success(name, detail string)
	Error(name, detail string)
	Warning(name, detail string) // stderr
	Detail(msg string)
	StreamStart(name, detail string)
	StreamDone(name, detail string)
	StreamError(name, detail string)
	Summary(msg string)
	Message(msg string)

	// stderr: errors
	ErrorMessage(err error)

	// interaction
	Confirm(title string) (bool, error)
	ConfirmWithDiff(title string, diffEntries []DiffEntry) (bool, error)

	// writers
	OutWriter() io.Writer
	ErrWriter() io.Writer
}

// StandardPrinter is the default terminal implementation of Printer.
type StandardPrinter struct {
	out      io.Writer
	err      io.Writer
	colWidth int // dynamic column width for Item/SubItem alignment
}

// NewStandardPrinter creates a StandardPrinter writing to stdout/stderr.
func NewStandardPrinter() *StandardPrinter {
	return &StandardPrinter{out: os.Stdout, err: os.Stderr}
}

// NewStandardPrinterWith creates a StandardPrinter with custom writers (for testing).
func NewStandardPrinterWith(out, err io.Writer) *StandardPrinter {
	return &StandardPrinter{out: out, err: err}
}

func (p *StandardPrinter) OutWriter() io.Writer { return p.out }
func (p *StandardPrinter) ErrWriter() io.Writer { return p.err }

// isOutTerminal reports whether the stdout writer is a terminal.
// Returns false when stdout is redirected to a file or pipe.
func (p *StandardPrinter) isOutTerminal() bool {
	f, ok := p.out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}

// termWidth returns the terminal width, or 0 if unavailable.
func (p *StandardPrinter) termWidth() int {
	f, ok := p.out.(*os.File)
	if !ok {
		return 0
	}
	w, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0
	}
	return w
}

const Separator_ = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

// Icon constants for plan/apply output.
const (
	IconAdd      = "+"
	IconChange   = "~"
	IconRemove   = "-"
	IconSuccess  = "✓"
	IconError    = "✗"
	IconWarning  = "⚠"
	IconArrow    = "→"
	IconEllipsis = "…"
)

func (p *StandardPrinter) Phase(msg string) {
	fmt.Fprintf(p.err, "%s\n", msg)
}

func (p *StandardPrinter) Progress(msg string) {
	fmt.Fprintf(p.err, "  %s\n", msg)
}

func (p *StandardPrinter) BlankLine() {
	fmt.Fprintln(p.err)
}

func (p *StandardPrinter) Separator() {
	if !p.isOutTerminal() {
		return
	}
	fmt.Fprintln(p.err)
	fmt.Fprintln(p.err, Dim.Render(Separator_))
	fmt.Fprintln(p.err)
}

func (p *StandardPrinter) Legend(creates, updates, deletes bool) {
	fmt.Fprintln(p.out, "Resource actions are indicated with the following symbols:")
	if creates {
		fmt.Fprintf(p.out, "  %s create\n", Green.Render(IconAdd))
	}
	if updates {
		fmt.Fprintf(p.out, "  %s update\n", Yellow.Render(IconChange))
	}
	if deletes {
		fmt.Fprintf(p.out, "  %s destroy\n", Red.Render(IconRemove))
	}
	fmt.Fprintln(p.out)
}

func (p *StandardPrinter) ActionHeader(name, action string) {
	fmt.Fprintf(p.out, "  %s %s %s\n", Dim.Render("#"), Bold.Render(name), Dim.Render(action))
}

func (p *StandardPrinter) GroupHeader(icon, name string) {
	fmt.Fprintf(p.out, "  %s %s\n", renderIcon(icon), Bold.Render(name))
}

func (p *StandardPrinter) GroupEnd() {
	fmt.Fprintln(p.out)
}

// SetColumnWidth sets the field column width for subsequent Item/SubItem/File calls.
// Pass 0 to reset to default widths.
func (p *StandardPrinter) SetColumnWidth(w int) {
	p.colWidth = w
}

// itemWidth returns the column width for top-level items (indent 6).
// Adds 4 to align with sub-items (indent 10) when using the same colWidth.
func (p *StandardPrinter) itemWidth() int {
	if p.colWidth > 0 {
		return p.colWidth + 4
	}
	return 30
}

// subItemWidth returns the column width for sub-level items (indent 10).
func (p *StandardPrinter) subItemWidth() int {
	if p.colWidth > 0 {
		return p.colWidth
	}
	return 26
}

func (p *StandardPrinter) SubGroupHeader(icon, name string) {
	fmt.Fprintf(p.out, "      %s %s\n", renderIcon(icon), Bold.Render(name))
}

// PrintChange prints a single field-level change (create, update, or delete).
// The Sub field controls indentation: false = top-level (6 spaces), true = sub-level (10 spaces).
func (p *StandardPrinter) PrintChange(item ChangeItem) {
	indent := "      " // 6 spaces
	width := p.itemWidth()
	if item.Sub {
		indent = "          " // 10 spaces
		width = p.subItemWidth()
	}
	icon := renderIcon(item.Icon)
	switch item.Icon {
	case IconAdd:
		val := FormatValue(item.Value)
		if tw := p.termWidth(); tw > 0 {
			prefix := len(indent) + 2 + 1 + width + 2
			_, val = truncateChangeValues("", val, tw, prefix)
		}
		fmt.Fprintf(p.out, "%s%s %-*s  %s\n",
			indent, icon, width, item.Field, Green.Render(val))
	case IconChange:
		old, new := item.Old, item.New
		if tw := p.termWidth(); tw > 0 {
			prefix := len(indent) + 2 + 1 + width + 2 // indent + icon + space + field (padded) + 2 spaces
			old, new = truncateChangeValues(old, new, tw, prefix)
		}
		fmt.Fprintf(p.out, "%s%s %-*s  %s %s %s\n",
			indent, icon, width, item.Field, Dim.Render(old), Dim.Render(IconArrow), Bold.Render(new))
	case IconRemove:
		val := FormatValue(item.Value)
		if tw := p.termWidth(); tw > 0 {
			prefix := len(indent) + 2 + 1 + width + 2
			_, val = truncateChangeValues("", val, tw, prefix)
		}
		fmt.Fprintf(p.out, "%s%s %-*s  %s\n",
			indent, icon, width, item.Field, Red.Render(val))
	}
}

// File change descriptions used in plan output.
// PrintFileChange prints a file-level change with diff stat.
func (p *StandardPrinter) PrintFileChange(item FileItem) {
	icon := renderIcon(item.Icon)
	if item.Reason != "" {
		// Dimmed line with skip reason instead of diff stat.
		fmt.Fprintf(p.out, "          %s %s  %s\n",
			icon, Dim.Render(fmt.Sprintf("%-*s", p.subItemWidth(), item.Path)), Dim.Render(item.Reason))
		return
	}
	stat := formatDiffStat(item.Added, item.Removed)
	fmt.Fprintf(p.out, "          %s %-*s %s\n",
		icon, p.subItemWidth(), item.Path, stat)
}

// PrintResult prints an apply result line.
func (p *StandardPrinter) PrintResult(item ResultItem) {
	icon := renderIcon(item.Icon)
	detail := item.Detail
	if item.Icon == IconError {
		detail = strings.ReplaceAll(detail, "\n", "\n          ")
	}
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		icon, p.itemWidth(), item.Field, detail)
}

// formatDiffStat formats added/removed line counts like git diff --stat.
func formatDiffStat(added, removed int) string {
	if added == 0 && removed == 0 {
		return ""
	}
	var parts []string
	if added > 0 {
		parts = append(parts, Green.Render(fmt.Sprintf("+%d", added)))
	}
	if removed > 0 {
		parts = append(parts, Red.Render(fmt.Sprintf("-%d", removed)))
	}
	return " " + strings.Join(parts, " ")
}

func renderIcon(icon string) string {
	switch icon {
	case IconAdd, IconSuccess:
		return Green.Render(icon)
	case IconRemove, IconError:
		return Red.Render(icon)
	default:
		return Yellow.Render(icon)
	}
}

func (p *StandardPrinter) Success(name, detail string) {
	fmt.Fprintf(p.out, "  %s %s  %s\n", Green.Render(IconSuccess), Bold.Render(name), detail)
}

func (p *StandardPrinter) Error(name, detail string) {
	detail = strings.ReplaceAll(detail, "\n", "\n    ")
	fmt.Fprintf(p.out, "  %s %s  %s\n", Red.Render(IconError), Bold.Render(name), detail)
}

func (p *StandardPrinter) Warning(name, detail string) {
	fmt.Fprintf(p.err, "  %s %s  %s\n", Yellow.Render(IconWarning), Bold.Render(name), detail)
}

func (p *StandardPrinter) Detail(msg string) {
	fmt.Fprintf(p.out, "      %s\n", Dim.Render(msg))
}

func (p *StandardPrinter) StreamStart(name, detail string) {
	fmt.Fprintf(p.err, "%s: %s\n", Bold.Render(name), detail)
}

func (p *StandardPrinter) StreamDone(name, detail string) {
	fmt.Fprintf(p.err, "%s: %s\n", Bold.Render(name), Green.Render(detail))
}

func (p *StandardPrinter) StreamError(name, detail string) {
	fmt.Fprintf(p.err, "%s: %s\n", Bold.Render(name), Red.Render(detail))
}

func (p *StandardPrinter) Summary(msg string) {
	if p.isOutTerminal() {
		fmt.Fprintln(p.err)
		fmt.Fprintln(p.err, Dim.Render(Separator_))
	}
	fmt.Fprintln(p.err)
	fmt.Fprintln(p.err, msg)
}

func (p *StandardPrinter) Message(msg string) {
	fmt.Fprintln(p.out, msg)
}

func (p *StandardPrinter) ErrorMessage(err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n  ")
	fmt.Fprintf(p.err, "%s %s\n", Red.Render("Error:"), msg)
}

func (p *StandardPrinter) Confirm(title string) (bool, error) {
	var confirm bool
	field := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&confirm)
	form := huh.NewForm(huh.NewGroup(field)).
		WithShowHelp(false).
		WithAccessible(true)
	if err := form.Run(); err != nil {
		return false, err
	}
	return confirm, nil
}

func (p *StandardPrinter) ConfirmWithDiff(title string, diffEntries []DiffEntry) (bool, error) {
	confirmed, err := RunConfirmWithDiff(title, diffEntries)
	if ErrFallback(err) {
		return p.Confirm(title)
	}
	return confirmed, err
}

// --- Package-level utilities ---

// DefaultPrinter is the package-level printer instance.
var DefaultPrinter Printer = NewStandardPrinter()

// OutputMode returns the apply output mode.
// Set via GH_INFRA_OUTPUT env var: "stream" or "spinner" (default).
func OutputMode() string {
	mode := os.Getenv("GH_INFRA_OUTPUT")
	if mode == "stream" {
		return "stream"
	}
	return "spinner"
}

// IsInteractive returns true if stderr is a terminal.
func IsInteractive() bool {
	f, ok := DefaultPrinter.ErrWriter().(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}

// FatalError prints a fatal error to stderr.
// Package-level because main.go cannot inject a Printer.
func FatalError(err error) {
	DefaultPrinter.ErrorMessage(err)
}

// FormatDuration formats a duration for display (e.g. "1s", "2.3s").
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// truncateChangeValues truncates old/new value strings to fit within termWidth.
// Arrow " → " is len(IconArrow)+2 bytes but 3 display columns.
// We use display-width arithmetic: arrow=3, ellipsis=1 display column.
func truncateChangeValues(old, new string, termWidth, prefixWidth int) (string, string) {
	const arrowDisplay = 3 // " → " = space + 1-col char + space
	oldLen := utf8.RuneCountInString(old)
	newLen := utf8.RuneCountInString(new)
	used := prefixWidth + oldLen + arrowDisplay + newLen
	if used <= termWidth {
		return old, new
	}

	avail := termWidth - prefixWidth - arrowDisplay
	if avail <= 0 {
		return old, new
	}

	// Give old 1/3, new 2/3 of available space; if old fits, give remainder to new
	oldMax := avail / 3
	newMax := avail - oldMax
	if oldLen <= oldMax {
		newMax = avail - oldLen
	} else if oldMax > 1 {
		old = truncateRunes(old, oldMax-1) + IconEllipsis
	}
	if newLen > newMax && newMax > 1 {
		new = truncateRunes(new, newMax-1) + IconEllipsis
	}

	return old, new
}

// truncateRunes returns the first n runes of s.
func truncateRunes(s string, n int) string {
	i := 0
	for j := range s {
		if i >= n {
			return s[:j]
		}
		i++
	}
	return s
}

// FormatValue formats a value for display.
func FormatValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return `""`
		}
		return val
	case []string:
		return "[" + strings.Join(val, ", ") + "]"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		data, err := goyaml.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return strings.TrimRight(string(data), "\n")
	}
}
