package manifest

import "fmt"

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
	ResourceLabel            = "Label"
	ResourceMilestone        = "Milestone"
	ResourceEnvironment      = "Environment"

	// Label sync mode values.
	LabelSyncAdditive = "additive"
	LabelSyncMirror   = "mirror"

	// Collection reconcile mode values.
	CollectionReconcileAdditive      = "additive"
	CollectionReconcileAuthoritative = "authoritative"

	// Deployment branch policy types and modes.
	DeploymentBranchTypeBranch  = "branch"
	DeploymentBranchTypeTag     = "tag"
	DeploymentBranchesAll       = "all"
	DeploymentBranchesProtected = "protected"
	DeploymentBranchesCustom    = "custom"

	// EnvironmentReviewersMax is how many reviewers GitHub allows on an
	// environment; EnvironmentWaitTimerMax is its longest wait timer, in
	// minutes.
	EnvironmentReviewersMax = 6
	EnvironmentWaitTimerMax = 43200

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

	// Pull request creation policy values.
	PullRequestCreationAll               = "all"
	PullRequestCreationCollaboratorsOnly = "collaborators_only"

	// DefaultMaxRepoList is the maximum number of repos to list in import.
	DefaultMaxRepoList = "1000"
)

// Repository represents a single repository declaration.
type Repository struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   RepositoryMetadata   `yaml:"metadata"`
	Reconcile  *RepositoryReconcile `yaml:"reconcile,omitempty"`
	Spec       RepositorySpec       `yaml:"spec"`
}

type RepositoryMetadata struct {
	Name  string `yaml:"name"  validate:"required"`
	Owner string `yaml:"owner" validate:"required"`
}

func (m RepositoryMetadata) FullName() string {
	return m.Owner + "/" + m.Name
}

type RepositorySpec struct {
	Description         *string            `yaml:"description,omitempty"`
	Homepage            *string            `yaml:"homepage,omitempty"`
	Visibility          *string            `yaml:"visibility,omitempty" validate:"omitempty,oneof=public private internal"`
	Archived            *bool              `yaml:"archived,omitempty"`
	Topics              []string           `yaml:"topics,omitempty"`
	Labels              []Label            `yaml:"labels,omitempty"            validate:"unique=name"`
	LabelSync           *string            `yaml:"label_sync,omitempty"        validate:"omitempty,oneof=additive mirror"`
	Milestones          []Milestone        `yaml:"milestones,omitempty"        validate:"unique=title"`
	Features            *Features          `yaml:"features,omitempty"`
	MergeStrategy       *MergeStrategy     `yaml:"merge_strategy,omitempty"`
	ReleaseImmutability *bool              `yaml:"release_immutability,omitempty"`
	Security            *Security          `yaml:"security,omitempty"`
	BranchProtection    []BranchProtection `yaml:"branch_protection,omitempty" validate:"unique=pattern"`
	Rulesets            []Ruleset          `yaml:"rulesets,omitempty"          validate:"unique=name"`
	Secrets             []Secret           `yaml:"secrets,omitempty"           validate:"unique=name"`
	Variables           []Variable         `yaml:"variables,omitempty"         validate:"unique=name"`
	Actions             *Actions           `yaml:"actions,omitempty"`
	Environments        []Environment      `yaml:"environments,omitempty"`

	BranchProtectionSet bool `yaml:"-"`
	EnvironmentsSet     bool `yaml:"-"`
	LabelsSet           bool `yaml:"-"`
	MilestonesSet       bool `yaml:"-"`
	RulesetsSet         bool `yaml:"-"`
	SecretsSet          bool `yaml:"-"`
	TopicsSet           bool `yaml:"-"`
	VariablesSet        bool `yaml:"-"`
}

type RepositoryReconcile struct {
	BranchProtection *string `yaml:"branch_protection,omitempty" validate:"omitempty,oneof=additive authoritative"`
	Labels           *string `yaml:"labels,omitempty"            validate:"omitempty,oneof=additive authoritative"`
	Rulesets         *string `yaml:"rulesets,omitempty"           validate:"omitempty,oneof=additive authoritative"`
	// DeploymentBranchPolicies applies to the name patterns of each declared
	// environment that sets custom_branch_policies; spec has no collection of
	// this name.
	DeploymentBranchPolicies *string `yaml:"deployment_branch_policies,omitempty" validate:"omitempty,oneof=additive authoritative"`
}

// UnmarshalYAML tracks whether collection fields were present in YAML. For
// stateless reconciliation, omitted, empty, and non-empty collections have
// different meanings; explicit null remains invalid.
func (s *RepositorySpec) UnmarshalYAML(unmarshal func(any) error) error {
	type raw RepositorySpec
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*s = RepositorySpec(r)

	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}

	if v, ok := fields["topics"]; ok {
		s.TopicsSet = true
		if v == nil {
			return fmt.Errorf("topics must be a sequence; use [] to remove all topics")
		}
	}
	if v, ok := fields["branch_protection"]; ok {
		s.BranchProtectionSet = true
		if v == nil {
			return fmt.Errorf("branch_protection must be a sequence; use [] with reconcile.branch_protection=authoritative to delete all branch protection rules")
		}
	}
	if v, ok := fields["labels"]; ok {
		s.LabelsSet = true
		if v == nil {
			return fmt.Errorf("labels must be a sequence; use [] with reconcile.labels=authoritative to delete all labels")
		}
	}
	if v, ok := fields["milestones"]; ok {
		s.MilestonesSet = true
		if v == nil {
			return fmt.Errorf("milestones must be a sequence; use [] to inherit no milestones from defaults")
		}
	}
	if v, ok := fields["rulesets"]; ok {
		s.RulesetsSet = true
		if v == nil {
			return fmt.Errorf("rulesets must be a sequence; use [] with reconcile.rulesets=authoritative to delete all rulesets")
		}
	}
	if v, ok := fields["secrets"]; ok {
		s.SecretsSet = true
		if v == nil {
			return fmt.Errorf("secrets must be a sequence; use [] to inherit no secrets from defaults")
		}
	}
	if v, ok := fields["variables"]; ok {
		s.VariablesSet = true
		if v == nil {
			return fmt.Errorf("variables must be a sequence; use [] to inherit no variables from defaults")
		}
	}
	if v, ok := fields["environments"]; ok {
		s.EnvironmentsSet = true
		if v == nil {
			return fmt.Errorf("environments must be a sequence; use [] to inherit no environments from defaults")
		}
	}

	return nil
}

// Security groups GitHub Advanced Security related repository settings,
// mirroring the "Advanced Security" section of the GitHub repository
// settings UI.
type Security struct {
	VulnerabilityAlerts           *bool `yaml:"vulnerability_alerts,omitempty"`
	AutomatedSecurityFixes        *bool `yaml:"automated_security_fixes,omitempty"`
	PrivateVulnerabilityReporting *bool `yaml:"private_vulnerability_reporting,omitempty"`
}

type Features struct {
	Issues       *bool         `yaml:"issues,omitempty"`
	Projects     *bool         `yaml:"projects,omitempty"`
	Wiki         *bool         `yaml:"wiki,omitempty"`
	Discussions  *bool         `yaml:"discussions,omitempty"`
	PullRequests *PullRequests `yaml:"pull_requests,omitempty"`
}

// PullRequests controls the pull request feature and creation policy.
// Supports both bool form (pull_requests: true/false) and object form
// (pull_requests: {creation: collaborators_only}).
//
// In bool form, true/false maps directly to has_pull_requests on the API.
// In object form, pull requests are implicitly enabled and creation controls
// the pull_request_creation_policy API field.
//
// Enabled is an internal field — it is not exposed in YAML.
type PullRequests struct {
	Enabled  *bool   `yaml:"-"` // internal; set by UnmarshalYAML
	Creation *string `yaml:"creation,omitempty" validate:"omitempty,oneof=all collaborators_only"`
}

// UnmarshalYAML allows PullRequests to be either a bool or a struct.
// Bool form sets Enabled directly. Object form implies Enabled=true.
func (pr *PullRequests) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		pr.Enabled = &b
		return nil
	}
	type raw PullRequests
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*pr = PullRequests(r)
	// Object form implies enabled=true.
	t := true
	pr.Enabled = &t
	return nil
}

// MarshalYAML emits a bare bool when Enabled is false or when no Creation
// is set (simple enable). Only emits the object form when Creation is set.
func (pr PullRequests) MarshalYAML() (any, error) {
	if pr.Enabled != nil && !*pr.Enabled {
		return false, nil
	}
	if pr.Creation == nil {
		return true, nil
	}
	// Emit object form with only the creation field.
	return struct {
		Creation string `yaml:"creation"`
	}{Creation: *pr.Creation}, nil
}

// IsEnabled returns the Enabled pointer, or nil if PullRequests is nil.
func (pr *PullRequests) IsEnabled() *bool {
	if pr == nil {
		return nil
	}
	return pr.Enabled
}

// GetCreation returns the Creation pointer, or nil if PullRequests is nil.
func (pr *PullRequests) GetCreation() *string {
	if pr == nil {
		return nil
	}
	return pr.Creation
}

type MergeStrategy struct {
	AllowMergeCommit         *bool   `yaml:"allow_merge_commit,omitempty"`
	AllowSquashMerge         *bool   `yaml:"allow_squash_merge,omitempty"`
	AllowRebaseMerge         *bool   `yaml:"allow_rebase_merge,omitempty"`
	AllowAutoMerge           *bool   `yaml:"allow_auto_merge,omitempty"`
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

// Disabled reports whether Actions is explicitly turned off. GitHub then
// stops reporting allowed_actions and rejects it with 409, and
// sha_pinning_required and selected actions have no effect, so callers
// neither send nor compare them.
func (a *Actions) Disabled() bool {
	return a != nil && a.Enabled != nil && !*a.Enabled
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

	// BypassActorsSet reports whether bypass_actors was present, so that
	// bypass_actors: [] (remove all) differs from leaving it out (keep).
	BypassActorsSet bool `yaml:"-"`
}

// UnmarshalYAML tracks whether bypass_actors was present.
func (r *Ruleset) UnmarshalYAML(unmarshal func(any) error) error {
	type raw Ruleset
	var v raw
	if err := unmarshal(&v); err != nil {
		return err
	}
	*r = Ruleset(v)

	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}
	if b, ok := fields["bypass_actors"]; ok {
		if b == nil {
			return fmt.Errorf("rulesets[%s].bypass_actors must be a sequence; use [] to remove all bypass actors", r.Name)
		}
		r.BypassActorsSet = true
	}
	return nil
}

// MarshalYAML keeps an explicit bypass_actors: [], which omitempty would drop.
func (r Ruleset) MarshalYAML() (any, error) {
	out := struct {
		Name         string                `yaml:"name"`
		Target       *string               `yaml:"target,omitempty"`
		Enforcement  *string               `yaml:"enforcement,omitempty"`
		BypassActors *[]RulesetBypassActor `yaml:"bypass_actors,omitempty"`
		Conditions   *RulesetConditions    `yaml:"conditions,omitempty"`
		Rules        RulesetRules          `yaml:"rules"`
	}{Name: r.Name, Target: r.Target, Enforcement: r.Enforcement, Conditions: r.Conditions, Rules: r.Rules}
	if r.BypassActorsSet || len(r.BypassActors) > 0 {
		actors := r.BypassActors
		if actors == nil {
			actors = []RulesetBypassActor{}
		}
		out.BypassActors = &actors
	}
	return out, nil
}

// HasBypassActors reports whether the manifest sets bypass_actors.
func (r Ruleset) HasBypassActors() bool {
	return r.BypassActorsSet || len(r.BypassActors) > 0
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
	Update                *RulesetUpdate       `yaml:"update,omitempty"`
	Deletion              *bool                `yaml:"deletion,omitempty"`
	Creation              *bool                `yaml:"creation,omitempty"`
	RequiredLinearHistory *bool                `yaml:"required_linear_history,omitempty"`
	RequiredSignatures    *bool                `yaml:"required_signatures,omitempty"`
}

// RulesetUpdate controls whether matching refs can be updated.
// It supports both bool form and object form with update rule parameters.
//
// Enabled is an internal field and is not exposed in YAML.
type RulesetUpdate struct {
	Enabled             *bool `yaml:"-"`
	AllowsFetchAndMerge *bool `yaml:"allows_fetch_and_merge,omitempty"`
}

// UnmarshalYAML allows RulesetUpdate to be either a bool or a struct.
// Object form implicitly enables the rule.
func (u *RulesetUpdate) UnmarshalYAML(unmarshal func(any) error) error {
	var b bool
	if err := unmarshal(&b); err == nil {
		u.Enabled = &b
		return nil
	}
	type raw RulesetUpdate
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*u = RulesetUpdate(r)
	u.Enabled = Ptr(true)
	return nil
}

// MarshalYAML emits bool form unless update rule parameters are configured.
func (u RulesetUpdate) MarshalYAML() (any, error) {
	if u.Enabled != nil && !*u.Enabled {
		return false, nil
	}
	if u.AllowsFetchAndMerge == nil {
		return true, nil
	}
	return struct {
		AllowsFetchAndMerge bool `yaml:"allows_fetch_and_merge"`
	}{AllowsFetchAndMerge: *u.AllowsFetchAndMerge}, nil
}

// IsEnabled returns the configured enabled state.
func (u *RulesetUpdate) IsEnabled() *bool {
	if u == nil {
		return nil
	}
	return u.Enabled
}

// RulesetPullRequest is the pull_request rule. It is either an object with
// parameters, or `false` to remove the rule from an existing ruleset.
type RulesetPullRequest struct {
	RequiredApprovingReviewCount   *int  `yaml:"required_approving_review_count,omitempty"`
	DismissStaleReviewsOnPush      *bool `yaml:"dismiss_stale_reviews_on_push,omitempty"`
	RequireCodeOwnerReview         *bool `yaml:"require_code_owner_review,omitempty"`
	RequireLastPushApproval        *bool `yaml:"require_last_push_approval,omitempty"`
	RequiredReviewThreadResolution *bool `yaml:"required_review_thread_resolution,omitempty"`

	Disabled bool `yaml:"-"` // set by `pull_request: false`
}

// UnmarshalYAML accepts `false` or an object.
func (p *RulesetPullRequest) UnmarshalYAML(unmarshal func(any) error) error {
	disabled, isBool, err := unmarshalFalse(unmarshal, "pull_request")
	if err != nil || isBool {
		p.Disabled = disabled
		return err
	}
	type raw RulesetPullRequest
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*p = RulesetPullRequest(r)
	return nil
}

// MarshalYAML emits `false` for a disabled rule.
func (p RulesetPullRequest) MarshalYAML() (any, error) {
	if p.Disabled {
		return false, nil
	}
	type raw RulesetPullRequest
	return raw(p), nil
}

// WithDefaults returns a copy with every unset parameter filled with GitHub's
// default (0 or false). The rulesets API requires all of them, and the update
// replaces the whole rule, so plan and apply both use these effective values.
func (p RulesetPullRequest) WithDefaults() RulesetPullRequest {
	if p.RequiredApprovingReviewCount == nil {
		p.RequiredApprovingReviewCount = Ptr(0)
	}
	if p.DismissStaleReviewsOnPush == nil {
		p.DismissStaleReviewsOnPush = Ptr(false)
	}
	if p.RequireCodeOwnerReview == nil {
		p.RequireCodeOwnerReview = Ptr(false)
	}
	if p.RequireLastPushApproval == nil {
		p.RequireLastPushApproval = Ptr(false)
	}
	if p.RequiredReviewThreadResolution == nil {
		p.RequiredReviewThreadResolution = Ptr(false)
	}
	return p
}

// RulesetStatusChecks is the required_status_checks rule. It is either an
// object, or `false` to remove the rule from an existing ruleset.
type RulesetStatusChecks struct {
	StrictRequiredStatusChecksPolicy *bool                `yaml:"strict_required_status_checks_policy,omitempty"`
	Contexts                         []RulesetStatusCheck `yaml:"contexts"`

	Disabled bool `yaml:"-"` // set by `required_status_checks: false`
}

// UnmarshalYAML accepts `false` or an object.
func (c *RulesetStatusChecks) UnmarshalYAML(unmarshal func(any) error) error {
	disabled, isBool, err := unmarshalFalse(unmarshal, "required_status_checks")
	if err != nil || isBool {
		c.Disabled = disabled
		return err
	}
	type raw RulesetStatusChecks
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*c = RulesetStatusChecks(r)
	return nil
}

// MarshalYAML emits `false` for a disabled rule.
func (c RulesetStatusChecks) MarshalYAML() (any, error) {
	if c.Disabled {
		return false, nil
	}
	type raw RulesetStatusChecks
	return raw(c), nil
}

// unmarshalFalse reports whether the value is the bool `false`. `true` is an
// error, since the rule needs its parameters.
func unmarshalFalse(unmarshal func(any) error, rule string) (disabled, isBool bool, err error) {
	var b bool
	if unmarshal(&b) != nil {
		return false, false, nil
	}
	if b {
		return false, true, fmt.Errorf("%s: true is not supported; set its parameters, or false to remove the rule", rule)
	}
	return true, true, nil
}

// PullRequestEnabled reports whether the manifest enables the pull_request rule.
func (r RulesetRules) PullRequestEnabled() bool {
	return r.PullRequest != nil && !r.PullRequest.Disabled
}

// PullRequestDisabled reports whether the manifest removes the pull_request rule.
func (r RulesetRules) PullRequestDisabled() bool {
	return r.PullRequest != nil && r.PullRequest.Disabled
}

// StatusChecksEnabled reports whether the manifest enables required_status_checks.
func (r RulesetRules) StatusChecksEnabled() bool {
	return r.RequiredStatusChecks != nil && !r.RequiredStatusChecks.Disabled
}

// StatusChecksDisabled reports whether the manifest removes required_status_checks.
func (r RulesetRules) StatusChecksDisabled() bool {
	return r.RequiredStatusChecks != nil && r.RequiredStatusChecks.Disabled
}

type RulesetStatusCheck struct {
	Context string `yaml:"context"`
	App     string `yaml:"app,omitempty"` // GitHub App slug (optional)
}

type Label struct {
	Name        string  `yaml:"name"        validate:"required"`
	Description *string `yaml:"description,omitempty"` // omitted leaves it as it is; "" clears
	Color       string  `yaml:"color"       validate:"required"`
}

// LabelSyncMode returns the effective label sync mode.
// nil is treated as additive (safe default).
func LabelSyncMode(s *string) string {
	if s == nil {
		return LabelSyncAdditive
	}
	return *s
}

func LabelsReconcileMode(r *RepositoryReconcile, legacyLabelSync *string) string {
	if r != nil && r.Labels != nil {
		return *r.Labels
	}
	switch LabelSyncMode(legacyLabelSync) {
	case LabelSyncMirror:
		return CollectionReconcileAuthoritative
	default:
		return CollectionReconcileAdditive
	}
}

func BranchProtectionReconcileMode(r *RepositoryReconcile) string {
	if r == nil || r.BranchProtection == nil {
		return CollectionReconcileAdditive
	}
	return *r.BranchProtection
}

func RulesetsReconcileMode(r *RepositoryReconcile) string {
	if r == nil || r.Rulesets == nil {
		return CollectionReconcileAdditive
	}
	return *r.Rulesets
}

func DeploymentBranchPoliciesReconcileMode(r *RepositoryReconcile) string {
	if r == nil || r.DeploymentBranchPolicies == nil {
		return CollectionReconcileAdditive
	}
	return *r.DeploymentBranchPolicies
}

type Milestone struct {
	Title       string  `yaml:"title"       validate:"required"`
	Description *string `yaml:"description,omitempty"`
	State       *string `yaml:"state,omitempty"  validate:"omitempty,oneof=open closed"`
	DueOn       *string `yaml:"due_on,omitempty"`
}

// MilestoneState returns the state a new milestone is created with:
// the manifest's, or GitHub's default "open". An existing milestone keeps its
// state when the manifest leaves it out.
func MilestoneState(s *string) string {
	if s == nil {
		return "open"
	}
	return *s
}

type Secret struct {
	Name  string `yaml:"name" validate:"required"`
	Value string `yaml:"value"`
}

type Variable struct {
	Name  string `yaml:"name" validate:"required"`
	Value string `yaml:"value"`
}

// Environment is a deployment environment. gh-infra creates and updates the
// environments a manifest declares and never deletes one: GitHub creates
// environments too (github-pages, or one a workflow names).
type Environment struct {
	Name                   string                  `yaml:"name" validate:"required"`
	WaitTimer              *int                    `yaml:"wait_timer,omitempty"`
	PreventSelfReview      *bool                   `yaml:"prevent_self_review,omitempty"`
	CanAdminsBypass        *bool                   `yaml:"can_admins_bypass,omitempty"`
	Reviewers              []EnvironmentReviewer   `yaml:"reviewers,omitempty"`
	DeploymentBranchPolicy *DeploymentBranchPolicy `yaml:"deployment_branch_policy,omitempty"`
	// Variables and Secrets are added and updated, never deleted, as those
	// of the repository.
	Variables []Variable `yaml:"variables,omitempty" validate:"unique=name"`
	Secrets   []Secret   `yaml:"secrets,omitempty"   validate:"unique=name"`

	ReviewersSet bool `yaml:"-"`
	VariablesSet bool `yaml:"-"`
	SecretsSet   bool `yaml:"-"`
}

// EnvironmentReviewer is a user or a team who can approve deployments.
type EnvironmentReviewer struct {
	User string `yaml:"user,omitempty"`
	Team string `yaml:"team,omitempty"` // team slug
}

// DeploymentBranchPolicy says which branches and tags can deploy to an
// environment: {} for all, protected_branches: true for protected branches,
// or custom_branch_policies for name patterns.
type DeploymentBranchPolicy struct {
	ProtectedBranches    *bool                  `yaml:"protected_branches,omitempty"`
	CustomBranchPolicies []DeploymentBranchRule `yaml:"custom_branch_policies,omitempty"`

	CustomBranchPoliciesSet bool `yaml:"-"`
}

// DeploymentBranchRule is a name pattern of branches or tags.
type DeploymentBranchRule struct {
	Name string `yaml:"name" validate:"required"`
	Type string `yaml:"type,omitempty" validate:"omitempty,oneof=branch tag"`
}

// RuleType returns the type of the pattern; GitHub's default is branch.
func (r DeploymentBranchRule) RuleType() string {
	if r.Type == "" {
		return DeploymentBranchTypeBranch
	}
	return r.Type
}

// UnmarshalYAML tracks whether reviewers was present, and rejects a null
// reviewers or deployment_branch_policy: a null policy reads too much like
// {}, which allows every branch.
func (e *Environment) UnmarshalYAML(unmarshal func(any) error) error {
	type raw Environment
	var v raw
	if err := unmarshal(&v); err != nil {
		return err
	}
	*e = Environment(v)

	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}
	if r, ok := fields["reviewers"]; ok {
		if r == nil {
			return fmt.Errorf("environments[%s].reviewers must be a sequence; use [] to remove all reviewers", e.Name)
		}
		e.ReviewersSet = true
	}
	if p, ok := fields["deployment_branch_policy"]; ok && p == nil {
		return fmt.Errorf("environments[%s].deployment_branch_policy must not be null; use {} to allow all branches and tags", e.Name)
	}
	if v, ok := fields["variables"]; ok {
		if v == nil {
			return fmt.Errorf("environments[%s].variables must be a sequence; use [] to inherit no variables from defaults", e.Name)
		}
		e.VariablesSet = true
	}
	if v, ok := fields["secrets"]; ok {
		if v == nil {
			return fmt.Errorf("environments[%s].secrets must be a sequence; use [] to inherit no secrets from defaults", e.Name)
		}
		e.SecretsSet = true
	}
	return nil
}

// MarshalYAML keeps an explicit reviewers: [], variables: [] and secrets: [],
// which omitempty would drop.
func (e Environment) MarshalYAML() (any, error) {
	out := struct {
		Name                   string                  `yaml:"name"`
		WaitTimer              *int                    `yaml:"wait_timer,omitempty"`
		PreventSelfReview      *bool                   `yaml:"prevent_self_review,omitempty"`
		CanAdminsBypass        *bool                   `yaml:"can_admins_bypass,omitempty"`
		Reviewers              *[]EnvironmentReviewer  `yaml:"reviewers,omitempty"`
		DeploymentBranchPolicy *DeploymentBranchPolicy `yaml:"deployment_branch_policy,omitempty"`
		Variables              *[]Variable             `yaml:"variables,omitempty"`
		Secrets                *[]Secret               `yaml:"secrets,omitempty"`
	}{Name: e.Name, WaitTimer: e.WaitTimer, PreventSelfReview: e.PreventSelfReview, CanAdminsBypass: e.CanAdminsBypass,
		DeploymentBranchPolicy: e.DeploymentBranchPolicy}
	// An explicit [] drops the defaults' variables or secrets in a
	// RepositorySet entry.
	if e.VariablesSet || len(e.Variables) > 0 {
		vars := e.Variables
		if vars == nil {
			vars = []Variable{}
		}
		out.Variables = &vars
	}
	if e.SecretsSet || len(e.Secrets) > 0 {
		secrets := e.Secrets
		if secrets == nil {
			secrets = []Secret{}
		}
		out.Secrets = &secrets
	}
	if e.HasReviewers() {
		reviewers := e.Reviewers
		if reviewers == nil {
			reviewers = []EnvironmentReviewer{}
		}
		out.Reviewers = &reviewers
	}
	return out, nil
}

// HasReviewers reports whether the manifest sets reviewers.
func (e Environment) HasReviewers() bool {
	return e.ReviewersSet || len(e.Reviewers) > 0
}

// UnmarshalYAML tracks whether custom_branch_policies was present: [] allows
// no branch, while leaving it out of {} allows every branch.
func (p *DeploymentBranchPolicy) UnmarshalYAML(unmarshal func(any) error) error {
	type raw DeploymentBranchPolicy
	var v raw
	if err := unmarshal(&v); err != nil {
		return err
	}
	*p = DeploymentBranchPolicy(v)

	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}
	if c, ok := fields["custom_branch_policies"]; ok {
		if c == nil {
			return fmt.Errorf("custom_branch_policies must be a sequence; use [] to allow no branch")
		}
		p.CustomBranchPoliciesSet = true
	}
	return nil
}

// MarshalYAML keeps an explicit custom_branch_policies: [], which omitempty
// would drop and so turn "no branch" into "every branch".
func (p DeploymentBranchPolicy) MarshalYAML() (any, error) {
	out := struct {
		ProtectedBranches    *bool                   `yaml:"protected_branches,omitempty"`
		CustomBranchPolicies *[]DeploymentBranchRule `yaml:"custom_branch_policies,omitempty"`
	}{ProtectedBranches: p.ProtectedBranches}
	if p.HasCustom() {
		rules := p.CustomBranchPolicies
		if rules == nil {
			rules = []DeploymentBranchRule{}
		}
		out.CustomBranchPolicies = &rules
	}
	return out, nil
}

// HasCustom reports whether custom_branch_policies is set.
func (p DeploymentBranchPolicy) HasCustom() bool {
	return p.CustomBranchPoliciesSet || len(p.CustomBranchPolicies) > 0
}

// Mode returns which branches can deploy: all, protected or custom.
func (p DeploymentBranchPolicy) Mode() string {
	switch {
	case p.HasCustom():
		return DeploymentBranchesCustom
	case p.ProtectedBranches != nil && *p.ProtectedBranches:
		return DeploymentBranchesProtected
	default:
		return DeploymentBranchesAll
	}
}
