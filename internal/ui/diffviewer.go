package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pmezard/go-difflib/difflib"
)

// DiffEntry holds the raw content for one file change.
// Diff text is generated at render time; Tab toggles Skip.
type DiffEntry struct {
	Path    string // file path
	Target  string // repo full name, e.g. "owner/repo"
	Icon    string // "+", "~", "-", "⚠"
	Current string // current file content
	Desired string // desired file content
	Skip    bool   // true = will not be applied (default false = apply)
}

// GenerateDiff produces a unified diff string between current and desired content.
func GenerateDiff(current, desired, path string) string {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(current),
		B:        difflib.SplitLines(desired),
		FromFile: path + " (current)",
		ToFile:   path + " (desired)",
		Context:  3,
	})
	return diff
}

// RunDiffViewer launches an interactive full-screen diff viewer.
func RunDiffViewer(entries []DiffEntry) error {
	if len(entries) == 0 {
		return nil
	}
	m := newDiffViewModel(entries)
	p := tea.NewProgram(&m)
	_, err := p.Run()
	return err
}

// --- bubbletea model ---

type diffViewModel struct {
	entries   []DiffEntry
	cursor    int // selected file index
	scrollY   int // scroll offset in diff pane
	width     int
	height    int
	listWidth int
}

func newDiffViewModel(entries []DiffEntry) diffViewModel {
	return diffViewModel{
		entries:   entries,
		listWidth: 30,
	}
}

func (m *diffViewModel) Init() tea.Cmd {
	return nil
}

func (m *diffViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.listWidth = m.calcListWidth()
		m.clampScroll()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit

		// file list navigation
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.scrollY = 0
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.scrollY = 0
			}
		case "tab":
			m.entries[m.cursor].Skip = !m.entries[m.cursor].Skip

		// diff scrolling
		case "d", "pgdown":
			m.scrollY += m.diffVisibleLines() / 2
			m.clampScroll()
		case "u", "pgup":
			m.scrollY -= m.diffVisibleLines() / 2
			m.clampScroll()
		}
	}
	return m, nil
}

func (m *diffViewModel) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	diffWidth := m.width - m.listWidth - 3 // 3 for separator
	if diffWidth < 10 {
		diffWidth = 10
	}
	visibleHeight := m.height - 3 // reserve 3 for padding + help line

	// Left pane: file list
	var listLines []string
	for i, e := range m.entries {
		labelWidth := m.listWidth - 6 // " ▸ ~ " = 5 + margin
		label := truncate(e.Path, labelWidth)
		var line string
		if e.Skip {
			// Grayed out when skipped
			if i == m.cursor {
				line = fmt.Sprintf(" ▸ %s %s", Dim.Render(e.Icon), Dim.Render(label))
			} else {
				line = fmt.Sprintf("   %s %s", Dim.Render(e.Icon), Dim.Render(label))
			}
		} else {
			icon := renderDiffIcon(e.Icon)
			if i == m.cursor {
				line = fmt.Sprintf(" ▸ %s %s", icon, Bold.Render(label))
			} else {
				line = fmt.Sprintf("   %s %s", icon, label)
			}
		}
		listLines = append(listLines, line)
	}
	// Pad to fill height
	for len(listLines) < visibleHeight {
		listLines = append(listLines, "")
	}
	if len(listLines) > visibleHeight {
		listLines = listLines[:visibleHeight]
	}

	// Right pane: content generated from raw data based on skip flag
	entry := m.entries[m.cursor]
	diffLines := m.buildRightPane(entry, diffWidth)

	// Apply scroll
	start := m.scrollY
	if start > len(diffLines) {
		start = len(diffLines)
	}
	end := start + visibleHeight
	if end > len(diffLines) {
		end = len(diffLines)
	}
	visible := diffLines[start:end]

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	var coloredLines []string
	for _, line := range visible {
		if line == "\x00divider\x00" {
			coloredLines = append(coloredLines, borderStyle.Render(strings.Repeat("─", diffWidth)))
			continue
		}
		if entry.Skip {
			// No diff coloring for skipped files — show plain text
			coloredLines = append(coloredLines, truncate(line, diffWidth))
		} else {
			coloredLines = append(coloredLines, colorDiffLine(line, diffWidth))
		}
	}
	for len(coloredLines) < visibleHeight {
		coloredLines = append(coloredLines, "")
	}

	// Compose panes
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("│")
	var rows []string
	for i := 0; i < visibleHeight; i++ {
		left := padRight(listLines[i], m.listWidth)
		right := padRight(coloredLines[i], diffWidth)
		rows = append(rows, left+" "+sep+" "+right)
	}

	// Help line
	help := Dim.Render("  ↑↓/jk: select  tab: toggle apply/skip  d/u: scroll  q: back")

	v := tea.NewView(strings.Join(rows, "\n") + "\n\n" + help)
	v.AltScreen = true
	return v
}

// calcListWidth computes left pane width based on longest file name.
// Layout per line: " ▸ ~ filename" = 4 (prefix " ▸ ") + icon(1) + " " + filename
// So overhead = 4 + 1 + 1 = 6, plus 2 for right margin.
func (m *diffViewModel) calcListWidth() int {
	const overhead = 8 // " ▸ ~ " + 2 margin
	maxPath := 0
	for _, e := range m.entries {
		if len(e.Path) > maxPath {
			maxPath = len(e.Path)
		}
	}
	lw := maxPath + overhead
	// Ensure at least half the terminal is available for the diff pane
	maxAllowed := m.width / 2
	if lw > maxAllowed {
		lw = maxAllowed
	}
	if lw < 20 {
		lw = 20
	}
	return lw
}

func (m *diffViewModel) diffVisibleLines() int {
	h := m.height - 2
	if h < 1 {
		return 1
	}
	return h
}

// buildRightPane generates the lines for the right pane based on the skip flag.
func (m *diffViewModel) buildRightPane(entry DiffEntry, width int) []string {
	const dividerMarker = "\x00divider\x00" // sentinel, rendered in View

	if entry.Skip {
		lines := []string{Dim.Render("Skipped (will not be applied):"), dividerMarker, ""}
		if entry.Current != "" {
			lines = append(lines, strings.Split(strings.TrimRight(entry.Current, "\n"), "\n")...)
		} else {
			lines = append(lines, Dim.Render("(empty)"))
		}
		return lines
	}

	// Show unified diff
	diff := GenerateDiff(entry.Current, entry.Desired, entry.Path)
	raw := strings.Split(diff, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	return raw
}

func (m *diffViewModel) clampScroll() {
	entry := m.entries[m.cursor]
	diffWidth := m.width - m.listWidth - 3
	if diffWidth < 10 {
		diffWidth = 10
	}
	lines := m.buildRightPane(entry, diffWidth)
	total := len(lines)
	maxScroll := total - m.diffVisibleLines()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollY > maxScroll {
		m.scrollY = maxScroll
	}
	if m.scrollY < 0 {
		m.scrollY = 0
	}
}

func renderDiffIcon(icon string) string {
	switch icon {
	case "+":
		return Green.Render(icon)
	case "~":
		return Yellow.Render(icon)
	case "-":
		return Red.Render(icon)
	case "⚠":
		return Yellow.Render(icon)
	default:
		return icon
	}
}

func colorDiffLine(line string, maxWidth int) string {
	display := truncate(line, maxWidth)
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		return Green.Render(display)
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		return Red.Render(display)
	}
	if strings.HasPrefix(line, "@@") {
		return Cyan.Render(display)
	}
	if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
		return Bold.Render(display)
	}
	return display
}

func truncate(s string, maxWidth int) string {
	if len(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return s[:maxWidth]
	}
	return s[:maxWidth-3] + "..."
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}
