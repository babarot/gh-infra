package ui

import (
	"testing"
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
