package manifest

import "fmt"

const (
	// APIVersion is the current API version for all manifest resources.
	APIVersion = "gh-infra/v1"

	// Kind constants for YAML document routing.
	KindRepository    = "Repository"
	KindRepositorySet = "RepositorySet"
	KindFile          = "File"
	KindFileSet       = "FileSet"

	// Visibility values for repository visibility.
	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal"

	// OnDrift values for FileSet drift handling.
	OnDriftWarn      = "warn"
	OnDriftOverwrite = "overwrite"
	OnDriftSkip      = "skip"

	// Via values for FileSet apply behavior.
	ViaPush        = "push"
	ViaPullRequest = "pull_request"

	// Deprecated: use Via* constants instead.
	CommitStrategyPush        = ViaPush
	CommitStrategyPullRequest = ViaPullRequest

	// Reconcile values for FileEntry reconcile behavior.
	ReconcilePatch      = "patch"       // default: add/update only
	ReconcileMirror     = "mirror"      // add/update + delete orphans
	ReconcileCreateOnly = "create_only" // create if missing, never update

	// Deprecated: use Reconcile* constants instead.
	SyncModePatch      = ReconcilePatch
	SyncModeMirror     = ReconcileMirror
	SyncModeCreateOnly = ReconcileCreateOnly

	// Resource type identifiers used in plan changes.
	ResourceRepository       = "Repository"
	ResourceBranchProtection = "BranchProtection"
	ResourceSecret           = "Secret"
	ResourceVariable         = "Variable"
	ResourceRuleset          = "Ruleset"
	ResourceActions          = "Actions"

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

	sourcePath string // manifest file path; set by parser, not serialized
	docIndex   int    // document index within the manifest file; set by parser
	fromSet    bool   // true if expanded from a RepositorySet
}

func (r *Repository) SetSource(path string, docIndex int) { r.sourcePath = path; r.docIndex = docIndex }
func (r *Repository) SourcePath() string                  { return r.sourcePath }
func (r *Repository) DocIndex() int                       { return r.docIndex }
func (r *Repository) SetFromSet(v bool)                   { r.fromSet = v }
func (r *Repository) FromSet() bool                       { return r.fromSet }

type RepositoryMetadata struct {
	Name  string `yaml:"name"  validate:"required"`
	Owner string `yaml:"owner" validate:"required"`
}

func (m RepositoryMetadata) FullName() string {
	return m.Owner + "/" + m.Name
}

type RepositorySpec struct {
	Description      *string            `yaml:"description,omitempty"`
	Homepage         *string            `yaml:"homepage,omitempty"`
	Visibility       *string            `yaml:"visibility,omitempty" validate:"omitempty,oneof=public private internal"`
	Archived         *bool              `yaml:"archived,omitempty"`
	Topics           []string           `yaml:"topics,omitempty"`
	Features         *Features          `yaml:"features,omitempty"`
	MergeStrategy    *MergeStrategy     `yaml:"merge_strategy,omitempty"`
	BranchProtection []BranchProtection `yaml:"branch_protection,omitempty" validate:"unique=pattern"`
	Rulesets         []Ruleset          `yaml:"rulesets,omitempty"          validate:"unique=name"`
	Secrets          []Secret           `yaml:"secrets,omitempty"           validate:"unique=name"`
	Variables        []Variable         `yaml:"variables,omitempty"         validate:"unique=name"`
	Actions          *Actions           `yaml:"actions,omitempty"`
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
	SquashMergeCommitTitle   *string `yaml:"squash_merge_commit_title,omitempty"   validate:"omitempty,oneof=PR_TITLE COMMIT_OR_PR_TITLE"`
	SquashMergeCommitMessage *string `yaml:"squash_merge_commit_message,omitempty" validate:"omitempty,oneof=COMMIT_MESSAGES PR_BODY BLANK"`
	MergeCommitTitle         *string `yaml:"merge_commit_title,omitempty"          validate:"omitempty,oneof=MERGE_MESSAGE PR_TITLE"`
	MergeCommitMessage       *string `yaml:"merge_commit_message,omitempty"        validate:"omitempty,oneof=PR_TITLE PR_BODY BLANK"`
}

// Actions controls GitHub Actions permissions for a repository.
// Enabled is required by the GitHub API in every PUT to /actions/permissions,
// so validation enforces it whenever any other actions field is specified.
type Actions struct {
	Enabled                *bool            `yaml:"enabled,omitempty"`
	AllowedActions         *string          `yaml:"allowed_actions,omitempty"         validate:"omitempty,oneof=all local_only selected"`
	SHAPinningRequired     *bool            `yaml:"sha_pinning_required,omitempty"`
	WorkflowPermissions    *string          `yaml:"workflow_permissions,omitempty"    validate:"omitempty,oneof=read write"`
	CanApprovePullRequests *bool            `yaml:"can_approve_pull_requests,omitempty"`
	SelectedActions        *SelectedActions `yaml:"selected_actions,omitempty"`
	ForkPRApproval         *string          `yaml:"fork_pr_approval,omitempty"        validate:"omitempty,oneof=first_time_contributors_new_to_github first_time_contributors all_external_contributors"`
}

type SelectedActions struct {
	GithubOwnedAllowed *bool    `yaml:"github_owned_allowed,omitempty"`
	VerifiedAllowed    *bool    `yaml:"verified_allowed,omitempty"`
	PatternsAllowed    []string `yaml:"patterns_allowed,omitempty"`
}

type BranchProtection struct {
	Pattern                 string        `yaml:"pattern" validate:"required"`
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
	Name         string               `yaml:"name"                  validate:"required"`
	Target       *string              `yaml:"target,omitempty"      validate:"omitempty,oneof=branch tag"`
	Enforcement  *string              `yaml:"enforcement,omitempty" validate:"omitempty,oneof=active evaluate disabled"`
	BypassActors []RulesetBypassActor `yaml:"bypass_actors,omitempty"`
	Conditions   *RulesetConditions   `yaml:"conditions,omitempty"`
	Rules        RulesetRules         `yaml:"rules"`
}

type RulesetBypassActor struct {
	Role       string `yaml:"role,omitempty" validate:"omitempty,oneof=admin write maintain"`
	Team       string `yaml:"team,omitempty"`        // team slug
	App        string `yaml:"app,omitempty"`         // GitHub App slug
	OrgAdmin   *bool  `yaml:"org-admin,omitempty"`   // true = OrganizationAdmin
	CustomRole string `yaml:"custom-role,omitempty"` // Enterprise Cloud custom role name
	BypassMode string `yaml:"bypass_mode" validate:"oneof=always pull_request exempt"`
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
	Name  string `yaml:"name" validate:"required"`
	Value string `yaml:"value"`
}

type Variable struct {
	Name  string `yaml:"name" validate:"required"`
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

// File represents files to manage in a single repository.
// At parse time, File is expanded into a FileSet with one repository entry.
type File struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   FileMetadata `yaml:"metadata"`
	Spec       FileSpec     `yaml:"spec"`
}

type FileMetadata struct {
	Name  string `yaml:"name"  validate:"required"`
	Owner string `yaml:"owner" validate:"required"`
}

func (m FileMetadata) FullName() string {
	return m.Owner + "/" + m.Name
}

type FileSpec struct {
	Files         []FileEntry `yaml:"files" validate:"required"`
	CommitMessage string      `yaml:"commit_message,omitempty"`
	Via           string      `yaml:"via,omitempty" validate:"omitempty,oneof=push pull_request"`
	Branch        string      `yaml:"branch,omitempty"`
	PRTitle       string      `yaml:"pr_title,omitempty"`
	PRBody        string      `yaml:"pr_body,omitempty"`

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedCommitStrategy string   `yaml:"commit_strategy,omitempty" deprecated:"via:use \"via\" instead"`
	DeprecatedOnApply        string   `yaml:"on_apply,omitempty"        deprecated:"via:use \"via\" instead"`
	DeprecatedOnDrift        string   `yaml:"on_drift,omitempty"        deprecated:":and will be ignored"`
	DeprecationWarnings      []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (s *FileSpec) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileSpec
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*s = FileSpec(r)
	// TODO: remove once commit_strategy and on_apply are fully removed.
	// Both are deprecated aliases for Via; MigrateDeprecated cannot detect
	// this conflict because it processes fields sequentially.
	if s.DeprecatedCommitStrategy != "" && s.DeprecatedOnApply != "" {
		return fmt.Errorf("cannot specify both \"commit_strategy\" and \"on_apply\"")
	}
	warnings, err := MigrateDeprecated(s)
	if err != nil {
		return err
	}
	s.DeprecationWarnings = warnings
	return nil
}

// FileSet represents a set of files to distribute to target repositories.
type FileSet struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   FileSetMetadata `yaml:"metadata"`
	Spec       FileSetSpec     `yaml:"spec"`

	sourcePath string // manifest file path; set by parser, not serialized
	docIndex   int    // document index within the manifest file; set by parser
}

type FileSetMetadata struct {
	Owner string `yaml:"owner" validate:"required"`
}

// SourcePath is the path to the manifest file that defined this FileSet.
// DocIndex is the 0-based document index within that file.
// Both are set by the parser and not serialized to YAML.
func (fs *FileSet) SetSource(path string, docIndex int) { fs.sourcePath = path; fs.docIndex = docIndex }
func (fs *FileSet) SourcePath() string                  { return fs.sourcePath }
func (fs *FileSet) DocIndex() int                       { return fs.docIndex }

type FileSetSpec struct {
	Repositories  []FileSetRepository `yaml:"repositories" validate:"required,unique=Name"`
	Files         []FileEntry         `yaml:"files"        validate:"required"`
	CommitMessage string              `yaml:"commit_message,omitempty"`
	Via           string              `yaml:"via,omitempty" validate:"omitempty,oneof=push pull_request"`
	Branch        string              `yaml:"branch,omitempty"`
	PRTitle       string              `yaml:"pr_title,omitempty"`
	PRBody        string              `yaml:"pr_body,omitempty"`

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedCommitStrategy string   `yaml:"commit_strategy,omitempty" deprecated:"via:use \"via\" instead"`
	DeprecatedOnApply        string   `yaml:"on_apply,omitempty"        deprecated:"via:use \"via\" instead"`
	DeprecatedOnDrift        string   `yaml:"on_drift,omitempty"        deprecated:":and will be ignored"`
	DeprecationWarnings      []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (s *FileSetSpec) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileSetSpec
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*s = FileSetSpec(r)
	// TODO: remove once commit_strategy and on_apply are fully removed.
	// Both are deprecated aliases for Via; MigrateDeprecated cannot detect
	// this conflict because it processes fields sequentially.
	if s.DeprecatedCommitStrategy != "" && s.DeprecatedOnApply != "" {
		return fmt.Errorf("cannot specify both \"commit_strategy\" and \"on_apply\"")
	}
	warnings, err := MigrateDeprecated(s)
	if err != nil {
		return err
	}
	s.DeprecationWarnings = warnings
	return nil
}

// FileSetRepository can be a simple string "repo" or a struct with overrides.
type FileSetRepository struct {
	Name      string      `yaml:"name" validate:"required"`
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
	Path           string            `yaml:"path"                validate:"required"`
	Content        string            `yaml:"content,omitempty" validate:"exclusive=source"`
	Source         string            `yaml:"source,omitempty"`
	Patches        []string          `yaml:"patches,omitempty"`
	Vars           map[string]string `yaml:"vars,omitempty"`
	Reconcile      string            `yaml:"reconcile,omitempty" validate:"omitempty,oneof=patch mirror create_only"`
	DirScope       string            `yaml:"-"`
	OriginalSource string            `yaml:"-"` // absolute path of local source file; preserved by ResolveFiles for pull

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedSyncMode  string   `yaml:"sync_mode,omitempty" deprecated:"reconcile:use \"reconcile\" instead"`
	DeprecatedOnDrift   string   `yaml:"on_drift,omitempty"  deprecated:":and will be ignored"`
	DeprecationWarnings []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (fe *FileEntry) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileEntry
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*fe = FileEntry(r)
	warnings, err := MigrateDeprecated(fe)
	if err != nil {
		if fe.Path != "" {
			return fmt.Errorf("%s: %w", fe.Path, err)
		}
		return err
	}
	// Prefix warnings with path for context
	for i, w := range warnings {
		if fe.Path != "" {
			warnings[i] = fe.Path + ": " + w
		}
	}
	fe.DeprecationWarnings = warnings
	return nil
}

// ParseResult holds all parsed resources from a path.
type ParseResult struct {
	Repositories []*Repository
	FileSets     []*FileSet
	Warnings     []string // deprecation warnings collected during parse
}

// Ptr returns a pointer to the given value.
//
//go:fix inline
func Ptr[T any](v T) *T { return new(v) }
