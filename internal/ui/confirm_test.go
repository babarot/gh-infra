package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirmPrompt(t *testing.T) {
	const title = "Apply 13 changes to 3 repositories?"
	tests := []struct {
		name string
		view string
		want string
	}{
		{name: "with diff",
			view: (&confirmDiffModel{title: title}).View().Content,
			want: "\n> " + title + "  y yes   n no   d diff\n"},
		{name: "without diff",
			view: (&confirmModel{title: title}).View().Content,
			want: "\n> " + title + "  y yes   n no\n"},
		{name: "answered",
			view: (&confirmDiffModel{title: title, done: true, confirmed: true}).View().Content,
			want: "\n> " + title + " Yes\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSIForTest(tt.view); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// Ctrl+C at a prompt is not an answer: it interrupts the run as in the
// spinner. n and esc answer no without interrupting.
func TestConfirmModels_CtrlCInterrupts(t *testing.T) {
	ctrlC := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	no := tea.KeyPressMsg{Code: 'n', Text: "n"}
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}

	for _, tt := range []struct {
		name string
		key  tea.KeyPressMsg
		want int
	}{
		{"ctrl+c", ctrlC, 1},
		{"n", no, 0},
		{"esc", esc, 0},
	} {
		models := map[string]func() (tea.Model, func(tea.Model) bool){
			"confirm": func() (tea.Model, func(tea.Model) bool) {
				return &confirmModel{title: "Apply?"}, func(m tea.Model) bool { return m.(*confirmModel).confirmed }
			},
			"confirm with diff": func() (tea.Model, func(tea.Model) bool) {
				m := newConfirmDiffModel("Apply?", []DiffEntry{{Path: "a"}})
				return &m, func(m tea.Model) bool { return m.(*confirmDiffModel).confirmed }
			},
		}
		for kind, newModel := range models {
			called := 0
			restore := SetInterruptHandler(func() { called++ })
			m, confirmed := newModel()
			m, _ = m.Update(tt.key)
			restore()
			if called != tt.want {
				t.Errorf("%s, %s: interrupt handler called %d times, want %d", kind, tt.name, called, tt.want)
			}
			if confirmed(m) {
				t.Errorf("%s, %s: confirmed, want not", kind, tt.name)
			}
		}
	}
}
