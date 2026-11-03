package repository

import "time"

// LabelUsage holds usage statistics for a label (for mirror mode display).
type LabelUsage struct {
	Count    int
	LastUsed time.Time
}

// CurrentState represents the current state of a GitHub repository.
type CurrentState struct {
	Owner               string
	Name                string
	IsNew               bool // true if the repository does not exist yet
	Description         string
	Archived            bool
	Homepage            string
	Visibility          string
	Topics              []string
	Features            CurrentFeatures
	MergeStrategy       CurrentMergeStrategy
	ReleaseImmutability bool
	Security            CurrentSecurity

	BranchProtection map[string]*CurrentBranchProtection // pattern → protection
	Rulesets         map[string]*CurrentRuleset          // name → ruleset
	Secrets          []string                            // names only (values are opaque)
	Variables        map[string]string                   // name → value
	Labels           map[string]*CurrentLabel            // name → label
	Milestones       map[string]*CurrentMilestone        // title → milestone
	Environments     map[string]*CurrentEnvironment      // lowercase name → environment; nil when not read
	Actions          CurrentActions
	Pages            CurrentPages

	// Unread is what import (a fetch without a spec) could not read, by
	// its manifest key: "labels", "actions", or "environments.<name>.variables"
	// for a part of an environment. Such a setting is left unset (a nil
	// collection), not read as none, so import does not write it and
	// import --into keeps the manifest's. With a spec, a setting that cannot
	// be read is an error instead, and Unread is nil.
	Unread map[string]error
}

// CurrentEnvironment is a deployment environment as GitHub reports it.
type CurrentEnvironment struct {
	Name            string // as GitHub spells it
	CanAdminsBypass bool
	WaitTimer       int // 0 when there is no wait timer rule
	// HasReviewersRule: GitHub keeps prevent_self_review in the required
	// reviewers rule, so without the rule it cannot be read.
	HasReviewersRule  bool
	PreventSelfReview bool
	Reviewers         []CurrentEnvironmentReviewer
	BranchPolicy      string // manifest.DeploymentBranches*
	// BranchRules are the name patterns of a custom policy. They are read
	// only when the manifest sets deployment_branch_policy (or on import).
	BranchRules []CurrentBranchRule
	// Secrets (names only) and Variables are read only when the manifest
	// declares some for the environment (or on import).
	Secrets   []string
	Variables map[string]string
}

type CurrentEnvironmentReviewer struct {
	Type string // User or Team
	ID   int
	Name string // user login or team slug
}

type CurrentBranchRule struct {
	ID   int
	Name string
	Type string // branch or tag
}

// CurrentPages is the GitHub Pages site as GitHub reports it.
type CurrentPages struct {
	Enabled   bool // false when GitHub has no site (404)
	BuildType string
	// Source is reported for a workflow site too: the last one it had.
	Source CurrentPagesSource
	CNAME  string // "" when there is none
	// HTTPSEnforced and CertificateState: the state of the site's
	// certificate ("approved" once it is issued), "" when GitHub reports none.
	HTTPSEnforced    bool
	CertificateState string
	// SourceBranchExists is read only when the manifest builds the site
	// from a branch (legacy): whether its source branch exists.
	SourceBranchExists bool
}

type CurrentPagesSource struct {
	Branch string
	Path   string
}

func (r *CurrentState) FullName() string {
	return r.Owner + "/" + r.Name
}

type CurrentSecurity struct {
	VulnerabilityAlerts           bool
	AutomatedSecurityFixes        bool
	PrivateVulnerabilityReporting bool
}

type CurrentFeatures struct {
	Issues              bool
	Projects            bool
	Wiki                bool
	Discussions         bool
	PullRequests        bool
	PullRequestCreation string // "all" or "collaborators_only"
}

type CurrentMergeStrategy struct {
	AllowMergeCommit         bool
	AllowSquashMerge         bool
	AllowRebaseMerge         bool
	AllowAutoMerge           bool
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
	RestrictPushes          bool // push restrictions are configured
	AllowForcePushes        bool
	AllowDeletions          bool
}

type CurrentStatusChecks struct {
	Strict   bool
	Contexts []string
}

// CurrentRuleset represents the current state of a GitHub repository ruleset.
type CurrentRuleset struct {
	ID           int
	Name         string
	Target       string
	Enforcement  string
	BypassActors []CurrentRulesetBypassActor
	Conditions   *CurrentRulesetConditions
	Rules        CurrentRulesetRules
}

type CurrentRulesetBypassActor struct {
	ActorID    int
	ActorType  string
	BypassMode string
}

type CurrentRulesetConditions struct {
	RefName *CurrentRulesetRefCondition
}

type CurrentRulesetRefCondition struct {
	Include []string
	Exclude []string
}

type CurrentRulesetRules struct {
	PullRequest           *CurrentRulesetPullRequest
	RequiredStatusChecks  *CurrentRulesetStatusChecks
	NonFastForward        bool
	Update                *CurrentRulesetUpdate
	Deletion              bool
	Creation              bool
	RequiredLinearHistory bool
	RequiredSignatures    bool
}

type CurrentRulesetUpdate struct {
	AllowsFetchAndMerge *bool
}

type CurrentRulesetPullRequest struct {
	RequiredApprovingReviewCount   int
	DismissStaleReviewsOnPush      bool
	RequireCodeOwnerReview         bool
	RequireLastPushApproval        bool
	RequiredReviewThreadResolution bool
}

type CurrentRulesetStatusChecks struct {
	StrictRequiredStatusChecksPolicy bool
	Contexts                         []CurrentRulesetStatusCheck
}

type CurrentRulesetStatusCheck struct {
	Context       string
	IntegrationID int
}

type CurrentLabel struct {
	Name        string
	Description string
	Color       string
}

type CurrentMilestone struct {
	Number      int
	Title       string
	Description string
	State       string
	DueOn       string // normalized to YYYY-MM-DD or empty
}

type CurrentActions struct {
	Enabled                bool
	AllowedActions         string
	SHAPinningRequired     bool
	WorkflowPermissions    string
	CanApprovePullRequests bool
	SelectedActions        *CurrentSelectedActions // nil when allowed_actions != "selected"
	ForkPRApproval         string                  // empty on user-owned repos
}

type CurrentSelectedActions struct {
	GithubOwnedAllowed bool
	VerifiedAllowed    bool
	PatternsAllowed    []string
}
