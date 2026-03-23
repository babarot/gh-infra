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

	// Strategy values for FileSet apply behavior.
	StrategyDirect      = "direct"
	StrategyPullRequest = "pull_request"

	// Resource type identifiers used in plan changes.
	ResourceRepository       = "Repository"
	ResourceBranchProtection = "BranchProtection"
	ResourceSecret           = "Secret"
	ResourceVariable         = "Variable"
	ResourceRuleset          = "Ruleset"

	// Ruleset enforcement values.
	RulesetEnforcementActive   = "active"
	RulesetEnforcementEvaluate = "evaluate"
	RulesetEnforcementDisabled = "disabled"

	// Ruleset target values.
	RulesetTargetBranch = "branch"
	RulesetTargetTag    = "tag"

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
	MergeStrategy    *MergeStrategy     `yaml:"merge_strategy,omitempty"`
	BranchProtection []BranchProtection `yaml:"branch_protection,omitempty"`
	Rulesets         []Ruleset          `yaml:"rulesets,omitempty"`
	Secrets          []Secret           `yaml:"secrets,omitempty"`
	Variables        []Variable         `yaml:"variables,omitempty"`
}

type Features struct {
	Issues      *bool `yaml:"issues,omitempty"`
	Projects    *bool `yaml:"projects,omitempty"`
	Wiki        *bool `yaml:"wiki,omitempty"`
	Discussions *bool `yaml:"discussions,omitempty"`
}

type MergeStrategy struct {
	AllowMergeCommit         *bool   `yaml:"allow_merge_commit,omitempty"`
	AllowSquashMerge         *bool   `yaml:"allow_squash_merge,omitempty"`
	AllowRebaseMerge         *bool   `yaml:"allow_rebase_merge,omitempty"`
	AutoDeleteHeadBranches   *bool   `yaml:"auto_delete_head_branches,omitempty"`
	SquashMergeCommitTitle   *string `yaml:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage *string `yaml:"squash_merge_commit_message,omitempty"`
	MergeCommitTitle         *string `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage       *string `yaml:"merge_commit_message,omitempty"`
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

// Ruleset represents a GitHub repository ruleset.
type Ruleset struct {
	Name         string               `yaml:"name"`
	Target       *string              `yaml:"target,omitempty"`      // "branch" (default) or "tag"
	Enforcement  *string              `yaml:"enforcement,omitempty"` // "active", "evaluate", "disabled"
	BypassActors []RulesetBypassActor `yaml:"bypass_actors,omitempty"`
	Conditions   *RulesetConditions   `yaml:"conditions,omitempty"`
	Rules        RulesetRules         `yaml:"rules"`
}

type RulesetBypassActor struct {
	Role       string `yaml:"role,omitempty"`        // admin, write, maintain
	Team       string `yaml:"team,omitempty"`        // team slug
	App        string `yaml:"app,omitempty"`         // GitHub App slug
	OrgAdmin   *bool  `yaml:"org-admin,omitempty"`   // true = OrganizationAdmin
	CustomRole string `yaml:"custom-role,omitempty"` // Enterprise Cloud custom role name
	BypassMode string `yaml:"bypass_mode"`
}

type RulesetConditions struct {
	RefName *RulesetRefCondition `yaml:"ref_name,omitempty"`
}

type RulesetRefCondition struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude,omitempty"`
}

type RulesetRules struct {
	PullRequest           *RulesetPullRequest  `yaml:"pull_request,omitempty"`
	RequiredStatusChecks  *RulesetStatusChecks `yaml:"required_status_checks,omitempty"`
	NonFastForward        *bool                `yaml:"non_fast_forward,omitempty"`
	Deletion              *bool                `yaml:"deletion,omitempty"`
	Creation              *bool                `yaml:"creation,omitempty"`
	RequiredLinearHistory *bool                `yaml:"required_linear_history,omitempty"`
	RequiredSignatures    *bool                `yaml:"required_signatures,omitempty"`
}

type RulesetPullRequest struct {
	RequiredApprovingReviewCount   *int  `yaml:"required_approving_review_count,omitempty"`
	DismissStaleReviewsOnPush      *bool `yaml:"dismiss_stale_reviews_on_push,omitempty"`
	RequireCodeOwnerReview         *bool `yaml:"require_code_owner_review,omitempty"`
	RequireLastPushApproval        *bool `yaml:"require_last_push_approval,omitempty"`
	RequiredReviewThreadResolution *bool `yaml:"required_review_thread_resolution,omitempty"`
}

type RulesetStatusChecks struct {
	StrictRequiredStatusChecksPolicy *bool                `yaml:"strict_required_status_checks_policy,omitempty"`
	Contexts                         []RulesetStatusCheck `yaml:"contexts"`
}

type RulesetStatusCheck struct {
	Context string `yaml:"context"`
	App     string `yaml:"app,omitempty"` // GitHub App slug (optional)
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
	Owner string `yaml:"owner"`
}

type FileSetSpec struct {
	Repositories  []FileSetRepository `yaml:"repositories"`
	Files         []FileEntry         `yaml:"files"`
	OnDrift       string              `yaml:"on_drift,omitempty"`       // warn (default), overwrite, skip
	CommitMessage string              `yaml:"commit_message,omitempty"` // custom commit message
	Strategy      string              `yaml:"strategy,omitempty"`       // direct (default), pull_request
	Branch        string              `yaml:"branch,omitempty"`         // branch name for pull_request strategy
}

// FileSetRepository can be a simple string "repo" or a struct with overrides.
type FileSetRepository struct {
	Name      string      `yaml:"name"`
	Overrides []FileEntry `yaml:"overrides,omitempty"`
}

// UnmarshalYAML allows FileSetRepository to be either a string or a struct.
func (t *FileSetRepository) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		t.Name = s
		return nil
	}
	type raw FileSetRepository
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*t = FileSetRepository(r)
	return nil
}

// RepoFullName returns the full "owner/repo" name for a repository entry.
func (fs *FileSet) RepoFullName(repoName string) string {
	return fs.Metadata.Owner + "/" + repoName
}

type FileEntry struct {
	Path    string            `yaml:"path"`
	Content string            `yaml:"content,omitempty"`
	Source  string            `yaml:"source,omitempty"` // local file path
	Vars    map[string]string `yaml:"vars,omitempty"`   // template variables
}

// ParseResult holds all parsed resources from a path.
type ParseResult struct {
	Repositories []*Repository
	FileSets     []*FileSet
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T { return &v }
