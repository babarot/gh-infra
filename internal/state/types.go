package state

// Repository represents the current state of a GitHub repository.
type Repository struct {
	Owner       string
	Name        string
	Description string
	Homepage    string
	Visibility  string
	Topics      []string
	Features    Features

	BranchProtection map[string]*BranchProtection // pattern → protection
	Secrets          []string                      // names only (values are opaque)
	Variables        map[string]string             // name → value
}

func (r *Repository) FullName() string {
	return r.Owner + "/" + r.Name
}

type Features struct {
	Issues                 bool
	Projects               bool
	Wiki                   bool
	Discussions            bool
	MergeCommit            bool
	SquashMerge            bool
	RebaseMerge            bool
	AutoDeleteHeadBranches bool
}

type BranchProtection struct {
	Pattern                 string
	RequiredReviews         int
	DismissStaleReviews     bool
	RequireCodeOwnerReviews bool
	RequireStatusChecks     *StatusChecks
	EnforceAdmins           bool
	AllowForcePushes        bool
	AllowDeletions          bool
}

type StatusChecks struct {
	Strict   bool
	Contexts []string
}
