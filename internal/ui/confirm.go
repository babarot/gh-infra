package ui

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Simple y/n confirm (single-keypress, no Enter required)
type confirmModel struct {
	title     string
	confirmed bool
	done      bool
}

func (m *confirmModel) Init() tea.Cmd { return nil }

func (m *confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		case "n", "N", "esc", "ctrl+c":
			m.confirmed = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *confirmModel) View() tea.View {
	var b strings.Builder
	if m.done {
		answer := Red.Render("No")
		if m.confirmed {
			answer = Green.Render("Yes")
		}
		fmt.Fprintf(&b, "\n%s %s %s\n\n",
			promptAccent.Render(">"),
			promptAccent.Render(m.title),
			answer,
		)
		return tea.NewView(b.String())
	}
	fmt.Fprintf(&b, "\n%s %s (%s / %s)\n",
		promptAccent.Render(">"),
		promptAccent.Render(m.title),
		Green.Render("(y)")+"es",
		Red.Render("(n)")+"o",
	)
	return tea.NewView(b.String())
}

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

// confirmDiffModel is a bubbletea model for the y/n/d confirmation prompt.
type confirmDiffModel struct {
	title       string
	diffEntries []DiffEntry
	confirmed   bool
	showDiff    bool
	done        bool
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
			m.done = true
			return m, tea.Quit
		case "n", "N", "esc", "ctrl+c":
			m.confirmed = false
			m.done = true
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

// promptAccent is the style for the confirm prompt indicator and title.
var promptAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("#7571F9")).Bold(true)

func (m *confirmDiffModel) View() tea.View {
	var b strings.Builder

	if m.done {
		answer := Red.Render("No")
		if m.confirmed {
			answer = Green.Render("Yes")
		}
		fmt.Fprintf(&b, "\n%s %s %s\n\n",
			promptAccent.Render(">"),
			promptAccent.Render(m.title),
			answer,
		)
		return tea.NewView(b.String())
	}

	// Show skipped files if any
	var selected []DiffEntry
	for _, e := range m.diffEntries {
		if e.Action != e.DefaultAction {
			selected = append(selected, e)
		}
	}
	if len(selected) > 0 {
		b.WriteString("\n")
		b.WriteString(Dim.Render("  Non-default actions:") + "\n")
		for _, e := range selected {
			fmt.Fprintf(&b, "    %s -> %s\n", Dim.Render(e.DisplayPath()), Dim.Render(string(e.Action)))
		}
	}

	fmt.Fprintf(&b, "\n%s %s (%s / %s / %s)\n",
		promptAccent.Render(">"),
		promptAccent.Render(m.title),
		Green.Render("(y)")+"es",
		Red.Render("(n)")+"o",
		Yellow.Render("(d)")+"iff",
	)
	return tea.NewView(b.String())
}

type showDiffMsg struct{}
type diffDoneMsg struct{ err error }
