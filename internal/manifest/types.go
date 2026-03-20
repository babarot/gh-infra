package manifest

// Document represents a single YAML document with kind routing.
type Document struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

// Repository represents a single repository declaration.
type Repository struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   RepositoryMetadata `yaml:"metadata"`
	Spec       RepositorySpec     `yaml:"spec"`
}

type RepositoryMetadata struct {
	Name      string `yaml:"name"`
	Owner     string `yaml:"owner"`
	ManagedBy string `yaml:"managed_by,omitempty"`
}

func (m RepositoryMetadata) FullName() string {
	return m.Owner + "/" + m.Name
}

type RepositorySpec struct {
	Description      *string           `yaml:"description,omitempty"`
	Homepage         *string           `yaml:"homepage,omitempty"`
	Visibility       *string           `yaml:"visibility,omitempty"`
	Topics           []string          `yaml:"topics,omitempty"`
	Features         *Features         `yaml:"features,omitempty"`
	BranchProtection []BranchProtection `yaml:"branch_protection,omitempty"`
	Secrets          []Secret          `yaml:"secrets,omitempty"`
	Variables        []Variable        `yaml:"variables,omitempty"`
}

type Features struct {
	Issues                  *bool   `yaml:"issues,omitempty"`
	Projects                *bool   `yaml:"projects,omitempty"`
	Wiki                    *bool   `yaml:"wiki,omitempty"`
	Discussions             *bool   `yaml:"discussions,omitempty"`
	MergeCommit             *bool   `yaml:"merge_commit,omitempty"`
	SquashMerge             *bool   `yaml:"squash_merge,omitempty"`
	RebaseMerge             *bool   `yaml:"rebase_merge,omitempty"`
	AutoDeleteHeadBranches  *bool   `yaml:"auto_delete_head_branches,omitempty"`
	MergeCommitTitle        *string `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage      *string `yaml:"merge_commit_message,omitempty"`
	SquashMergeCommitTitle  *string `yaml:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage *string `yaml:"squash_merge_commit_message,omitempty"`
}

type BranchProtection struct {
	Pattern                 string        `yaml:"pattern"`
	RequiredReviews         *int          `yaml:"required_reviews,omitempty"`
	DismissStaleReviews     *bool         `yaml:"dismiss_stale_reviews,omitempty"`
	RequireCodeOwnerReviews *bool         `yaml:"require_code_owner_reviews,omitempty"`
	RequireStatusChecks     *StatusChecks `yaml:"require_status_checks,omitempty"`
	EnforceAdmins           *bool         `yaml:"enforce_admins,omitempty"`
	RestrictPushes          *bool         `yaml:"restrict_pushes,omitempty"`
	AllowForcePushes        *bool         `yaml:"allow_force_pushes,omitempty"`
	AllowDeletions          *bool         `yaml:"allow_deletions,omitempty"`
}

type StatusChecks struct {
	Strict   bool     `yaml:"strict"`
	Contexts []string `yaml:"contexts"`
}

type Secret struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type Variable struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// RepositorySet represents multiple repositories with shared defaults.
type RepositorySet struct {
	APIVersion   string               `yaml:"apiVersion"`
	Kind         string               `yaml:"kind"`
	Metadata     RepositorySetMetadata `yaml:"metadata"`
	Defaults     *RepositorySetDefaults `yaml:"defaults,omitempty"`
	Repositories []RepositorySetEntry  `yaml:"repositories"`
}

type RepositorySetMetadata struct {
	Owner string `yaml:"owner"`
}

type RepositorySetDefaults struct {
	Spec RepositorySpec `yaml:"spec"`
}

type RepositorySetEntry struct {
	Name string         `yaml:"name"`
	Spec RepositorySpec `yaml:"spec"`
}

// Helper functions for pointer creation
func StringPtr(s string) *string { return &s }
func BoolPtr(b bool) *bool      { return &b }
func IntPtr(i int) *int          { return &i }
