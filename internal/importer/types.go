package importer

import (
	"fmt"
	"maps"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

// Target is a repository to import into.
type Target struct {
	Owner string
	Name  string
}

// FullName returns "owner/name".
func (t Target) FullName() string {
	return t.Owner + "/" + t.Name
}

// Matches holds manifest resources that matched a target repository.
// Repositories and RepositorySets are separated because their YAML write-back
// paths differ ($.spec vs $.repositories[N].spec).
type Matches struct {
	Repositories   []*manifest.RepositoryDocument
	RepositorySets []*manifest.RepositoryDocument
	FileSets       []*manifest.FileDocument
}

// HasRepo reports whether any repository-level matches exist.
func (m Matches) HasRepo() bool { return len(m.Repositories) > 0 || len(m.RepositorySets) > 0 }

// HasFiles reports whether any file-level matches exist.
func (m Matches) HasFiles() bool { return len(m.FileSets) > 0 }

// IsEmpty reports whether no matches were found.
func (m Matches) IsEmpty() bool { return !m.HasRepo() && !m.HasFiles() }

// TargetMatches pairs a Target with its Matches.
type TargetMatches struct {
	Target  Target
	Matches Matches
}

// FieldDiff represents a single field-level difference (import-specific).
type FieldDiff struct {
	Target string // owner/repo
	Field  string // e.g. "description", "features.issues", "rulesets.my-rule.enforcement"
	Old    any    // local value
	New    any    // GitHub value
}

// RepoResult is a per-Repository/RepositorySet change plan.
type RepoResult struct {
	Diffs         []FieldDiff       // field-level diffs (for display)
	ManifestEdits map[string][]byte // YAML patch results (path → updated bytes)
	UpdatedDocs   int
}

// HasChanges reports whether any diffs were detected.
func (rp RepoResult) HasChanges() bool {
	return len(rp.Diffs) > 0
}

// Result is the aggregate change plan across all targets.
type Result struct {
	RepoDiffs     []FieldDiff       // all repo-level diffs
	FileChanges   []Change          // file-level changes
	ManifestEdits map[string][]byte // YAML patch results (path → final bytes)
	UpdatedDocs   int
}

// AddRepoResult merges a RepoPlan into this IntoPlan.
func (p *Result) AddRepoResult(rp RepoResult) {
	p.RepoDiffs = append(p.RepoDiffs, rp.Diffs...)
	maps.Copy(p.ManifestEdits, rp.ManifestEdits)
	p.UpdatedDocs += rp.UpdatedDocs
}

// HasChanges reports whether any changes exist.
func (p Result) HasChanges() bool {
	return len(p.RepoDiffs) > 0 || HasFileChanges(p.FileChanges)
}

// HasFileChanges reports whether any file changes are non-noop.
func HasFileChanges(changes []Change) bool {
	for _, c := range changes {
		if c.Type != fileset.ChangeNoOp {
			return true
		}
	}
	return false
}

// WriteMode controls how a file change is written back.
type WriteMode string

const (
	WriteSource WriteMode = "source" // overwrite local source file
	WriteInline WriteMode = "inline" // AST-edit content: block in YAML
	WritePatch  WriteMode = "patch"  // generate/update patches in manifest YAML
	WriteSkip   WriteMode = "skip"   // skip (not writable)
)

// ImportAction is the user-facing write-back choice in the interactive viewer.
type ImportAction string

const (
	ActionWrite ImportAction = "write"
	ActionPatch ImportAction = "patch"
	ActionSkip  ImportAction = "skip"
)

// Change represents a single file-level import change.
type Change struct {
	Target             string             // owner/repo
	Path               string             // file path in the repository
	Type               fileset.ChangeType // create/update/noop
	Current            string             // default current content for the selected action
	WriteCurrent       string             // current content for write action
	PatchCurrent       string             // current content for patch action
	Desired            string             // GitHub content
	WriteMode          WriteMode          // deprecated compatibility alias of SuggestedWriteMode
	SuggestedWriteMode WriteMode
	AllowedActions     []ImportAction
	SelectedAction     ImportAction
	LocalTarget        string // write-back path (WriteSource)
	ManifestPath       string // manifest path (WriteInline/WritePatch)
	DocIndex           int
	YAMLPath           string              // write action YAML path, e.g. $.spec.files[0].content
	PatchYAMLPath      string              // patch action YAML path, e.g. $.spec.files[0]
	PatchContent       string              // generated patch content (WritePatch only)
	PatchEntry         *manifest.FileEntry // original FileEntry for WritePatch (to reconstruct with patches)
	Reason             string              // skip reason
	Warnings           []string
}

// CurrentForAction returns the current content shown for the given action.
func (c Change) CurrentForAction(action ImportAction) string {
	switch action {
	case ActionPatch:
		return c.PatchCurrent
	case ActionWrite:
		return c.WriteCurrent
	default:
		return c.Current
	}
}

// DisplayPath returns the write-back target path shown to the user for an action.
func (c Change) DisplayPath(action ImportAction) string {
	if action == "" {
		action = DefaultAction(c.SuggestedWriteMode)
		if c.SuggestedWriteMode == "" && c.WriteMode != "" {
			action = DefaultAction(c.WriteMode)
		}
	}
	if action == ActionPatch && c.ManifestPath != "" {
		return c.ManifestPath + ":" + c.Path + " (patches)"
	}
	if c.LocalTarget != "" {
		return c.LocalTarget
	}
	if c.ManifestPath != "" {
		return c.ManifestPath + ":" + c.Path
	}
	return c.Path
}

// HasAction reports whether the action is selectable for this change.
func (c Change) HasAction(action ImportAction) bool {
	if len(c.AllowedActions) == 0 {
		return true
	}
	for _, allowed := range c.AllowedActions {
		if allowed == action {
			return true
		}
	}
	return false
}

// EffectiveWriteMode resolves the selected action to the internal write mode.
func (c Change) EffectiveWriteMode() (WriteMode, error) {
	action := c.SelectedAction
	if action == "" {
		action = DefaultAction(c.SuggestedWriteMode)
		if c.SuggestedWriteMode == "" && c.WriteMode != "" {
			action = DefaultAction(c.WriteMode)
		}
	}
	if !c.HasAction(action) {
		return "", fmt.Errorf("action %q is not allowed for %s", action, c.Path)
	}

	switch action {
	case ActionSkip:
		return WriteSkip, nil
	case ActionPatch:
		if c.PatchYAMLPath == "" && c.YAMLPath != "" {
			return WritePatch, nil
		}
		if c.PatchYAMLPath == "" || c.PatchEntry == nil {
			return "", fmt.Errorf("patch action is not available for %s", c.Path)
		}
		return WritePatch, nil
	case ActionWrite:
		if c.LocalTarget != "" {
			return WriteSource, nil
		}
		if c.YAMLPath != "" {
			return WriteInline, nil
		}
		return "", fmt.Errorf("write action is not available for %s", c.Path)
	default:
		return "", fmt.Errorf("unknown import action %q", action)
	}
}

// UpdateTypeForAction recomputes the effective change type after action selection.
func (c *Change) UpdateTypeForAction() {
	if c.SelectedAction == ActionSkip {
		c.Type = fileset.ChangeNoOp
		return
	}
	if c.CurrentForAction(c.SelectedAction) == "" && c.Desired == "" {
		c.Type = fileset.ChangeNoOp
		return
	}
	if strings.TrimRight(c.CurrentForAction(c.SelectedAction), "\n") == strings.TrimRight(c.Desired, "\n") {
		c.Type = fileset.ChangeNoOp
		return
	}
	c.Type = fileset.ChangeUpdate
}

// DefaultAction returns the default user-facing action for a suggested mode.
func DefaultAction(mode WriteMode) ImportAction {
	switch mode {
	case WritePatch:
		return ActionPatch
	case WriteSkip:
		return ActionSkip
	default:
		return ActionWrite
	}
}
