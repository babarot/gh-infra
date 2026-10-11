package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/ui"
)

// confirmPrinter answers Confirm/ConfirmWithDiff; other methods are unused.
type confirmPrinter struct {
	ui.Printer
	asked bool
	err   error
}

func (p *confirmPrinter) Confirm(string) (bool, error) {
	p.asked = true
	return p.err == nil, p.err
}

func (p *confirmPrinter) ConfirmWithDiff(string, []ui.DiffEntry) (bool, error) {
	p.asked = true
	return p.err == nil, p.err
}

type fakeImportDiff struct {
	entries []ui.DiffEntry
	applied []ui.DiffEntry
	called  bool
}

func (d *fakeImportDiff) DiffEntries() []ui.DiffEntry { return d.entries }
func (d *fakeImportDiff) ApplySelections(e []ui.DiffEntry) {
	d.called = true
	d.applied = e
}

func TestConfirmImport(t *testing.T) {
	files := []ui.DiffEntry{{Path: "a.txt", Action: "patch", DefaultAction: "patch"}}

	for _, tt := range []struct {
		name        string
		entries     []ui.DiffEntry
		autoApprove bool
		confirmErr  error
		wantOK      bool
		wantAsked   bool
		wantApplied bool
		wantErr     string
	}{
		{"auto-approve with file changes", files, true, nil, true, false, true, ""},
		{"auto-approve with repository changes only", nil, true, nil, true, false, true, ""},
		{"asks with file changes", files, false, nil, true, true, true, ""},
		{"asks with repository changes only", nil, false, nil, true, true, false, ""},
		{"no terminal", files, false, errors.New("could not open TTY"), false, true, false, "use --auto-approve"},
		{"no terminal, repository changes only", nil, false, errors.New("could not open TTY"), false, true, false, "use --auto-approve"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := &confirmPrinter{err: tt.confirmErr}
			d := &fakeImportDiff{entries: tt.entries}
			ok, err := confirmImport(p, d, tt.autoApprove)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "could not open TTY") {
					t.Fatalf("err = %v, want it to mention %q and the cause", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if ok != tt.wantOK || p.asked != tt.wantAsked {
				t.Errorf("ok=%v asked=%v, want ok=%v asked=%v", ok, p.asked, tt.wantOK, tt.wantAsked)
			}
			if tt.wantApplied && (!d.called || len(d.applied) != len(tt.entries)) {
				t.Errorf("selections not applied: called=%v applied=%v", d.called, d.applied)
			}
			if tt.confirmErr != nil && d.called {
				t.Error("selections applied although confirmation failed")
			}
		})
	}
}

func TestImportCmd_AutoApproveRequiresInto(t *testing.T) {
	c := newImportCmd()
	c.SetArgs([]string{"org/repo", "--auto-approve"})
	c.SilenceErrors, c.SilenceUsage = true, true
	if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "--auto-approve requires --into") {
		t.Errorf("err = %v", err)
	}
}
