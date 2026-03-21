package manifest

const (
	// APIVersion is the current API version for all manifest resources.
	APIVersion = "gh-infra/v1"

	// Kind constants for YAML document routing.
	KindRepository    = "Repository"
	KindRepositorySet = "RepositorySet"
	KindFileSet       = "FileSet"

	// Visibility values for repository visibility.
	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal"

	// OnDrift values for FileSet drift handling.
	OnDriftWarn      = "warn"
	OnDriftOverwrite = "overwrite"
	OnDriftSkip      = "skip"

	// ManagedBy values for repository management mode.
	ManagedBySelf = "self"

	// Resource type identifiers used in plan changes.
	ResourceRepository       = "Repository"
	ResourceBranchProtection = "BranchProtection"
	ResourceSecret           = "Secret"
	ResourceVariable         = "Variable"

	// Squash merge commit title options.
	SquashMergeCommitTitlePRTitle         = "PR_TITLE"
	SquashMergeCommitTitleCommitOrPRTitle = "COMMIT_OR_PR_TITLE"

	// Squash merge commit message options.
	SquashMergeCommitMessageCommitMessages = "COMMIT_MESSAGES"
	SquashMergeCommitMessagePRBody         = "PR_BODY"
	SquashMergeCommitMessageBlank          = "BLANK"

	// Merge commit title options.
	MergeCommitTitleMergeMessage = "MERGE_MESSAGE"
	MergeCommitTitlePRTitle      = "PR_TITLE"

	// Merge commit message options.
	MergeCommitMessagePRTitle = "PR_TITLE"
	MergeCommitMessagePRBody  = "PR_BODY"
	MergeCommitMessageBlank   = "BLANK"

	// DefaultMaxRepoList is the maximum number of repos to list in import.
	DefaultMaxRepoList = "1000"
)

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
	Description      *string            `yaml:"description,omitempty"`
	Homepage         *string            `yaml:"homepage,omitempty"`
	Visibility       *string            `yaml:"visibility,omitempty"`
	Archived         *bool              `yaml:"archived,omitempty"`
	Topics           []string           `yaml:"topics,omitempty"`
	Features         *Features          `yaml:"features,omitempty"`
	BranchProtection []BranchProtection `yaml:"branch_protection,omitempty"`
	Secrets          []Secret           `yaml:"secrets,omitempty"`
	Variables        []Variable         `yaml:"variables,omitempty"`
}

type Features struct {
	Issues                   *bool   `yaml:"issues,omitempty"`
	Projects                 *bool   `yaml:"projects,omitempty"`
	Wiki                     *bool   `yaml:"wiki,omitempty"`
	Discussions              *bool   `yaml:"discussions,omitempty"`
	MergeCommit              *bool   `yaml:"merge_commit,omitempty"`
	SquashMerge              *bool   `yaml:"squash_merge,omitempty"`
	RebaseMerge              *bool   `yaml:"rebase_merge,omitempty"`
	AutoDeleteHeadBranches   *bool   `yaml:"auto_delete_head_branches,omitempty"`
	MergeCommitTitle         *string `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage       *string `yaml:"merge_commit_message,omitempty"`
	SquashMergeCommitTitle   *string `yaml:"squash_merge_commit_title,omitempty"`
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
	APIVersion   string                 `yaml:"apiVersion"`
	Kind         string                 `yaml:"kind"`
	Metadata     RepositorySetMetadata  `yaml:"metadata"`
	Defaults     *RepositorySetDefaults `yaml:"defaults,omitempty"`
	Repositories []RepositorySetEntry   `yaml:"repositories"`
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

// FileSet represents a set of files to distribute to target repositories.
type FileSet struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   FileSetMetadata `yaml:"metadata"`
	Spec       FileSetSpec     `yaml:"spec"`
}

type FileSetMetadata struct {
	Name string `yaml:"name"`
}

type FileSetSpec struct {
	Targets []FileSetTarget `yaml:"targets"`
	Files   []FileEntry     `yaml:"files"`
	OnDrift string          `yaml:"on_drift,omitempty"` // warn (default), overwrite, skip
}

// FileSetTarget can be a simple string "owner/repo" or a struct with overrides.
type FileSetTarget struct {
	Name      string      `yaml:"name"`
	Overrides []FileEntry `yaml:"overrides,omitempty"`
}

type FileEntry struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content,omitempty"`
	Source  string `yaml:"source,omitempty"` // local file path
}

// ParseResult holds all parsed resources from a path.
type ParseResult struct {
	Repositories []*Repository
	FileSets     []*FileSet
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T { return &v }
