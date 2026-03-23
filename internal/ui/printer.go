package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
)

// Printer handles all user-facing output. All output goes through this
// to ensure consistent styling and stderr/stdout separation.
type Printer struct {
	out io.Writer // stdout (plan results, apply results)
	err io.Writer // stderr (progress, status messages)
}

// DefaultPrinter is the package-level printer.
var DefaultPrinter = &Printer{out: os.Stdout, err: os.Stderr}

// OutWriter returns the stdout writer.
func (p *Printer) OutWriter() io.Writer { return p.out }

// ErrWriter returns the stderr writer.
func (p *Printer) ErrWriter() io.Writer { return p.err }

// SetWriters overrides output destinations (for testing).
func SetWriters(out, err io.Writer) {
	DefaultPrinter.out = out
	DefaultPrinter.err = err
}

// ResetWriters restores default stdout/stderr.
func ResetWriters() {
	DefaultPrinter.out = os.Stdout
	DefaultPrinter.err = os.Stderr
}

// IsInteractive returns true if stderr is a terminal (not piped/redirected).
func IsInteractive() bool {
	f, ok := DefaultPrinter.err.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}

const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

// --- Progress messages (stderr) ---

func StartPhase(path string) {
	p := DefaultPrinter
	fmt.Fprintf(p.err, "Reading desired state from %s ...\n", path)
	fmt.Fprintf(p.err, "Fetching current state from GitHub API ...\n\n")
}

func Refreshing(name string) {
	fmt.Fprintf(DefaultPrinter.err, "  Refreshing %s...\n", name)
}

func RefreshingFileSet(repo string) {
	fmt.Fprintf(DefaultPrinter.err, "  Refreshing %s...\n", repo)
}

func Creating(name, field string) {
	fmt.Fprintf(DefaultPrinter.err, "  Creating %s %s...\n", name, field)
}

func Updating(name, field string) {
	fmt.Fprintf(DefaultPrinter.err, "  Updating %s %s...\n", name, field)
}

func Destroying(name, field string) {
	fmt.Fprintf(DefaultPrinter.err, "  Destroying %s %s...\n", name, field)
}

func Committing(repo string, fileCount int) {
	fmt.Fprintf(DefaultPrinter.err, "  Committing %s (%d files)...\n", repo, fileCount)
}

func Importing(name string) {
	fmt.Fprintf(DefaultPrinter.err, "Importing %s ...\n", name)
}

func ImportStart(count int) {
	label := "repository"
	if count != 1 {
		label = "repositories"
	}
	fmt.Fprintf(DefaultPrinter.err, "Importing %d %s from GitHub API ...\n\n", count, label)
}

func ImportOutputStart() {
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
	fmt.Fprintln(DefaultPrinter.out)
}

func ImportSummary(succeeded, failed int) {
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintf(DefaultPrinter.out, "Import complete! %s exported", Bold.Render(fmt.Sprintf("%d", succeeded)))
	if failed > 0 {
		fmt.Fprintf(DefaultPrinter.out, ", %s failed", Bold.Render(fmt.Sprintf("%d", failed)))
	}
	fmt.Fprintln(DefaultPrinter.out, ".")
}

func SkipImportError(name string, err error) {
	fmt.Fprintf(DefaultPrinter.err, "  %s skipping %s: %v\n", Yellow.Render("⚠"), name, err)
}

// --- Plan output (stdout) ---

func PlanSeparator() {
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
}

func PlanHeader(creates, updates, deletes int) {
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
	fmt.Fprintln(DefaultPrinter.out)
}

func PlanRepoGroup(name string) {
	fmt.Fprintf(DefaultPrinter.out, "  %s %s\n", Yellow.Render("~"), Bold.Render(name))
}

func PlanRepoGroupNew(name string) {
	fmt.Fprintf(DefaultPrinter.out, "  %s %s  %s\n",
		Green.Render("+"), Bold.Render(name), Green.Render("(new)"))
}

func PlanFileSetGroup(fileCount int, repo string) {
	label := fmt.Sprintf("%d file", fileCount)
	if fileCount != 1 {
		label += "s"
	}
	fmt.Fprintf(DefaultPrinter.out, "  %s FileSet: %s → %s\n",
		Yellow.Render("~"), Bold.Render(label), Bold.Render(repo))
}

func PlanCreate(field string, value any) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-30s  %s\n",
		Green.Render("+"), field, Green.Render(fmt.Sprintf("%v", value)))
}

func PlanUpdate(field string, oldVal, newVal string) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-30s  %s %s %s\n",
		Yellow.Render("~"), field, Dim.Render(oldVal), Dim.Render("→"), Bold.Render(newVal))
}

func PlanDelete(field string, value any) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-30s  %s\n",
		Red.Render("-"), field, Red.Render(fmt.Sprintf("%v", value)))
}

func PlanFileCreate(path string, width int) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-*s  %s\n",
		Green.Render("+"), width, path, Green.Render("(new file)"))
}

func PlanFileUpdate(path string, width int) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-*s  %s\n",
		Yellow.Render("~"), width, path, Yellow.Render("(content changed)"))
}

func PlanFileDrift(path, onDrift string, width int) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-*s  %s  on_drift: %s\n",
		Yellow.Render("⚠"), width, path, Yellow.Render("[drift]"), onDrift)
}

func PlanFileSkip(path string, width int) {
	fmt.Fprintf(DefaultPrinter.out, "      %s %-*s  %s  on_drift: skip\n",
		Dim.Render("-"), width, path, Dim.Render("[drift]"))
}

func PlanGroupEnd() {
	fmt.Fprintln(DefaultPrinter.out)
}

func PlanFooter(creates, updates, deletes, drifts int) {
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
	fmt.Fprintln(DefaultPrinter.out)

	parts := []string{
		fmt.Sprintf("%s to create", Bold.Render(fmt.Sprintf("%d", creates))),
		fmt.Sprintf("%s to update", Bold.Render(fmt.Sprintf("%d", updates))),
		fmt.Sprintf("%s to destroy", Bold.Render(fmt.Sprintf("%d", deletes))),
	}
	if drifts > 0 {
		parts = append(parts, fmt.Sprintf("%s drifted", Bold.Render(fmt.Sprintf("%d", drifts))))
	}
	fmt.Fprintf(DefaultPrinter.out, "Plan: %s\n", strings.Join(parts, ", "))
	fmt.Fprintf(DefaultPrinter.out, "To apply, run: %s\n", Bold.Render("gh infra apply"))
}

// --- Apply results (stdout) ---

func ResultSuccess(name, field string, changeType any) {
	fmt.Fprintf(DefaultPrinter.out, "  %s %s  %s %sd\n",
		Green.Render("✓"), Bold.Render(name), field, changeType)
}

func ResultError(name, field string, err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n    ")
	fmt.Fprintf(DefaultPrinter.out, "  %s %s  %s: %s\n",
		Red.Render("✗"), Bold.Render(name), field, msg)
}

func ResultSkipped(name, path, onDrift string) {
	fmt.Fprintf(DefaultPrinter.out, "  %s %s %s  drift detected, skipped (on_drift: %s)\n",
		Yellow.Render("⚠"), Bold.Render(name), path, onDrift)
}

func ApplySummary(succeeded, failed int) {
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintln(DefaultPrinter.out, Dim.Render(separator))
	fmt.Fprintln(DefaultPrinter.out)
	fmt.Fprintf(DefaultPrinter.out, "Apply complete! %d changes applied", succeeded)
	if failed > 0 {
		fmt.Fprintf(DefaultPrinter.out, ", %d failed", failed)
	}
	fmt.Fprintln(DefaultPrinter.out, ".")
}

// --- Refresh errors (stderr) ---

func RefreshError(name string, err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n    ")
	fmt.Fprintf(DefaultPrinter.err, "  %s %s: %s\n", Red.Render("✗"), Bold.Render(name), msg)
}

func RefreshErrorSummary(count int) {
	label := "error"
	if count > 1 {
		label = "errors"
	}
	fmt.Fprintf(DefaultPrinter.err, "\n  %s\n", Yellow.Render(fmt.Sprintf("%d %s occurred during refresh. Affected repositories were skipped.", count, label)))
}

// --- Error messages (stderr) ---

func FatalError(err error) {
	msg := strings.ReplaceAll(err.Error(), "\n", "\n  ")
	fmt.Fprintf(DefaultPrinter.err, "\n%s %s\n", Red.Render("Error:"), msg)
}

// --- Status messages (stdout) ---

func NoChanges() {
	fmt.Fprintln(DefaultPrinter.out, "\nNo changes. Infrastructure is up-to-date.")
}

func NoResources(path string) {
	fmt.Fprintln(DefaultPrinter.out, "No resources found in", path)
}

func ApplyCancelled() {
	fmt.Fprintln(DefaultPrinter.out, "Apply cancelled.")
}

// Confirm shows a yes/no prompt with the given message.
func Confirm(title string) (bool, error) {
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

// --- Validate output (stdout) ---

func ValidateSummary(repos, filesets int) {
	fmt.Fprintf(DefaultPrinter.out, "%s Valid: %d repositories, %d filesets defined\n",
		Green.Render("✓"), repos, filesets)
}

func ValidateRepo(name string) {
	fmt.Fprintf(DefaultPrinter.out, "  - Repository: %s\n", name)
}

func ValidateFileSet(name string, files, repos int) {
	fmt.Fprintf(DefaultPrinter.out, "  - FileSet: %s (%d files → %d repositories)\n", name, files, repos)
}

// --- Format helpers ---

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
