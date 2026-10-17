package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	goyaml "github.com/goccy/go-yaml"
)

// ChangeItem represents a single field-level change for PrintChange.
type ChangeItem struct {
	Icon  string // IconAdd, IconChange, IconRemove
	Field string
	Value any         // for create/delete: the value; for update: ignored
	Old   string      // for update only
	New   string      // for update only
	Level IndentLevel // IndentItem (default) or IndentSub
	Width int         // field column width; 0 uses the printer's column width
}

// FileItem represents a file-level change for PrintFileChange.
type FileItem struct {
	Icon    string // IconAdd, IconChange, IconRemove, IconWarning
	Path    string
	Added   int
	Removed int
	Reason  string // skip reason (if set, line is dimmed and reason replaces diff stat)
	OldMode string // file mode before the change; empty for a new file
	NewMode string // file mode after the change; set only when the mode changes
}

// ResultItem represents an apply result for PrintResult.
type ResultItem struct {
	Icon   string // IconSuccess, IconError, IconWarning
	Field  string
	Detail string
	Level  IndentLevel // IndentItem (default) or IndentSub
}

// Printer is the interface for all user-facing output.
type Printer interface {
	// stderr: progress/status
	Phase(msg string)
	Progress(msg string)
	BlankLine() // empty line to stderr

	// stdout: structured output
	Separator()
	RepoHeader(icon, name, detail, right string) // e.g. "~ babarot/repo        2 to create · 5 to update"
	GroupHeader(icon, name string)
	GroupEnd()
	SetColumnWidth(w int) // set field/path column width for alignment
	SubGroupHeader(icon, name string)
	PrintChange(item ChangeItem)
	PrintFileChange(item FileItem)
	PrintDiffBlock(diff string) // emit colored unified diff at IndentSub indent
	PrintResult(item ResultItem)
	Success(name, detail string)
	PullRequest(name, url string) // e.g. "↗ babarot/repo  pull request #42  <url>"
	Error(name, detail string)
	ErrorReport(name, detail string) // multi-line per-target block (stderr)
	Warning(name, detail string)     // stderr
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
	// mu keeps the lines that concurrent work prints (progress, stream lines,
	// errors and warnings) whole when several goroutines print at once, and
	// guards pendingBlank.
	mu sync.Mutex
	// pendingBlank is the blank line that ends a block (GroupEnd,
	// PrintDiffBlock), not written yet: line writes it before the next line,
	// and lineAfterBlank drops it before output that starts with its own blank
	// line. One value covers stdout and stderr, which show as one stream in a
	// terminal. It belongs to this printer only: lines printed through another
	// StandardPrinter (DefaultPrinter for the spinner's fallback) neither write
	// nor drop it, which is fine as long as they come before the body.
	pendingBlank bool
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

// errTermWidth returns the terminal width of the stderr writer, or 0 if
// unavailable. Used for wrapping error reports that are written to stderr;
// stderr may still be a TTY even when stdout has been redirected.
func (p *StandardPrinter) errTermWidth() int {
	f, ok := p.err.(*os.File)
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

// IndentLevel represents a nesting level in the output hierarchy.
type IndentLevel int

const (
	IndentRoot IndentLevel = 0 // 2 spaces  — GroupHeader, Legend, Success, Error
	IndentItem IndentLevel = 1 // 6 spaces  — SubGroupHeader, top-level changes
	IndentSub  IndentLevel = 2 // 10 spaces — sub-level changes, file changes
)

const (
	indentBase = 2 // spaces for root level
	indentUnit = 4 // additional spaces per level
)

// Indent returns the whitespace prefix string for a given level.
func Indent(level IndentLevel) string {
	return strings.Repeat(" ", indentBase+int(level)*indentUnit)
}

// continuation returns the indent for wrapped lines at a given level.
// Adds 2 spaces beyond the level indent to clear the icon column.
func continuation(level IndentLevel) string {
	return Indent(level) + "  "
}

// columnWidthOffset is the character difference between sub-item and item indent.
// Used to pad item-level columns so values align with sub-item values.
var columnWidthOffset = len(Indent(IndentSub)) - len(Indent(IndentItem))

func (p *StandardPrinter) Phase(msg string) {
	p.line(p.err, "%s\n", msg)
}

// line prints under mu, after the blank line a block left pending. Every
// output of the printer goes through line or lineAfterBlank.
func (p *StandardPrinter) line(w io.Writer, format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pendingBlank {
		fmt.Fprintln(w)
		p.pendingBlank = false
	}
	fmt.Fprintf(w, format, args...)
}

// lineAfterBlank prints output that starts with its own blank line (Summary,
// Separator, BlankLine, a Message starting with "\n"). The blank line a block
// left pending is dropped, so one blank line separates them.
func (p *StandardPrinter) lineAfterBlank(w io.Writer, format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingBlank = false
	fmt.Fprintf(w, format, args...)
}

// endBlock leaves the blank line that ends a block pending (see
// pendingBlank). Two in a row are one, and one that nothing follows is never
// written, so the body does not end with a blank line.
func (p *StandardPrinter) endBlock() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingBlank = true
}

func (p *StandardPrinter) Progress(msg string) {
	p.line(p.err, "%s%s\n", Indent(IndentRoot), msg)
}

func (p *StandardPrinter) BlankLine() {
	p.lineAfterBlank(p.err, "\n")
}

func (p *StandardPrinter) Separator() {
	// Without a terminal nothing is written, so a pending blank stays for
	// the next line rather than being dropped.
	if !p.isOutTerminal() {
		return
	}
	p.lineAfterBlank(p.err, "\n%s\n\n", Dim.Render(Separator_))
}

// repoHeaderColumn is the column where RepoHeader starts its right-hand text.
const repoHeaderColumn = 60

// RepoHeader prints the first line of a repository's block: the icon, the
// name (linked to GitHub), detail in dim (such as the manifest the repository
// comes from) and, from repoHeaderColumn on, right in dim (such as
// "2 to create · 5 to update"). detail and right may be empty.
func (p *StandardPrinter) RepoHeader(icon, name, detail, right string) {
	line := Indent(IndentRoot) + renderIcon(icon) + " " + repoStyle(name, p.linkRepos()).Render(name)
	width := len(Indent(IndentRoot)) + 2 + utf8.RuneCountInString(name)
	if detail != "" {
		line += " " + Dim.Render(detail)
		width += 1 + utf8.RuneCountInString(detail)
	}
	if right != "" {
		line += strings.Repeat(" ", max(2, repoHeaderColumn-width)) + Dim.Render(right)
	}
	p.line(p.out, "%s\n", line)
}

func (p *StandardPrinter) GroupHeader(icon, name string) {
	display := repoStyle(name, p.linkRepos()).Render(name)
	p.line(p.out, "%s%s %s\n", Indent(IndentRoot), renderIcon(icon), display)
}

// GroupEnd ends a group with a blank line, left pending (see endBlock).
func (p *StandardPrinter) GroupEnd() {
	p.endBlock()
}

// SetColumnWidth sets the field column width for subsequent Item/SubItem/File calls.
// Pass 0 to reset to default widths.
func (p *StandardPrinter) SetColumnWidth(w int) {
	p.colWidth = w
}

const defaultSubItemWidth = 26

// itemWidth returns the column width for top-level items.
// Adds columnWidthOffset to align values with sub-items at a deeper indent.
func (p *StandardPrinter) itemWidth() int {
	if p.colWidth > 0 {
		return p.colWidth + columnWidthOffset
	}
	return defaultSubItemWidth + columnWidthOffset
}

// subItemWidth returns the column width for sub-level items.
func (p *StandardPrinter) subItemWidth() int {
	if p.colWidth > 0 {
		return p.colWidth
	}
	return defaultSubItemWidth
}

// widthForLevel returns the appropriate column width for the given indent level.
func (p *StandardPrinter) widthForLevel(level IndentLevel) int {
	if level >= IndentSub {
		return p.subItemWidth()
	}
	return p.itemWidth()
}

func (p *StandardPrinter) SubGroupHeader(icon, name string) {
	p.line(p.out, "%s%s %s\n", Indent(IndentItem), renderIcon(icon), Bold.Render(name))
}

// PrintChange prints a single field-level change (create, update, or delete).
// The Sub field controls indentation: false = top-level, true = sub-level.
func (p *StandardPrinter) PrintChange(item ChangeItem) {
	level := max(item.Level, IndentItem)
	ind := Indent(level)
	width := p.widthForLevel(level)
	if item.Width > 0 {
		width = item.Width
	}
	icon := renderIcon(item.Icon)
	switch item.Icon {
	case IconAdd:
		val := FormatValue(item.Value)
		if tw := p.termWidth(); tw > 0 {
			prefix := len(ind) + 2 + 1 + width + 2
			_, val = truncateChangeValues("", val, tw, prefix)
		}
		p.line(p.out, "%s%s %-*s  %s\n",
			ind, icon, width, item.Field, Green.Render(val))
	case IconChange:
		old, new := item.Old, item.New
		if tw := p.termWidth(); tw > 0 {
			prefix := len(ind) + 2 + 1 + width + 2 // indent + icon + space + field (padded) + 2 spaces
			old, new = truncateChangeValues(old, new, tw, prefix)
		}
		p.line(p.out, "%s%s %-*s  %s %s %s\n",
			ind, icon, width, item.Field, Dim.Render(old), Dim.Render(IconArrow), Bold.Render(new))
	case IconRemove:
		val := FormatValue(item.Value)
		if tw := p.termWidth(); tw > 0 {
			prefix := len(ind) + 2 + 1 + width + 2
			_, val = truncateChangeValues("", val, tw, prefix)
		}
		p.line(p.out, "%s%s %-*s  %s\n",
			ind, icon, width, item.Field, Red.Render(val))
	}
}

// File change descriptions used in plan output.
// PrintFileChange prints a file-level change with diff stat.
func (p *StandardPrinter) PrintFileChange(item FileItem) {
	ind := Indent(IndentSub)
	icon := renderIcon(item.Icon)
	if item.Reason != "" {
		p.line(p.out, "%s%s %s  %s\n",
			ind, icon, Dim.Render(fmt.Sprintf("%-*s", p.subItemWidth(), item.Path)), Dim.Render(item.Reason))
		return
	}
	stat := formatDiffStat(item.Added, item.Removed)
	if item.NewMode != "" {
		mode := "mode " + item.NewMode
		if item.OldMode != "" {
			mode = fmt.Sprintf("mode %s %s %s", item.OldMode, IconArrow, item.NewMode)
		}
		stat += " " + Yellow.Render(mode)
	}
	p.line(p.out, "%s%s %-*s %s\n",
		ind, icon, p.subItemWidth(), item.Path, stat)
}

// PrintDiffBlock renders a unified diff string to stdout, indented at IndentSub.
// Each line is colored using the same scheme as the interactive diff viewer:
// added lines ("+") in green, removed lines ("-") in red, hunk headers ("@@") in cyan,
// file headers ("---"/"+++") in bold, context lines as-is.
// The block ends with a blank line as a visual separator, left pending (see
// endBlock), so it merges with the one GroupEnd leaves after the last file.
func (p *StandardPrinter) PrintDiffBlock(diff string) {
	if diff == "" {
		return
	}
	ind := Indent(IndentSub)
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimSuffix(diff, "\n"), "\n") {
		fmt.Fprintf(&b, "%s%s\n", ind, colorDiffLine(line, 0))
	}
	p.line(p.out, "%s", b.String())
	p.endBlock()
}

// PrintResult prints an apply result line.
func (p *StandardPrinter) PrintResult(item ResultItem) {
	level := max(item.Level, IndentItem)
	ind := Indent(level)
	width := p.widthForLevel(level)
	icon := renderIcon(item.Icon)
	detail := item.Detail
	if item.Icon == IconError {
		// Continuation indent aligns with the detail column: indent + icon(1)
		// + space(1) + padded field(width) + gap(2). Continuation lines carry
		// the same width budget as ErrorReport (errorReportWidthRatio of the
		// terminal width) so long error detail soft-wraps instead of
		// overflowing the terminal edge.
		contPrefix := strings.Repeat(" ", len(ind)+width+4)
		avail := p.termWidth()*errorReportWidthRatio/100 - len(contPrefix)
		segments := wrapDetail(detail, avail)
		detail = strings.Join(segments, "\n"+contPrefix)
	}
	p.line(p.out, "%s%s %-*s  %s\n",
		ind, icon, width, item.Field, detail)
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

// linkRepos reports whether repository names get hyperlinks. They are left
// out when styles are disabled (NO_COLOR, plan --ci) or stdout is not a
// terminal, since the escape sequences would end up in logs and copied text.
func (p *StandardPrinter) linkRepos() bool {
	return hyperlinks && p.isOutTerminal()
}

// repoStyle returns a bold style with an OSC 8 hyperlink to the GitHub repo.
// If link is false or name does not contain a slash, returns plain Bold.
func repoStyle(name string, link bool) lipgloss.Style {
	if !link || !strings.Contains(name, "/") {
		return Bold
	}
	return Bold.Hyperlink("https://github.com/" + name)
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

// PullRequest prints a pull request a repository got: the repository, its
// number, and its URL, linked like repository names (linkRepos).
func (p *StandardPrinter) PullRequest(name, url string) {
	number := ""
	if _, n, ok := strings.Cut(url, "/pull/"); ok {
		number = " #" + strings.TrimRight(n, "/")
	}
	shown := url
	if p.linkRepos() {
		shown = Bold.Hyperlink(url).Render(url)
	}
	p.line(p.out, "%s%s %s  pull request%s  %s\n", Indent(IndentRoot), Cyan.Render("↗"), repoStyle(name, p.linkRepos()).Render(name), number, shown)
}

func (p *StandardPrinter) Success(name, detail string) {
	p.line(p.out, "%s%s %s  %s\n", Indent(IndentRoot), Green.Render(IconSuccess), Bold.Render(name), detail)
}

func (p *StandardPrinter) Error(name, detail string) {
	detail = strings.ReplaceAll(detail, "\n", "\n"+continuation(IndentRoot))
	p.line(p.out, "%s%s %s  %s\n", Indent(IndentRoot), Red.Render(IconError), Bold.Render(name), detail)
}

// errorReportWidthRatio is the fraction of the terminal width that
// ErrorReport wraps its detail lines at. A value below 100 leaves a visible
// right margin that improves readability and avoids lines touching the
// terminal edge.
const errorReportWidthRatio = 99

// ErrorReport renders a multi-line error block for one target. Used by
// RefreshTracker.PrintErrors to surface per-repository fetch/validate errors
// after the spinner has finished, using consistent indentation:
//
//	<name>:
//	  <detail line 1>
//	  <detail line 2>
//
// Detail lines are soft-wrapped at word boundaries to
// errorReportWidthRatio percent of the terminal width, and continuation
// lines retain the same indent.
func (p *StandardPrinter) ErrorReport(name, detail string) {
	indent := continuation(IndentRoot)
	avail := p.errTermWidth()*errorReportWidthRatio/100 - len(indent)

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s:\n", Indent(IndentRoot), Bold.Render(name))
	for _, seg := range wrapDetail(detail, avail) {
		fmt.Fprintf(&b, "%s%s\n", indent, Dim.Render(seg))
	}
	p.line(p.err, "%s", b.String())
}

// wrapDetail splits detail into a sequence of display segments, soft-wrapping
// each embedded line at width columns on word boundaries. Single words longer
// than width are not broken. If width <= 0, existing \n separators are kept
// but no wrapping is applied.
//
// This is the shared primitive for rendering multi-line error detail consistently
// across block output (ErrorReport) and tabular output (PrintResult).
// Callers are responsible for prepending any continuation indent to each
// segment before emitting.
func wrapDetail(detail string, width int) []string {
	var segments []string
	for line := range strings.SplitSeq(detail, "\n") {
		if width <= 0 || utf8.RuneCountInString(line) <= width {
			segments = append(segments, line)
			continue
		}
		segments = append(segments, wrapByWords(line, width)...)
	}
	return segments
}

// wrapByWords wraps s into lines of at most width columns on word boundaries.
// Single words longer than width are emitted on their own line without being
// broken. If width <= 0 or s has no content, the original string is returned
// unchanged as a single segment.
func wrapByWords(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}
	var lines []string
	var cur strings.Builder
	curLen := 0
	for _, w := range words {
		wLen := utf8.RuneCountInString(w)
		if curLen == 0 {
			cur.WriteString(w)
			curLen = wLen
			continue
		}
		if curLen+1+wLen > width {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			curLen = wLen
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
		curLen += 1 + wLen
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}

func (p *StandardPrinter) Warning(name, detail string) {
	p.line(p.err, "%s%s %s  %s\n", Indent(IndentRoot), Yellow.Render(IconWarning), Bold.Render(name), detail)
}

func (p *StandardPrinter) Detail(msg string) {
	p.line(p.out, "%s%s\n", Indent(IndentItem), Dim.Render(msg))
}

func (p *StandardPrinter) StreamStart(name, detail string) {
	p.line(p.err, "%s: %s\n", Bold.Render(name), detail)
}

func (p *StandardPrinter) StreamDone(name, detail string) {
	p.line(p.err, "%s: %s\n", Bold.Render(name), Green.Render(detail))
}

func (p *StandardPrinter) StreamError(name, detail string) {
	p.line(p.err, "%s: %s\n", Bold.Render(name), Red.Render(detail))
}

func (p *StandardPrinter) Summary(msg string) {
	sep := ""
	if p.isOutTerminal() {
		sep = "\n" + Dim.Render(Separator_) + "\n"
	}
	p.lineAfterBlank(p.err, "%s\n%s\n", sep, msg)
}

func (p *StandardPrinter) Message(msg string) {
	if strings.HasPrefix(msg, "\n") {
		p.lineAfterBlank(p.out, "%s\n", msg)
		return
	}
	p.line(p.out, "%s\n", msg)
}

func (p *StandardPrinter) ErrorMessage(err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n"+Indent(IndentRoot))
	p.line(p.err, "%s %s\n", Red.Render("Error:"), msg)
}

func (p *StandardPrinter) Confirm(title string) (bool, error) {
	m := &confirmModel{title: title}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return false, err
	}
	cm, ok := result.(*confirmModel)
	if !ok {
		return false, fmt.Errorf("unexpected model type: %T", result)
	}
	return cm.confirmed, nil
}

func (p *StandardPrinter) ConfirmWithDiff(title string, diffEntries []DiffEntry) (bool, error) {
	if len(diffEntries) == 0 || !term.IsTerminal(os.Stdin.Fd()) {
		return p.Confirm(title)
	}
	m := newConfirmDiffModel(title, diffEntries)
	result, err := tea.NewProgram(&m).Run()
	if err != nil {
		return false, err
	}
	cm, ok := result.(*confirmDiffModel)
	if !ok {
		return false, fmt.Errorf("unexpected model type: %T", result)
	}
	return cm.confirmed, nil
}

// --- Package-level utilities ---

// DefaultPrinter is the package-level printer instance.
var DefaultPrinter Printer = NewStandardPrinter()

// SetDefaultPrinter replaces DefaultPrinter and returns a function that
// restores the previous one.
func SetDefaultPrinter(p Printer) (restore func()) {
	prev := DefaultPrinter
	DefaultPrinter = p
	return func() { DefaultPrinter = prev }
}

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
