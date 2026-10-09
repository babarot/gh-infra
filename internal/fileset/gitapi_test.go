package fileset

import "testing"

func TestNeedsGitDataAPI(t *testing.T) {
	tests := []struct {
		name    string
		changes []Change
		want    bool
	}{
		{"empty", nil, false},
		{"no mode set", []Change{
			{Path: "a.txt", Type: ChangeCreate},
			{Path: "b.txt", Type: ChangeUpdate},
			{Path: "c.txt", Type: ChangeDelete},
		}, false},
		{"create executable", []Change{
			{Path: "a.txt", Type: ChangeCreate},
			{Path: "bin/tool", Type: ChangeCreate, Mode: ModeExecutable},
		}, true},
		{"create non-executable", []Change{
			{Path: "a.txt", Type: ChangeCreate, Mode: ModeFile},
		}, false},
		{"update keeps mode", []Change{
			{Path: "bin/tool", Type: ChangeUpdate, Mode: ModeExecutable, CurrentMode: ModeExecutable},
		}, false},
		{"update sets x bit", []Change{
			{Path: "bin/tool", Type: ChangeUpdate, Mode: ModeExecutable, CurrentMode: ModeFile},
		}, true},
		{"update clears x bit", []Change{
			{Path: "bin/tool", Type: ChangeUpdate, Mode: ModeFile, CurrentMode: ModeExecutable},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsGitDataAPI(tt.changes); got != tt.want {
				t.Errorf("needsGitDataAPI = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsHeadConflict_GitDataAPI(t *testing.T) {
	if !isHeadConflict(errString("update ref: Update is not a fast forward (HTTP 422)")) {
		t.Error("a non-fast-forward ref update should be treated as a HEAD conflict")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
