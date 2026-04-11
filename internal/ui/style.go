package ui

import (
	"os"

	"charm.land/lipgloss/v2"
)

var (
	Green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	Bold   = lipgloss.NewStyle().Bold(true)
	Dim    = lipgloss.NewStyle().Faint(true)
)

func init() {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		DisableStyles()
	}
}

// DisableStyles resets all styles to plain text (no color, no formatting).
func DisableStyles() {
	Green = lipgloss.NewStyle()
	Red = lipgloss.NewStyle()
	Yellow = lipgloss.NewStyle()
	Cyan = lipgloss.NewStyle()
	Bold = lipgloss.NewStyle()
	Dim = lipgloss.NewStyle()
}
