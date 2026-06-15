package fileset

import "testing"

func TestNeedsGitDataAPI_ReturnsFalse_WhenNoExecutable(t *testing.T) {
	changes := []Change{
		{Path: "a.txt", Type: ChangeCreate, Executable: false},
		{Path: "b.txt", Type: ChangeUpdate, Executable: false},
		{Path: "c.txt", Type: ChangeDelete, Executable: false},
	}
	if needsGitDataAPI(changes) {
		t.Error("needsGitDataAPI = true, want false when no change has Executable set")
	}
}

func TestNeedsGitDataAPI_ReturnsTrue_WhenAnyExecutable(t *testing.T) {
	changes := []Change{
		{Path: "a.txt", Type: ChangeCreate, Executable: false},
		{Path: "bin/tool", Type: ChangeCreate, Executable: true},
	}
	if !needsGitDataAPI(changes) {
		t.Error("needsGitDataAPI = false, want true when at least one change has Executable set")
	}
}

func TestNeedsGitDataAPI_ReturnsTrue_WhenAllExecutable(t *testing.T) {
	changes := []Change{
		{Path: "bin/a", Type: ChangeCreate, Executable: true},
		{Path: "bin/b", Type: ChangeCreate, Executable: true},
	}
	if !needsGitDataAPI(changes) {
		t.Error("needsGitDataAPI = false, want true when all changes have Executable set")
	}
}

func TestNeedsGitDataAPI_ReturnsFalse_WhenEmpty(t *testing.T) {
	if needsGitDataAPI(nil) {
		t.Error("needsGitDataAPI = true, want false for empty change list")
	}
}
