package fileset

// State represents the current state of a file in a repository.
type State struct {
	Path    string
	Content string
	SHA     string // needed for updates via Contents API
	Exists  bool
}

// Change represents a planned change for a file.
type Change struct {
	FileSetOwner string // org/owner that owns this FileSet
	Target       string // owner/repo
	Path         string
	Type         ChangeType
	Current      string // current content (if exists)
	Desired      string // desired content
	SHA          string // current SHA (for updates)
	Via          string // "push" or "pull_request" (from FileSet spec)
}

type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeUpdate ChangeType = "update"
	ChangeDelete ChangeType = "delete"
	ChangeNoOp   ChangeType = "noop"
)
