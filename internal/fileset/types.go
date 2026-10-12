package fileset

import "github.com/babarot/gh-infra/internal/manifest"

// State represents the current state of a file in a repository.
type State struct {
	Path    string
	Content string
	SHA     string // needed for updates via Contents API
	Exists  bool
}

// Change represents a planned change for a file.
type Change struct {
	// FileSet is the FileSet that planned this change. Apply matches changes
	// on it rather than FileSetID, which distinct FileSets can share.
	FileSet *manifest.FileSet
	// FileSetID is the FileSet's Identity(): it names the FileSet in plan
	// output and JSON and derives the default commit message, PR branch, and
	// PR body. It is not unique across FileSets.
	FileSetID   string
	Target      string // owner/repo
	Path        string
	Type        ChangeType
	Current     string // current content (if exists)
	Desired     string // desired content
	SHA         string // current SHA (for updates)
	Mode        string // desired file mode from `executable`; empty leaves the mode as is
	CurrentMode string // file mode on GitHub; set only when Mode is set and the file exists
	Via         string // "push" or "pull_request"; push for a pull_request FileSet when NoCommits
	// NoCommits is set when the target repository has no commits, so a
	// pull_request FileSet is pushed to it (see PushToEmpty).
	NoCommits bool
}

// NoCommitsNote explains why a pull_request FileSet is pushed to a repository.
const NoCommitsNote = "pushed to the default branch: the repository has no commits to open a pull request from"

// PushToEmpty plans the changes of pull_request FileSets for target as a
// push. A repository with no commits has no branch to open a pull request
// from, so apply pushes the files to it, and the plan has to say so.
func PushToEmpty(changes []Change, target string) {
	for i := range changes {
		c := &changes[i]
		if c.Target == target && c.Via == manifest.ViaPullRequest {
			c.Via = manifest.ViaPush
			c.NoCommits = true
		}
	}
}

// Git file modes that a FileSet can set.
const (
	ModeFile       = "100644"
	ModeExecutable = "100755"
)

// ModeChanged reports whether applying the change alters the file mode.
// createCommitOnBranch creates files as 100644 and keeps the mode of existing
// files, so only a change away from that needs the Git Data API.
func (c Change) ModeChanged() bool {
	if c.Mode == "" || c.Type == ChangeDelete || c.Type == ChangeNoOp {
		return false
	}
	current := c.CurrentMode
	if current == "" {
		current = ModeFile
	}
	return c.Mode != current
}

type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeUpdate ChangeType = "update"
	ChangeDelete ChangeType = "delete"
	ChangeNoOp   ChangeType = "noop"
)
