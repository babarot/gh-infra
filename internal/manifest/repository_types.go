package manifest

const (
	// Visibility values for repository visibility.
	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal"

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

// Repository represents a single repository declaration.
type Repository struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   RepositoryMetadata `yaml:"metadata"`
	Spec       RepositorySpec     `yaml:"spec"`
}

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
