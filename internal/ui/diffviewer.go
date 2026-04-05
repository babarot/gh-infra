package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pmezard/go-difflib/difflib"
)

// DiffEntry holds the raw content for one file change.
// Diff text is generated at render time; Tab cycles the selected action.
type DiffEntry struct {
	Path           string   // display path for the initial action
	RepoPath       string   // repo-relative path
	Target         string   // repo full name, e.g. "owner/repo"
	Icon           string   // "+", "~", "-", "⚠"
	Current        string   // current file content for selected action
	WriteCurrent   string   // current content for write action
	PatchCurrent   string   // current content for patch action
	Desired        string   // desired file content
	Action         string   // selected action
	DefaultAction  string   // default action
	AllowedActions []string // selectable actions
	WriteTarget    string   // display path for write
	PatchTarget    string   // display path for patch
	Skip           bool     // deprecated compatibility flag
}

// GenerateDiff produces a unified diff string between current and desired content.
// Tabs are expanded to spaces so indentation is consistent across context/add/delete lines.
func GenerateDiff(current, desired, path string) string {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(current),
		B:        difflib.SplitLines(desired),
		FromFile: path + " (current)",
		ToFile:   path + " (desired)",
		Context:  3,
	})
	return expandTabs(diff)
}

// expandTabs replaces tab characters with 4 spaces for consistent terminal rendering.
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
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
	entries    []DiffEntry
	cursor     int // selected file index
	scrollY    int // scroll offset in diff pane
	listOffset int // scroll offset for file list pane
	width      int
	height     int
	listWidth  int
}

// listItem represents one visual line in the file list pane.
type listItem struct {
	text     string
	entryIdx int // index into entries, or -1 for group headers
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
			m.entries[m.cursor].cycleAction()
			m.scrollY = 0

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

	diffWidth := max(m.width-m.listWidth-3, 10) // 3 for separator
	visibleHeight := m.height - 3               // reserve 3 for padding + help line

	// Left pane: file list grouped by repository
	items := m.buildListItems()

	// Find cursor's visual line and auto-scroll to keep it visible
	cursorLine := 0
	for i, item := range items {
		if item.entryIdx == m.cursor {
			cursorLine = i
			break
		}
	}
	if cursorLine < m.listOffset {
		m.listOffset = cursorLine
	}
	if cursorLine >= m.listOffset+visibleHeight {
		m.listOffset = cursorLine - visibleHeight + 1
	}

	endIdx := min(m.listOffset+visibleHeight, len(items))
	var listLines []string
	for _, item := range items[m.listOffset:endIdx] {
		listLines = append(listLines, item.text)
	}
	for len(listLines) < visibleHeight {
		listLines = append(listLines, "")
	}

	// Right pane: content generated from raw data based on selected action.
	entry := m.entries[m.cursor]
	diffLines := m.buildRightPane(entry, diffWidth)

	// Apply scroll
	start := min(m.scrollY, len(diffLines))
	end := min(start+visibleHeight, len(diffLines))
	visible := diffLines[start:end]

	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	var coloredLines []string
	for _, line := range visible {
		if line == "\x00divider\x00" {
			coloredLines = append(coloredLines, borderStyle.Render(strings.Repeat("─", diffWidth)))
			continue
		}
		if entry.effectiveAction() == "skip" {
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
	for i := range visibleHeight {
		left := padRight(listLines[i], m.listWidth)
		right := padRight(coloredLines[i], diffWidth)
		rows = append(rows, left+" "+sep+" "+right)
	}

	// Help line
	help := Dim.Render("  ↑↓/jk: select  tab: cycle write/patch/skip  d/u: scroll  q: back")

	v := tea.NewView(strings.Join(rows, "\n") + "\n\n" + help)
	v.AltScreen = true
	return v
}

// calcListWidth computes left pane width based on longest file name.
// Layout per line: " ▸ ~ filename" = 4 (prefix " ▸ ") + icon(1) + " " + filename
// So overhead = 4 + 1 + 1 = 6, plus 2 for right margin.
func (m *diffViewModel) calcListWidth() int {
	const overhead = 8 // "  ▸ ~ " prefix + margin
	maxLen := 0
	for _, e := range m.entries {
		// File entry width
		if w := len(e.DisplayPath()) + overhead; w > maxLen {
			maxLen = w
		}
		// Repo header width: "  owner/repo"
		if w := len(e.Target) + 2; w > maxLen {
			maxLen = w
		}
	}
	lw := maxLen
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

// buildRightPane generates the lines for the right pane based on the selected action.
func (m *diffViewModel) buildRightPane(entry DiffEntry, width int) []string {
	const dividerMarker = "\x00divider\x00" // sentinel, rendered in View

	lines := []string{
		Dim.Render("Action: " + renderAction(entry.Action, entry.DefaultAction)),
		Dim.Render("Target: " + entry.DisplayPath()),
		dividerMarker,
		"",
	}

	if entry.effectiveAction() == "skip" {
		lines[0] = Dim.Render("Action: skip (will not be applied)")
		if entry.Current != "" {
			lines = append(lines, strings.Split(strings.TrimRight(entry.Current, "\n"), "\n")...)
		} else {
			lines = append(lines, Dim.Render("(empty)"))
		}
		return lines
	}

	// Show unified diff
	diff := GenerateDiff(entry.Current, entry.Desired, entry.DisplayPath())
	raw := strings.Split(diff, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	return append(lines, raw...)
}

// buildListItems builds visual list lines grouped by repository (Target).
func (m *diffViewModel) buildListItems() []listItem {
	var items []listItem
	lastTarget := ""
	for i, e := range m.entries {
		if e.Target != lastTarget {
			lastTarget = e.Target
			header := "  " + Dim.Render(e.Target)
			items = append(items, listItem{text: header, entryIdx: -1})
		}

		labelWidth := m.listWidth - 8 // "  ▸ ~ " prefix + margin
		label := truncate(e.DisplayPath(), labelWidth)
		var line string
		if e.effectiveAction() == "skip" {
			if i == m.cursor {
				line = fmt.Sprintf("  ▸ %s %s", Dim.Render(e.Icon), Dim.Render(label))
			} else {
				line = fmt.Sprintf("    %s %s", Dim.Render(e.Icon), Dim.Render(label))
			}
		} else {
			icon := renderDiffIcon(e.Icon)
			if i == m.cursor {
				line = fmt.Sprintf("  ▸ %s %s", icon, Bold.Render(label))
			} else {
				line = fmt.Sprintf("    %s %s", icon, label)
			}
		}
		items = append(items, listItem{text: line, entryIdx: i})
	}
	return items
}

func (e *DiffEntry) cycleAction() {
	if len(e.AllowedActions) <= 1 {
		return
	}

	current := 0
	for i, action := range e.AllowedActions {
		if action == e.Action {
			current = i
			break
		}
	}

	e.Action = e.AllowedActions[(current+1)%len(e.AllowedActions)]
	e.Current = e.currentForAction(e.Action)
}

func (e DiffEntry) DisplayPath() string {
	switch e.effectiveAction() {
	case "patch":
		if e.PatchTarget != "" {
			return e.PatchTarget
		}
	case "write":
		if e.WriteTarget != "" {
			return e.WriteTarget
		}
	}
	return e.Path
}

func renderAction(action, defaultAction string) string {
	if action == "" {
		action = "write"
	}
	if defaultAction == "" {
		defaultAction = "write"
	}
	label := string(action)
	if action == defaultAction {
		return label + " (default)"
	}
	return label
}

func (e DiffEntry) effectiveAction() string {
	if e.Action != "" {
		return e.Action
	}
	if e.Skip {
		return "skip"
	}
	return "write"
}

func (e DiffEntry) currentForAction(action string) string {
	switch action {
	case "patch":
		return e.PatchCurrent
	case "write":
		return e.WriteCurrent
	default:
		return e.Current
	}
}

func (m *diffViewModel) clampScroll() {
	entry := m.entries[m.cursor]
	diffWidth := max(m.width-m.listWidth-3, 10)
	lines := m.buildRightPane(entry, diffWidth)
	total := len(lines)
	maxScroll := max(total-m.diffVisibleLines(), 0)
	m.scrollY = max(min(m.scrollY, maxScroll), 0)
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
