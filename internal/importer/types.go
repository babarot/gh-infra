package importer

import (
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

// RepoPlan is a per-Repository/RepositorySet change plan.
type RepoPlan struct {
	Diffs         []FieldDiff       // field-level diffs (for display)
	ManifestEdits map[string][]byte // YAML patch results (path → updated bytes)
	UpdatedDocs   int
}

// HasChanges reports whether any diffs were detected.
func (rp RepoPlan) HasChanges() bool {
	return len(rp.Diffs) > 0
}

// IntoPlan is the aggregate change plan across all targets.
type IntoPlan struct {
	RepoDiffs     []FieldDiff       // all repo-level diffs
	FileChanges   []Change          // file-level changes
	ManifestEdits map[string][]byte // YAML patch results (path → final bytes)
	UpdatedDocs   int
}

// AddRepoPlan merges a RepoPlan into this IntoPlan.
func (p *IntoPlan) AddRepoPlan(rp RepoPlan) {
	p.RepoDiffs = append(p.RepoDiffs, rp.Diffs...)
	for path, data := range rp.ManifestEdits {
		p.ManifestEdits[path] = data
	}
	p.UpdatedDocs += rp.UpdatedDocs
}

// HasChanges reports whether any changes exist.
func (p IntoPlan) HasChanges() bool {
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
	WriteSkip   WriteMode = "skip"   // skip (not writable)
)

// Change represents a single file-level import change.
type Change struct {
	Target       string             // owner/repo
	Path         string             // file path in the repository
	Type         fileset.ChangeType // create/update/noop
	Current      string             // local content
	Desired      string             // GitHub content
	WriteMode    WriteMode
	LocalTarget  string // write-back path (WriteSource)
	ManifestPath string // manifest path (WriteInline)
	DocIndex     int
	YAMLPath     string // e.g. $.spec.files[0].content
	Reason       string // skip reason
	Warnings     []string
}
