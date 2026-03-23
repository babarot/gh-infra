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

	// stdout: structured output
	Separator()
	GroupHeader(icon, name string)
	GroupEnd()
	ItemCreate(field string, value any)
	ItemUpdate(field, old, new string)
	ItemDelete(field string, value any)
	Success(name, detail string)
	Error(name, detail string)
	Warning(name, detail string) // stderr
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
	out io.Writer
	err io.Writer
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

func (p *StandardPrinter) Separator() {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, Dim.Render(Separator_))
	fmt.Fprintln(p.out)
}

func (p *StandardPrinter) GroupHeader(icon, name string) {
	var styledIcon string
	switch icon {
	case "+":
		styledIcon = Green.Render("+")
	case "-":
		styledIcon = Red.Render("-")
	default:
		styledIcon = Yellow.Render(icon)
	}
	fmt.Fprintf(p.out, "  %s %s\n", styledIcon, Bold.Render(name))
}

func (p *StandardPrinter) GroupEnd() {
	fmt.Fprintln(p.out)
}

func (p *StandardPrinter) ItemCreate(field string, value any) {
	fmt.Fprintf(p.out, "      %s %-30s  %s\n",
		Green.Render("+"), field, Green.Render(fmt.Sprintf("%v", value)))
}

func (p *StandardPrinter) ItemUpdate(field, oldVal, newVal string) {
	fmt.Fprintf(p.out, "      %s %-30s  %s %s %s\n",
		Yellow.Render("~"), field, Dim.Render(oldVal), Dim.Render("→"), Bold.Render(newVal))
}

func (p *StandardPrinter) ItemDelete(field string, value any) {
	fmt.Fprintf(p.out, "      %s %-30s  %s\n",
		Red.Render("-"), field, Red.Render(fmt.Sprintf("%v", value)))
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
