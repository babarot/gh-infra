package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// Printer is the interface for all user-facing output.
type Printer interface {
	// stderr: progress/status
	Phase(msg string)
	Progress(msg string)
	BlankLine() // empty line to stderr

	// stdout: structured output
	Separator()
	GroupHeader(icon, name string)
	GroupEnd()
	SetColumnWidth(w int) // set field/path column width for Item/SubItem
	ItemCreate(field string, value any)
	ItemUpdate(field, old, new string)
	ItemDelete(field string, value any)
	SubGroupHeader(icon, name string)
	SubItemCreate(field string, value any)
	SubItemUpdate(field, old, new string)
	SubItemDelete(field string, value any)
	FileCreate(path string)
	FileUpdate(path string)
	FileDelete(path string)
	FileDrift(path, onDrift string)
	FileSkip(path string)
	Success(name, detail string)
	Error(name, detail string)
	Warning(name, detail string) // stderr
	ResultSuccess(field, detail string)
	ResultError(field, detail string)
	ResultWarning(field, detail string)
	Summary(msg string)
	Message(msg string)

	// stderr: errors
	ErrorMessage(err error)

	// interaction
	Confirm(title string) (bool, error)

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

const Separator_ = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

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
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, Dim.Render(Separator_))
	fmt.Fprintln(p.out)
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

func (p *StandardPrinter) ItemCreate(field string, value any) {
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		Green.Render("+"), p.itemWidth(), field, Green.Render(fmt.Sprintf("%v", value)))
}

func (p *StandardPrinter) ItemUpdate(field, oldVal, newVal string) {
	fmt.Fprintf(p.out, "      %s %-*s  %s %s %s\n",
		Yellow.Render("~"), p.itemWidth(), field, Dim.Render(oldVal), Dim.Render("→"), Bold.Render(newVal))
}

func (p *StandardPrinter) ItemDelete(field string, value any) {
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		Red.Render("-"), p.itemWidth(), field, Red.Render(fmt.Sprintf("%v", value)))
}

func (p *StandardPrinter) SubGroupHeader(icon, name string) {
	fmt.Fprintf(p.out, "      %s %s\n", renderIcon(icon), Bold.Render(name))
}

func (p *StandardPrinter) SubItemCreate(field string, value any) {
	fmt.Fprintf(p.out, "          %s %-*s  %s\n",
		Green.Render("+"), p.subItemWidth(), field, Green.Render(fmt.Sprintf("%v", value)))
}

func (p *StandardPrinter) SubItemUpdate(field, oldVal, newVal string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s %s %s\n",
		Yellow.Render("~"), p.subItemWidth(), field, Dim.Render(oldVal), Dim.Render("→"), Bold.Render(newVal))
}

func (p *StandardPrinter) SubItemDelete(field string, value any) {
	fmt.Fprintf(p.out, "          %s %-*s  %s\n",
		Red.Render("-"), p.subItemWidth(), field, Red.Render(fmt.Sprintf("%v", value)))
}

func (p *StandardPrinter) FileCreate(path string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s\n",
		Green.Render("+"), p.subItemWidth(), path, Green.Render("(new file)"))
}

func (p *StandardPrinter) FileUpdate(path string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s\n",
		Yellow.Render("~"), p.subItemWidth(), path, Yellow.Render("(content changed)"))
}

func (p *StandardPrinter) FileDelete(path string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s\n",
		Red.Render("-"), p.subItemWidth(), path, Red.Render("(deleted)"))
}

func (p *StandardPrinter) FileDrift(path, onDrift string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s  on_drift: %s\n",
		Yellow.Render("⚠"), p.subItemWidth(), path, Yellow.Render("[drift]"), onDrift)
}

func (p *StandardPrinter) FileSkip(path string) {
	fmt.Fprintf(p.out, "          %s %-*s  %s  on_drift: skip\n",
		Dim.Render("-"), p.subItemWidth(), path, Dim.Render("[drift]"))
}

func renderIcon(icon string) string {
	switch icon {
	case "+", "✓":
		return Green.Render(icon)
	case "-", "✗":
		return Red.Render(icon)
	default:
		return Yellow.Render(icon)
	}
}

func (p *StandardPrinter) Success(name, detail string) {
	fmt.Fprintf(p.out, "  %s %s  %s\n", Green.Render("✓"), Bold.Render(name), detail)
}

func (p *StandardPrinter) Error(name, detail string) {
	detail = strings.ReplaceAll(detail, "\n", "\n    ")
	fmt.Fprintf(p.out, "  %s %s  %s\n", Red.Render("✗"), Bold.Render(name), detail)
}

func (p *StandardPrinter) Warning(name, detail string) {
	fmt.Fprintf(p.err, "  %s %s  %s\n", Yellow.Render("⚠"), Bold.Render(name), detail)
}

func (p *StandardPrinter) ResultSuccess(field, detail string) {
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		Green.Render("✓"), p.itemWidth(), field, detail)
}

func (p *StandardPrinter) ResultError(field, detail string) {
	detail = strings.ReplaceAll(detail, "\n", "\n          ")
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		Red.Render("✗"), p.itemWidth(), field, detail)
}

func (p *StandardPrinter) ResultWarning(field, detail string) {
	fmt.Fprintf(p.out, "      %s %-*s  %s\n",
		Yellow.Render("⚠"), p.itemWidth(), field, detail)
}

func (p *StandardPrinter) Summary(msg string) {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, Dim.Render(Separator_))
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, msg)
}

func (p *StandardPrinter) Message(msg string) {
	fmt.Fprintln(p.out, msg)
}

func (p *StandardPrinter) ErrorMessage(err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n  ")
	fmt.Fprintf(p.err, "\n%s %s\n", Red.Render("Error:"), msg)
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

// --- Package-level utilities ---

// DefaultPrinter is the package-level printer instance.
var DefaultPrinter Printer = NewStandardPrinter()

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

// FormatValue formats a value for display.
func FormatValue(v any) string {
	switch val := v.(type) {
	case []string:
		return "[" + strings.Join(val, ", ") + "]"
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}
