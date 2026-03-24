package cmd

import (
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/ui"
)

func TestBuildDiffEntries(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileCreate, Desired: "new\n", OnDrift: "overwrite"},
		{Path: "b.txt", Type: fileset.FileUpdate, Current: "old\n", Desired: "new\n", OnDrift: "warn"},
		{Path: "c.txt", Type: fileset.FileNoOp},
		{Path: "d.txt", Type: fileset.FileSkip},
		{Path: "e.txt", Type: fileset.FileDelete, Current: "bye\n", OnDrift: "overwrite"},
		{Path: "f.txt", Type: fileset.FileDrift, Current: "x\n", Desired: "y\n", OnDrift: "warn"},
	}

	entries := buildDiffEntries(changes)

	// Should filter out NoOp and Skip
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Check icons
	if entries[0].Icon != ui.IconAdd {
		t.Errorf("a.txt icon = %q, want %q", entries[0].Icon, ui.IconAdd)
	}
	if entries[1].Icon != ui.IconChange {
		t.Errorf("b.txt icon = %q, want %q", entries[1].Icon, ui.IconChange)
	}
	if entries[2].Icon != ui.IconRemove {
		t.Errorf("e.txt icon = %q, want %q", entries[2].Icon, ui.IconRemove)
	}
	if entries[3].Icon != ui.IconWarning {
		t.Errorf("f.txt icon = %q, want %q", entries[3].Icon, ui.IconWarning)
	}

	// Check OnDrift passthrough
	if entries[0].OnDrift != "overwrite" {
		t.Errorf("a.txt OnDrift = %q, want overwrite", entries[0].OnDrift)
	}

	// Check Current/Desired passthrough
	if entries[1].Current != "old\n" || entries[1].Desired != "new\n" {
		t.Errorf("b.txt Current/Desired mismatch")
	}
}

func TestApplyOnDriftOverrides_NoChange(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileDrift, OnDrift: "warn", Drifted: true},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", OnDrift: "warn"}, // no change
	}

	applyOnDriftOverrides(changes, entries)

	if changes[0].Type != fileset.FileDrift {
		t.Errorf("expected FileDrift (unchanged), got %s", changes[0].Type)
	}
}

func TestApplyOnDriftOverrides_WarnToOverwrite(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileDrift, OnDrift: "warn", Drifted: true},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", OnDrift: "overwrite"},
	}

	applyOnDriftOverrides(changes, entries)

	if changes[0].OnDrift != "overwrite" {
		t.Errorf("OnDrift = %q, want overwrite", changes[0].OnDrift)
	}
	if changes[0].Type != fileset.FileUpdate {
		t.Errorf("Type = %s, want FileUpdate", changes[0].Type)
	}
}

func TestApplyOnDriftOverrides_WarnToSkip(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileDrift, OnDrift: "warn", Drifted: true},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", OnDrift: "skip"},
	}

	applyOnDriftOverrides(changes, entries)

	if changes[0].Type != fileset.FileSkip {
		t.Errorf("Type = %s, want FileSkip", changes[0].Type)
	}
}

func TestApplyOnDriftOverrides_OverwriteToWarn(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileUpdate, OnDrift: "overwrite", Drifted: true},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", OnDrift: "warn"},
	}

	applyOnDriftOverrides(changes, entries)

	if changes[0].Type != fileset.FileDrift {
		t.Errorf("Type = %s, want FileDrift", changes[0].Type)
	}
}

func TestApplyOnDriftOverrides_NonDriftedIgnored(t *testing.T) {
	changes := []fileset.FileChange{
		{Path: "a.txt", Type: fileset.FileCreate, OnDrift: "overwrite", Drifted: false},
	}
	entries := []ui.DiffEntry{
		{Path: "a.txt", OnDrift: "skip"},
	}

	applyOnDriftOverrides(changes, entries)

	// OnDrift updated but Type should NOT change (not drifted)
	if changes[0].OnDrift != "skip" {
		t.Errorf("OnDrift = %q, want skip", changes[0].OnDrift)
	}
	if changes[0].Type != fileset.FileCreate {
		t.Errorf("Type = %s, want FileCreate (non-drifted should keep type)", changes[0].Type)
	}
}
