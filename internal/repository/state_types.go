package repository

// CurrentState represents the current state of a GitHub repository.
type CurrentState struct {
	Owner       string
	Name        string
	IsNew       bool // true if the repository does not exist yet
	Description string
	Archived    bool
	Homepage    string
	Visibility  string
	Topics      []string
	Features    CurrentFeatures

	BranchProtection map[string]*CurrentBranchProtection // pattern → protection
	Secrets          []string                            // names only (values are opaque)
	Variables        map[string]string                   // name → value
}

func (r *CurrentState) FullName() string {
	return r.Owner + "/" + r.Name
}

type CurrentFeatures struct {
	Issues                   bool
	Projects                 bool
	Wiki                     bool
	Discussions              bool
	MergeCommit              bool
	SquashMerge              bool
	RebaseMerge              bool
	AutoDeleteHeadBranches   bool
	MergeCommitTitle         string
	MergeCommitMessage       string
	SquashMergeCommitTitle   string
	SquashMergeCommitMessage string
}

type CurrentBranchProtection struct {
	Pattern                 string
	RequiredReviews         int
	DismissStaleReviews     bool
	RequireCodeOwnerReviews bool
	RequireStatusChecks     *CurrentStatusChecks
	EnforceAdmins           bool
	AllowForcePushes        bool
	AllowDeletions          bool
}

type CurrentStatusChecks struct {
	Strict   bool
	Contexts []string
}
