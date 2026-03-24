package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

// RunConfirmWithDiff runs a y/n/d confirmation prompt backed by bubbletea.
// If diffEntries is empty or stdin is not a TTY, falls back to the huh-based Confirm.
func RunConfirmWithDiff(title string, diffEntries []DiffEntry) (confirmed bool, err error) {
	if len(diffEntries) == 0 || !term.IsTerminal(os.Stdin.Fd()) {
		return false, errFallback
	}

	m := newConfirmDiffModel(title, diffEntries)
	prog := tea.NewProgram(&m)
	result, err := prog.Run()
	if err != nil {
		return false, err
	}
	cm := result.(*confirmDiffModel)
	return cm.confirmed, nil
}

// errFallback signals that ConfirmWithDiff should fall back to the plain Confirm.
var errFallback = fmt.Errorf("fallback")

// ErrFallback returns true if the error signals a fallback to plain Confirm.
func ErrFallback(err error) bool { return err == errFallback }

// confirmDiffModel is a bubbletea model for the y/n/d confirmation prompt.
type confirmDiffModel struct {
	title       string
	diffEntries []DiffEntry
	confirmed   bool
	showDiff    bool
}

func newConfirmDiffModel(title string, entries []DiffEntry) confirmDiffModel {
	return confirmDiffModel{title: title, diffEntries: entries}
}

func (m *confirmDiffModel) Init() tea.Cmd { return nil }

func (m *confirmDiffModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			return m, tea.Quit
		case "n", "N", "esc", "ctrl+c":
			m.confirmed = false
			return m, tea.Quit
		case "d", "D":
			m.showDiff = true
			return m, func() tea.Msg { return showDiffMsg{} }
		}
	case showDiffMsg:
		if m.showDiff {
			m.showDiff = false
			return m, tea.Exec(
				newDiffViewerCmd(m.diffEntries),
				func(err error) tea.Msg { return diffDoneMsg{err: err} },
			)
		}
	case diffDoneMsg:
		if msg.err != nil {
			m.confirmed = false
			return m, tea.Quit
		}
	}
	return m, nil
}

// huhIndigo matches the huh Charm theme's title color (#7571F9).
var huhIndigo = lipgloss.NewStyle().Foreground(lipgloss.Color("#7571F9")).Bold(true)

func (m *confirmDiffModel) View() tea.View {
	var b strings.Builder

	// Show on_drift overrides if any
	overrides := DriftOverrides(m.diffEntries)
	if len(overrides) > 0 {
		b.WriteString("\n")
		b.WriteString(Yellow.Render("  on_drift overrides (this run only):") + "\n")
		for _, o := range overrides {
			b.WriteString(fmt.Sprintf("    %s: %s → %s\n",
				o.Path,
				Dim.Render(o.From),
				Bold.Render(o.To),
			))
		}
	}

	b.WriteString(fmt.Sprintf("\n%s %s (%s / %s / %s)\n",
		huhIndigo.Render(">"),
		huhIndigo.Render(m.title),
		Green.Render("(y)")+"es",
		Red.Render("(n)")+"o",
		Yellow.Render("(d)")+"iff",
	))
	return tea.NewView(b.String())
}

type showDiffMsg struct{}
type diffDoneMsg struct{ err error }

// diffViewerExecCmd wraps the diff viewer as a tea.ExecCommand so bubbletea
// handles terminal state transitions (altscreen, raw mode) cleanly.
type diffViewerExecCmd struct {
	entries []DiffEntry
}

func newDiffViewerCmd(entries []DiffEntry) *diffViewerExecCmd {
	return &diffViewerExecCmd{entries: entries}
}

func (c *diffViewerExecCmd) Run() error {
	return RunDiffViewer(c.entries)
}

func (c *diffViewerExecCmd) SetStdin(_ io.Reader)  {}
func (c *diffViewerExecCmd) SetStdout(_ io.Writer) {}
func (c *diffViewerExecCmd) SetStderr(_ io.Writer) {}
