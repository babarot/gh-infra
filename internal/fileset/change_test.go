package fileset

import "testing"

// ---------------------------------------------------------------------------
// DiffStat
// ---------------------------------------------------------------------------

func TestDiffStat_AddedLines(t *testing.T) {
	added, removed := DiffStat("", "line1\nline2\nline3\n")
	if added != 3 {
		t.Errorf("added = %d, want 3", added)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestDiffStat_RemovedLines(t *testing.T) {
	added, removed := DiffStat("line1\nline2\n", "")
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
}

func TestDiffStat_Mixed(t *testing.T) {
	added, removed := DiffStat("old line\nkept\n", "kept\nnew line\n")
	if added != 1 {
		t.Errorf("added = %d, want 1", added)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
}

func TestDiffStat_Identical(t *testing.T) {
	added, removed := DiffStat("same\n", "same\n")
	if added != 0 || removed != 0 {
		t.Errorf("expected 0/0, got %d/%d", added, removed)
	}
}

func TestDiffStat_BothEmpty(t *testing.T) {
	added, removed := DiffStat("", "")
	if added != 0 || removed != 0 {
		t.Errorf("expected 0/0, got %d/%d", added, removed)
	}
}

// ---------------------------------------------------------------------------
// PlanTargetNames
// ---------------------------------------------------------------------------

func TestPlanTargetNames_All(t *testing.T) {
	fileSets := makeFileSet("owner", "repo", nil)
	tasks := PlanTargetNames(fileSets, "")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Name != "Fetching owner/repo (files)" {
		t.Errorf("task name = %q", tasks[0].Name)
	}
}

func TestPlanTargetNames_Filtered(t *testing.T) {
	fileSets := makeFileSet("owner", "repo", nil)
	tasks := PlanTargetNames(fileSets, "owner/other")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for non-matching filter, got %d", len(tasks))
	}
}

func TestPlanTargetNames_FilterMatch(t *testing.T) {
	fileSets := makeFileSet("owner", "repo", nil)
	tasks := PlanTargetNames(fileSets, "owner/repo")
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for matching filter, got %d", len(tasks))
	}
}
