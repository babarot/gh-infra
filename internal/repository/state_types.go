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
	Actions          CurrentActions
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
	Issues      bool
	Projects    bool
	Wiki        bool
	Discussions bool
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
	Deletion              bool
	Creation              bool
	RequiredLinearHistory bool
	RequiredSignatures    bool
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
