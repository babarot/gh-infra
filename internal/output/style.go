package output

import "charm.land/lipgloss/v2"

var (
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	Bold   = lipgloss.NewStyle().Bold(true)
	Dim    = lipgloss.NewStyle().Faint(true)
)

// DisableStyles resets all styles to plain text (no color, no formatting).
// Used in tests to make output assertions simple.
func DisableStyles() {
	Green = lipgloss.NewStyle()
	Red = lipgloss.NewStyle()
	Yellow = lipgloss.NewStyle()
	Cyan = lipgloss.NewStyle()
	Bold = lipgloss.NewStyle()
	Dim = lipgloss.NewStyle()
}
