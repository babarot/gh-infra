package infra

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// Output formats for plan and apply. Markdown is for plan only.
const (
	OutputText     = "text"
	OutputJSON     = "json"
	OutputMarkdown = "markdown"
)

// ValidateOutput returns an error if format is not one of the given formats.
// An empty format means text.
func ValidateOutput(format string, formats ...string) error {
	if format == "" || format == OutputText || slices.Contains(formats, format) {
		return nil
	}
	return fmt.Errorf("invalid output format %q (use %s)", format, strings.Join(append([]string{OutputText}, formats...), ", "))
}

// isMachineOutput reports whether format prints only a document, so nothing
// else may be printed while planning or applying.
func isMachineOutput(format string) bool {
	return format == OutputJSON || format == OutputMarkdown
}

// documentFormatVersion is bumped only when a field is removed or changes
// meaning; adding fields keeps it.
const documentFormatVersion = 1

// Document is the machine-readable result of plan or apply.
type Document struct {
	FormatVersion int           `json:"format_version"`
	Command       string        `json:"command"`
	Summary       Summary       `json:"summary"`
	Changes       []ChangeEntry `json:"changes"`
	Warnings      []Message     `json:"warnings"`
	Errors        []Message     `json:"errors"`
}

// Summary counts the entries in Document.Changes. Applied, Failed and
// Skipped are set only by apply.
type Summary struct {
	Create  int  `json:"create"`
	Update  int  `json:"update"`
	Delete  int  `json:"delete"`
	Applied *int `json:"applied,omitempty"`
	Failed  *int `json:"failed,omitempty"`
	Skipped *int `json:"skipped,omitempty"`
}

// ChangeEntry is one planned change: a repository setting, a grouped
// setting with its details, a named resource (ruleset, label, ...), or a
// file. Before and After are the values plan shows, not raw API payloads;
// secret values never appear.
type ChangeEntry struct {
	Target   string   `json:"target"`
	Resource string   `json:"resource"`
	Action   string   `json:"action"`
	Name     string   `json:"name,omitempty"`
	Field    string   `json:"field,omitempty"`
	Path     string   `json:"path,omitempty"`
	Before   any      `json:"before,omitempty"`
	After    any      `json:"after,omitempty"`
	Details  []Detail `json:"details,omitempty"`

	// File changes only.
	FileSet string `json:"fileset,omitempty"`
	Via     string `json:"via,omitempty"`
	Lines   *Lines `json:"lines,omitempty"`
	Mode    *Mode  `json:"mode,omitempty"`
	Diff    string `json:"diff,omitempty"`

	// Apply only.
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	PRURL  string `json:"pr_url,omitempty"`
}

// Detail is a child of a grouped change.
type Detail struct {
	Field   string   `json:"field"`
	Action  string   `json:"action"`
	Before  any      `json:"before,omitempty"`
	After   any      `json:"after,omitempty"`
	Details []Detail `json:"details,omitempty"`
}

// Lines is the line count of a file diff.
type Lines struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// Mode is a file mode change.
type Mode struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after"`
}

// Message is a warning or an error, with the repository it is about if any.
type Message struct {
	Target  string `json:"target,omitempty"`
	Message string `json:"message"`
}

// Apply statuses.
const (
	statusApplied = "applied"
	statusFailed  = "failed"
	statusSkipped = "skipped"
)

// fileResource is the resource name of file changes in a Document.
const fileResource = "File"

// unsignedCommitNote is shown when a mode change needs an unsigned commit.
const unsignedCommitNote = "Changing file modes needs the Git Data API, so this commit will not be signed"

// Write encodes the document as indented JSON.
func (d Document) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// PlanDocument builds the plan document from a plan result.
func (r *PlanResult) PlanDocument(showDiff bool) Document {
	doc := Document{
		FormatVersion: documentFormatVersion,
		Command:       "plan",
		Changes:       []ChangeEntry{},
		Warnings:      []Message{},
		Errors:        []Message{},
	}

	for _, w := range r.Warnings {
		doc.Warnings = append(doc.Warnings, Message{Message: w})
	}
	for _, e := range r.TargetErrors {
		doc.Errors = append(doc.Errors, Message{Target: e.Name, Message: e.Err.Error()})
	}

	entries := r.planEntries(showDiff)
	unsigned := make(map[string]bool)
	for _, e := range entries {
		doc.Changes = append(doc.Changes, e.entry)
		if e.file != nil && e.file.ModeChanged() && !unsigned[e.file.Target] {
			unsigned[e.file.Target] = true
			doc.Warnings = append(doc.Warnings, Message{Target: e.file.Target, Message: unsignedCommitNote})
		}
	}

	for _, c := range doc.Changes {
		switch c.Action {
		case string(repository.ChangeCreate):
			doc.Summary.Create++
		case string(repository.ChangeUpdate):
			doc.Summary.Update++
		case string(repository.ChangeDelete):
			doc.Summary.Delete++
		}
	}
	return doc
}

// ApplyDocument builds the apply document: the plan document with the result
// of each change.
func (r *PlanResult) ApplyDocument(applied *ApplyOutcome) Document {
	doc := r.PlanDocument(false)
	doc.Command = "apply"

	if applied != nil {
		repoResults := applied.RepoResults
		fileResults := applied.FileResults
		for i, e := range r.planEntries(false) {
			c := &doc.Changes[i]
			switch {
			case e.file != nil:
				j := findFileResult(fileResults, e.file.Target, e.file.Path)
				if j < 0 {
					continue
				}
				res := fileResults[j]
				fileResults = append(fileResults[:j:j], fileResults[j+1:]...)
				c.PRURL = res.PRURL
				setStatus(c, res.Err, res.Skipped)
			case e.repo != nil:
				j := findRepoResult(repoResults, *e.repo)
				if j < 0 {
					continue
				}
				res := repoResults[j]
				repoResults = append(repoResults[:j:j], repoResults[j+1:]...)
				setStatus(c, res.Err, false)
			}
		}
	}

	var nApplied, nFailed, nSkipped int
	for _, c := range doc.Changes {
		switch c.Status {
		case statusApplied:
			nApplied++
		case statusFailed:
			nFailed++
		case statusSkipped:
			nSkipped++
		}
	}
	doc.Summary.Applied = &nApplied
	doc.Summary.Failed = &nFailed
	doc.Summary.Skipped = &nSkipped
	return doc
}

// planEntry is a document entry with the change it was built from.
type planEntry struct {
	entry ChangeEntry
	repo  *repository.Change
	file  *fileset.Change
}

// planEntries returns the document entries in the order plan prints them.
func (r *PlanResult) planEntries(showDiff bool) []planEntry {
	var out []planEntry
	forEachTarget(r.RepoChanges, r.FileChanges, func(_ string, repoChanges []repository.Change, fileChanges []fileset.Change) {
		for _, c := range repoChanges {
			out = append(out, planEntry{entry: repoChangeEntry(c), repo: &c})
		}
		for _, c := range fileChanges {
			out = append(out, planEntry{entry: fileChangeEntry(c, showDiff), file: &c})
		}
	})
	return out
}

func setStatus(c *ChangeEntry, err error, skipped bool) {
	switch {
	case err != nil:
		c.Status = statusFailed
		c.Error = err.Error()
	case skipped:
		c.Status = statusSkipped
	default:
		c.Status = statusApplied
	}
}

func findRepoResult(results []repository.ApplyResult, c repository.Change) int {
	for i, res := range results {
		rc := res.Change
		if rc.Name == c.Name && rc.Resource == c.Resource && rc.Field == c.Field && rc.Type == c.Type {
			return i
		}
	}
	return -1
}

func findFileResult(results []fileset.ApplyResult, target, path string) int {
	for i, res := range results {
		if res.Change.Target == target && res.Change.Path == path {
			return i
		}
	}
	return -1
}

func repoChangeEntry(c repository.Change) ChangeEntry {
	e := ChangeEntry{
		Target:   c.Name,
		Resource: c.Resource,
		Action:   string(c.Type),
		Field:    c.Field,
		Before:   c.OldValue,
		After:    c.NewValue,
		Details:  details(c.Details),
	}
	// Named resources carry their name in the resource ("Ruleset[main]") or
	// in the field ("Label" with field "bug"); the document has it in name.
	if kind, name, ok := strings.Cut(c.Resource, "["); ok && strings.HasSuffix(name, "]") {
		e.Resource = kind
		e.Name = strings.TrimSuffix(name, "]")
		e.Field = ""
	} else {
		switch c.Resource {
		case manifest.ResourceLabel, manifest.ResourceMilestone, manifest.ResourceSecret, manifest.ResourceVariable:
			e.Name = c.Field
			e.Field = ""
		}
	}
	return e
}

func details(changes []repository.Change) []Detail {
	var out []Detail
	for _, c := range changes {
		if c.Type == repository.ChangeNoOp {
			continue
		}
		out = append(out, Detail{
			Field:   c.Field,
			Action:  string(c.Type),
			Before:  c.OldValue,
			After:   c.NewValue,
			Details: details(c.Details),
		})
	}
	return out
}

func fileChangeEntry(c fileset.Change, showDiff bool) ChangeEntry {
	added, removed := fileset.DiffStat(c.Current, c.Desired)
	e := ChangeEntry{
		Target:   c.Target,
		Resource: fileResource,
		Action:   string(c.Type),
		Path:     c.Path,
		FileSet:  c.FileSetID,
		Via:      c.Via,
		Lines:    &Lines{Added: added, Removed: removed},
	}
	if c.ModeChanged() {
		e.Mode = &Mode{Before: c.CurrentMode, After: c.Mode}
	}
	if showDiff {
		e.Diff = fileDiff(c)
	}
	return e
}

// forEachTarget calls fn for each repository that has changes, in the order
// plan prints them: by first appearance, repository changes before file
// changes. No-op changes are dropped.
func forEachTarget(repoChanges []repository.Change, fileChanges []fileset.Change, fn func(target string, repoChanges []repository.Change, fileChanges []fileset.Change)) {
	var order []string
	seen := make(map[string]bool)
	repoBy := make(map[string][]repository.Change)
	fileBy := make(map[string][]fileset.Change)
	for _, c := range repoChanges {
		if c.Type == repository.ChangeNoOp {
			continue
		}
		if !seen[c.Name] {
			seen[c.Name] = true
			order = append(order, c.Name)
		}
		repoBy[c.Name] = append(repoBy[c.Name], c)
	}
	for _, c := range fileChanges {
		if c.Type == fileset.ChangeNoOp {
			continue
		}
		if !seen[c.Target] {
			seen[c.Target] = true
			order = append(order, c.Target)
		}
		fileBy[c.Target] = append(fileBy[c.Target], c)
	}
	for _, target := range order {
		fn(target, repoBy[target], fileBy[target])
	}
}

// discardPrinter returns a printer that prints nothing, for machine-readable
// output where stdout must carry only the document.
func discardPrinter() *ui.StandardPrinter {
	return ui.NewStandardPrinterWith(io.Discard, io.Discard)
}
