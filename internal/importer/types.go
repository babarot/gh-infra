package importer

import (
	"fmt"
	"maps"
	"slices"
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

// Change represents a single file-level import change.
type Change struct {
	Target             string             // owner/repo
	Path               string             // file path in the repository
	Type               fileset.ChangeType // create/update/noop
	Current            string             // current content for the effective write mode
	WriteCurrent       string             // current content for write mode
	PatchCurrent       string             // current content for patch mode
	Desired            string             // GitHub content
	WriteMode          WriteMode          // effective write mode selected for write-back
	SuggestedWriteMode WriteMode          // planner-chosen default write mode
	AvailableModes     []WriteMode        // write modes the importer can support for this change
	LocalTarget        string             // write-back path (WriteSource)
	ManifestPath       string             // manifest path (WriteInline/WritePatch)
	DocIndex           int
	YAMLPath           string              // write action YAML path, e.g. $.spec.files[0].content
	PatchYAMLPath      string              // patch action YAML path, e.g. $.spec.files[0]
	PatchContent       string              // generated patch content (WritePatch only)
	PatchEntry         *manifest.FileEntry // original FileEntry for WritePatch (to reconstruct with patches)
	Reason             string              // skip reason
	Warnings           []string
}

// CurrentForMode returns the current content shown for the given write mode.
func (c Change) CurrentForMode(mode WriteMode) string {
	switch mode {
	case WritePatch:
		if c.PatchCurrent == "" {
			return c.Current
		}
		return c.PatchCurrent
	case WriteSource, WriteInline:
		if c.WriteCurrent == "" {
			return c.Current
		}
		return c.WriteCurrent
	default:
		return c.Current
	}
}

// DisplayPathForMode returns the write-back target path shown to the user for a mode.
func (c Change) DisplayPathForMode(mode WriteMode) string {
	if mode == "" {
		mode = c.EffectiveWriteMode()
	}
	if mode == WritePatch && c.ManifestPath != "" {
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

// SupportsMode reports whether the write mode is selectable for this change.
func (c Change) SupportsMode(mode WriteMode) bool {
	available := c.availableModes()
	if len(available) == 0 {
		return mode == WriteSkip
	}
	return slices.Contains(available, mode)
}

func (c Change) availableModes() []WriteMode {
	if len(c.AvailableModes) > 0 {
		return c.AvailableModes
	}
	if c.WriteMode == WriteSkip || c.SuggestedWriteMode == WriteSkip {
		return nil
	}

	var modes []WriteMode
	switch c.WriteMode {
	case WriteSource, WriteInline, WritePatch:
		modes = append(modes, c.WriteMode)
	}
	switch c.SuggestedWriteMode {
	case WriteSource, WriteInline, WritePatch:
		if !slices.Contains(modes, c.SuggestedWriteMode) {
			modes = append(modes, c.SuggestedWriteMode)
		}
	}
	if c.LocalTarget != "" && !slices.Contains(modes, WriteSource) {
		modes = append(modes, WriteSource)
	}
	if c.YAMLPath != "" && !slices.Contains(modes, WriteInline) {
		modes = append(modes, WriteInline)
	}
	if c.PatchYAMLPath != "" || c.PatchEntry != nil {
		if !slices.Contains(modes, WritePatch) {
			modes = append(modes, WritePatch)
		}
	}
	return modes
}

// EffectiveWriteMode returns the concrete write mode selected for this change.
func (c Change) EffectiveWriteMode() WriteMode {
	if c.WriteMode != "" {
		return c.WriteMode
	}
	if c.SuggestedWriteMode != "" {
		return c.SuggestedWriteMode
	}
	return WriteSkip
}

// ValidateWriteMode reports whether the effective write mode can be used.
func (c Change) ValidateWriteMode() error {
	mode := c.EffectiveWriteMode()
	if mode == WriteSkip {
		return nil
	}
	if !c.SupportsMode(mode) {
		return fmt.Errorf("write mode %q is not allowed for %s", mode, c.Path)
	}
	if mode == WritePatch {
		if c.PatchYAMLPath == "" && c.YAMLPath != "" {
			return nil
		}
		if c.PatchYAMLPath == "" || c.PatchEntry == nil {
			return fmt.Errorf("patch mode is not available for %s", c.Path)
		}
	}
	if mode == WriteSource && c.LocalTarget == "" {
		return fmt.Errorf("source mode is not available for %s", c.Path)
	}
	if mode == WriteInline && c.YAMLPath == "" {
		return fmt.Errorf("inline mode is not available for %s", c.Path)
	}
	return nil
}

// UpdateTypeForMode recomputes the effective change type after selecting a write mode.
func (c *Change) UpdateTypeForMode(mode WriteMode) {
	c.WriteMode = mode
	c.Current = c.CurrentForMode(mode)
	if mode == WriteSkip {
		c.Type = fileset.ChangeNoOp
		return
	}
	if c.CurrentForMode(mode) == "" && c.Desired == "" {
		c.Type = fileset.ChangeNoOp
		return
	}
	if strings.TrimRight(c.CurrentForMode(mode), "\n") == strings.TrimRight(c.Desired, "\n") {
		c.Type = fileset.ChangeNoOp
		return
	}
	c.Type = fileset.ChangeUpdate
}
