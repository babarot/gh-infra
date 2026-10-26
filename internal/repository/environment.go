package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// Fields of an environment change. The settings child carries the names of
// the fields its PUT sends; a branch_policy child creates or deletes one
// name pattern.
const (
	envFieldSettings     = "settings"
	envFieldWaitTimer    = "wait_timer"
	envFieldSelfReview   = "prevent_self_review"
	envFieldAdminsBypass = "can_admins_bypass"
	envFieldReviewers    = "reviewers"
	envFieldBranchPolicy = "deployment_branch_policy"
	envFieldBranchRule   = "branch_policy"
	envFieldVariable     = "variable"
	envFieldSecret       = "secret"
)

// defaultBranchPolicy is shown for a new environment whose policy the
// manifest leaves out.
const defaultBranchPolicy = manifest.DeploymentBranchesAll + " (GitHub's default)"

func hasDetail(c Change, match func(Change) bool) bool {
	return slices.ContainsFunc(c.Details, match)
}

// removedByGitHub marks a pattern GitHub deletes itself when a policy stops
// being custom: the plan shows it, and apply sends nothing for it.
const removedByGitHub = " (removed by GitHub)"

func environmentResource(name string) string {
	return fmt.Sprintf("%s[%s]", manifest.ResourceEnvironment, name)
}

// keptReviewers returns the reviewers an apply can add to an environment of
// a repository it creates: only its owner. GitHub drops a reviewer who cannot
// read the repository, and in a new one no other user or team can yet (gh-infra
// does not manage access). deferred reports whether any were left for a
// later plan.
func keptReviewers(owner string, env manifest.Environment, newRepo bool) (kept []manifest.EnvironmentReviewer, deferred bool) {
	if !newRepo {
		return env.Reviewers, false
	}
	for _, rv := range env.Reviewers {
		if rv.User != "" && strings.EqualFold(rv.User, owner) {
			kept = append(kept, rv)
		} else {
			deferred = true
		}
	}
	return kept, deferred
}

// sendsSelfReview reports whether prevent_self_review can be sent with
// reviewers: GitHub keeps it in the reviewers rule, so it waits for the
// reviewers when they are all deferred.
func sendsSelfReview(env manifest.Environment, kept []manifest.EnvironmentReviewer, deferred bool) bool {
	return env.PreventSelfReview != nil && (!deferred || len(kept) > 0)
}

func formatReviewers(rvs []manifest.EnvironmentReviewer) string {
	if len(rvs) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(rvs))
	for _, rv := range rvs {
		if rv.Team != "" {
			names = append(names, "team:"+rv.Team)
		} else {
			names = append(names, rv.User)
		}
	}
	return strings.Join(names, ", ")
}

func currentReviewers(rvs []CurrentEnvironmentReviewer) []manifest.EnvironmentReviewer {
	out := make([]manifest.EnvironmentReviewer, 0, len(rvs))
	for _, rv := range rvs {
		if rv.Type == manifest.EnvironmentReviewerTeam {
			out = append(out, manifest.EnvironmentReviewer{Team: rv.Name})
		} else {
			out = append(out, manifest.EnvironmentReviewer{User: rv.Name})
		}
	}
	return out
}

func reviewerKey(rv manifest.EnvironmentReviewer) string {
	if rv.Team != "" {
		return "team:" + strings.ToLower(rv.Team)
	}
	return "user:" + strings.ToLower(rv.User)
}

// sameReviewers compares reviewers as sets, as GitHub does not keep an order.
func sameReviewers(a, b []manifest.EnvironmentReviewer) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, rv := range a {
		set[reviewerKey(rv)] = true
	}
	for _, rv := range b {
		if !set[reviewerKey(rv)] {
			return false
		}
	}
	return true
}

func formatBranchRule(name, typ string) string {
	if typ == manifest.DeploymentBranchTypeTag {
		return "tag:" + name
	}
	return name
}

// diffEnvironments plans the environments the manifest declares. Each one is
// a single change: Details show what changes, and Children are what apply
// does in order: the PUT of the environment, then the patterns.
// Environments the manifest leaves out are left alone.
func diffEnvironments(name string, desired *manifest.Repository, current *CurrentState, forceSecrets bool) []Change {
	if !desired.Spec.ManagesEnvironments() {
		return nil
	}
	reconcile := manifest.DeploymentBranchPoliciesReconcileMode(desired.Reconcile)
	var changes []Change
	for _, env := range desired.Spec.Environments {
		cur := current.Environments[strings.ToLower(env.Name)]
		var c Change
		if cur == nil {
			c = diffNewEnvironment(name, desired.Metadata.Owner, env, current.IsNew)
		} else {
			c = diffExistingEnvironment(name, env, cur, reconcile)
		}
		diffEnvironmentValues(&c, env, cur, forceSecrets)
		if len(c.Children) > 0 {
			changes = append(changes, c)
		}
	}
	return changes
}

func diffNewEnvironment(name, owner string, env manifest.Environment, newRepo bool) Change {
	var details []Change
	var fields []string
	add := func(field string, v any) {
		details = append(details, Change{Type: ChangeCreate, Field: field, NewValue: v})
		fields = append(fields, field)
	}
	// wait_timer: 0 is no wait timer, as GitHub creates an environment.
	if env.WaitTimer != nil && *env.WaitTimer > 0 {
		add(envFieldWaitTimer, *env.WaitTimer)
	}
	if env.CanAdminsBypass != nil {
		add(envFieldAdminsBypass, *env.CanAdminsBypass)
	}
	kept, deferred := keptReviewers(owner, env, newRepo)
	if len(kept) > 0 {
		add(envFieldReviewers, formatReviewers(kept))
	}
	if sendsSelfReview(env, kept, deferred) {
		add(envFieldSelfReview, *env.PreventSelfReview)
	}
	var rules []Change
	if p := env.DeploymentBranchPolicy; p != nil {
		add(envFieldBranchPolicy, p.Mode())
		for _, r := range p.CustomBranchPolicies {
			rules = append(rules, Change{Type: ChangeCreate, Field: envFieldBranchRule, NewValue: r})
			details = append(details, Change{Type: ChangeCreate, Field: envFieldBranchRule, NewValue: formatBranchRule(r.Name, r.RuleType())})
		}
	} else {
		details = append(details, Change{Type: ChangeCreate, Field: envFieldBranchPolicy, NewValue: defaultBranchPolicy})
	}
	children := append([]Change{{Type: ChangeCreate, Field: envFieldSettings, NewValue: fields}}, rules...)
	return Change{
		Type:     ChangeCreate,
		Resource: environmentResource(env.Name),
		Name:     name,
		Field:    "environment",
		NewValue: env.Name,
		Details:  details,
		Children: children,
	}
}

func diffExistingEnvironment(name string, env manifest.Environment, cur *CurrentEnvironment, reconcile string) Change {
	var details []Change
	var fields []string
	update := func(field string, old, new any) {
		details = append(details, Change{Type: ChangeUpdate, Field: field, OldValue: old, NewValue: new})
		fields = append(fields, field)
	}
	if w := env.WaitTimer; w != nil && *w != cur.WaitTimer {
		update(envFieldWaitTimer, cur.WaitTimer, *w)
	}
	if b := env.CanAdminsBypass; b != nil && *b != cur.CanAdminsBypass {
		update(envFieldAdminsBypass, cur.CanAdminsBypass, *b)
	}
	if env.HasReviewers() {
		if old := currentReviewers(cur.Reviewers); !sameReviewers(env.Reviewers, old) {
			update(envFieldReviewers, formatReviewers(old), formatReviewers(env.Reviewers))
		}
	}
	// Without a reviewers rule, prevent_self_review is not set: GitHub
	// creates the rule with false.
	if v := env.PreventSelfReview; v != nil {
		if old := cur.HasReviewersRule && cur.PreventSelfReview; *v != old {
			update(envFieldSelfReview, old, *v)
		}
	}

	var deletes, creates []Change
	if p := env.DeploymentBranchPolicy; p != nil {
		mode := p.Mode()
		if mode != cur.BranchPolicy {
			update(envFieldBranchPolicy, cur.BranchPolicy, mode)
		}
		if mode != manifest.DeploymentBranchesCustom {
			// GitHub deletes the patterns of a policy that stops being
			// custom; the plan shows it, though apply sends nothing for it.
			for _, r := range cur.BranchRules {
				details = append(details, Change{Type: ChangeDelete, Field: envFieldBranchRule, OldValue: formatBranchRule(r.Name, r.Type) + removedByGitHub})
			}
		} else {
			// A policy that is not custom has no patterns: GitHub deleted
			// them when it stopped being custom.
			var curRules []CurrentBranchRule
			if cur.BranchPolicy == manifest.DeploymentBranchesCustom {
				curRules = cur.BranchRules
			}
			want := make(map[[2]string]bool, len(p.CustomBranchPolicies))
			for _, r := range p.CustomBranchPolicies {
				want[[2]string{r.Name, r.RuleType()}] = true
			}
			have := make(map[[2]string]bool, len(curRules))
			for _, r := range curRules {
				have[[2]string{r.Name, r.Type}] = true
				if !want[[2]string{r.Name, r.Type}] && reconcile == manifest.CollectionReconcileAuthoritative {
					deletes = append(deletes, Change{Type: ChangeDelete, Field: envFieldBranchRule, OldValue: r})
					details = append(details, Change{Type: ChangeDelete, Field: envFieldBranchRule, OldValue: formatBranchRule(r.Name, r.Type)})
				}
			}
			for _, r := range p.CustomBranchPolicies {
				if !have[[2]string{r.Name, r.RuleType()}] {
					creates = append(creates, Change{Type: ChangeCreate, Field: envFieldBranchRule, NewValue: r})
					details = append(details, Change{Type: ChangeCreate, Field: envFieldBranchRule, NewValue: formatBranchRule(r.Name, r.RuleType())})
				}
			}
		}
	}

	var children []Change
	if len(fields) > 0 {
		children = append(children, Change{Type: ChangeUpdate, Field: envFieldSettings, NewValue: fields})
	}
	children = append(children, deletes...)
	children = append(children, creates...)
	return Change{
		Type:     ChangeUpdate,
		Resource: environmentResource(cur.Name),
		Name:     name,
		Field:    "environment",
		NewValue: cur.Name,
		Details:  details,
		Children: children,
	}
}

// diffEnvironmentValues adds the variables and secrets of an environment to
// its change; they are set after the environment exists. As for those of the
// repository, a secret's value cannot be read, so an existing secret is set
// again only with --force-secrets, and none is deleted.
func diffEnvironmentValues(c *Change, env manifest.Environment, cur *CurrentEnvironment, forceSecrets bool) {
	var curVars map[string]string
	var curSecrets []string
	if cur != nil {
		curVars, curSecrets = cur.Variables, cur.Secrets
	}
	for _, v := range env.Variables {
		old, exists := curVars[v.Name]
		switch {
		case !exists:
			c.Details = append(c.Details, Change{Type: ChangeCreate, Field: "variables." + v.Name, NewValue: v.Value})
		case old != v.Value:
			c.Details = append(c.Details, Change{Type: ChangeUpdate, Field: "variables." + v.Name, OldValue: old, NewValue: v.Value})
		default:
			continue
		}
		c.Children = append(c.Children, Change{Type: ChangeUpdate, Field: envFieldVariable, NewValue: v.Name})
	}
	for _, sec := range env.Secrets {
		switch {
		case !slices.Contains(curSecrets, sec.Name):
			c.Details = append(c.Details, Change{Type: ChangeCreate, Field: "secrets." + sec.Name, NewValue: "(new)"})
		case forceSecrets:
			c.Details = append(c.Details, Change{Type: ChangeUpdate, Field: "secrets." + sec.Name, OldValue: "(exists)", NewValue: "(force update)"})
		default:
			continue
		}
		c.Children = append(c.Children, Change{Type: ChangeUpdate, Field: envFieldSecret, NewValue: sec.Name})
	}
}

// environmentCreateNotes lists the reviewers a new repository's environments
// get from a later plan.
func environmentCreateNotes(desired *manifest.Repository) []string {
	var notes []string
	for _, env := range desired.Spec.Environments {
		kept, deferred := keptReviewers(desired.Metadata.Owner, env, true)
		if !deferred {
			continue
		}
		what := "reviewers"
		if env.PreventSelfReview != nil && len(kept) == 0 {
			what = "reviewers and prevent_self_review"
		} else if len(kept) > 0 {
			what = "reviewers other than " + desired.Metadata.Owner
		}
		notes = append(notes, fmt.Sprintf("%s of environment %q are applied by a later plan, once they can read the repository", what, env.Name))
	}
	return notes
}

// ChangeNotes warns about planned changes that let more deploy to an
// environment than before, which are easy to miss in a plan.
func ChangeNotes(changes []Change) []string {
	var notes []string
	for _, c := range changes {
		resource, env, ok := splitApplyResource(c.Resource)
		if !ok || resource != manifest.ResourceEnvironment {
			continue
		}
		if c.Type == ChangeCreate {
			// A new environment allows every branch unless the manifest
			// limits it, so its secrets are readable from any branch.
			if hasDetail(c, func(d Change) bool { return d.Field == envFieldBranchPolicy && d.NewValue == defaultBranchPolicy }) &&
				hasDetail(c, func(d Change) bool { return strings.HasPrefix(d.Field, "secrets.") }) {
				notes = append(notes, fmt.Sprintf("environment %q: its secrets are readable from any branch; set deployment_branch_policy to limit them", env))
			}
			continue
		}
		for _, d := range c.Details {
			if d.Type != ChangeUpdate {
				continue
			}
			switch {
			case d.Field == envFieldBranchPolicy && d.NewValue == manifest.DeploymentBranchesAll:
				notes = append(notes, fmt.Sprintf("environment %q: any branch or tag can deploy after apply", env))
			case d.Field == envFieldReviewers && d.NewValue == "(none)":
				notes = append(notes, fmt.Sprintf("environment %q: deployments no longer need a review", env))
			case d.Field == envFieldAdminsBypass && d.NewValue == true:
				notes = append(notes, fmt.Sprintf("environment %q: admins can bypass its protection rules", env))
			}
		}
	}
	return notes
}

// validateEnvironmentDependencies checks what needs the current state:
// prevent_self_review needs reviewers to keep it in.
func validateEnvironmentDependencies(desired *manifest.Repository, current *CurrentState) error {
	for _, env := range desired.Spec.Environments {
		if env.PreventSelfReview == nil {
			continue
		}
		reviewers := env.Reviewers
		if !env.HasReviewers() {
			if cur := current.Environments[strings.ToLower(env.Name)]; cur != nil {
				reviewers = currentReviewers(cur.Reviewers)
			}
		}
		if len(reviewers) == 0 {
			return fmt.Errorf("environments[%s].prevent_self_review needs at least one reviewer, and the environment has none (list reviewers)", env.Name)
		}
	}
	return nil
}

// sentEnvironmentFields returns, per environment, the fields its planned PUT
// sends.
func sentEnvironmentFields(changes []Change) map[string][]string {
	out := make(map[string][]string)
	for _, c := range changes {
		resource, env, ok := splitApplyResource(c.Resource)
		if !ok || resource != manifest.ResourceEnvironment {
			continue
		}
		for _, child := range c.Children {
			if fields, ok := child.NewValue.([]string); ok && child.Field == envFieldSettings {
				out[strings.ToLower(env)] = fields
			}
		}
	}
	return out
}

// checkEnvironments catches at plan what GitHub would reject or silently
// drop when the planned environment changes are applied: reviewers it cannot
// find or who cannot read the repository, and wait timers and reviewers the
// owner's plan does not allow on a private repository.
func (p *Processor) checkEnvironments(ctx context.Context, desired *manifest.Repository, current *CurrentState, changes []Change, resolver *manifest.Resolver) error {
	sent := sentEnvironmentFields(changes)
	if len(sent) == 0 {
		return nil
	}
	owner, repo := desired.Metadata.Owner, desired.Metadata.Name
	private := effectiveVisibility(desired, current) == manifest.VisibilityPrivate
	for _, env := range desired.Spec.Environments {
		fields, ok := sent[strings.ToLower(env.Name)]
		if !ok {
			continue
		}
		var cur *CurrentEnvironment
		if current.Environments != nil {
			cur = current.Environments[strings.ToLower(env.Name)]
		}
		if slices.Contains(fields, envFieldReviewers) {
			kept, _ := keptReviewers(owner, env, current.IsNew)
			var have []manifest.EnvironmentReviewer
			if cur != nil {
				have = currentReviewers(cur.Reviewers)
			}
			for _, rv := range kept {
				if resolver != nil {
					if _, err := resolver.ResolveEnvironmentReviewer(ctx, rv); err != nil {
						return fmt.Errorf("environments[%s].reviewers: %w", env.Name, err)
					}
				}
				// A new repository has no access to check: only its owner is
				// added there (keptReviewers). Reviewers already on the
				// environment are not checked again.
				if current.IsNew || slices.ContainsFunc(have, func(h manifest.EnvironmentReviewer) bool { return reviewerKey(h) == reviewerKey(rv) }) {
					continue
				}
				if err := p.checkReviewerAccess(ctx, owner, repo, rv); err != nil {
					return fmt.Errorf("environments[%s].reviewers: %w", env.Name, err)
				}
			}
		}
		if private {
			if err := p.checkPlanAllows(ctx, owner, env.Name, fields, current); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkReviewerAccess fails when rv cannot read the repository: GitHub
// accepts such a reviewer and drops it.
func (p *Processor) checkReviewerAccess(ctx context.Context, owner, repo string, rv manifest.EnvironmentReviewer) error {
	path := fmt.Sprintf("repos/%s/%s/collaborators/%s", owner, repo, rv.User)
	if rv.Team != "" {
		path = fmt.Sprintf("orgs/%s/teams/%s/repos/%s/%s", owner, rv.Team, owner, repo)
	}
	_, err := p.runner.Run(ctx, "api", path)
	if err == nil {
		return nil
	}
	if errors.Is(err, gh.ErrNotFound) {
		return fmt.Errorf("%s cannot read %s/%s, and GitHub drops a reviewer without read access; give it access first", formatReviewers([]manifest.EnvironmentReviewer{rv}), owner, repo)
	}
	return fmt.Errorf("check access of %s: %w", formatReviewers([]manifest.EnvironmentReviewer{rv}), err)
}

// checkPlanAllows fails when the planned PUT sends a wait timer or reviewers
// to an environment of a private repository and the owner's plan may not
// allow them (GitHub Free, Pro and Team allow them only on public
// repositories). An environment of the repository that already has the rule
// shows the plan allows it; otherwise the owner's plan is read, which needs
// an organization owner's token or, for a user, the user's own token with
// the user scope.
func (p *Processor) checkPlanAllows(ctx context.Context, owner, env string, fields []string, current *CurrentState) error {
	var rules []string
	if slices.Contains(fields, envFieldWaitTimer) {
		rules = append(rules, "wait_timer")
	}
	if slices.Contains(fields, envFieldReviewers) || slices.Contains(fields, envFieldSelfReview) {
		rules = append(rules, "reviewers")
	}
	for _, rule := range rules {
		if repositoryHasRule(current, rule) {
			continue
		}
		plan, err := p.ownerPlan(ctx, owner)
		if err != nil {
			return err
		}
		switch plan {
		case "enterprise":
			continue
		case "free", "pro", "team":
			return fmt.Errorf("environments[%s].%s: GitHub's %s plan allows it only on public repositories", env, rule, plan)
		default:
			return fmt.Errorf("environments[%s].%s: cannot tell whether the plan of %s allows it on a private repository (GitHub Free, Pro and Team do not); plan with a token that can read the plan (an organization owner's, or the user's own with the user scope), or add it once on GitHub", env, rule, owner)
		}
	}
	return nil
}

// repositoryHasRule reports whether an environment of the repository has the
// rule, which shows the owner's plan allows it.
func repositoryHasRule(current *CurrentState, rule string) bool {
	for _, e := range current.Environments {
		if (rule == "wait_timer" && e.WaitTimer > 0) || (rule == "reviewers" && e.HasReviewersRule) {
			return true
		}
	}
	return false
}

// planCache holds the plan of each owner, read once per run.
type planCache struct {
	mu    sync.Mutex
	plans map[string]string
}

// ownerPlan returns the name of the owner's GitHub plan, or "" when it
// cannot be read: an organization returns its plan only to an owner, and a
// user only to the user, with the user scope.
func (p *Processor) ownerPlan(ctx context.Context, owner string) (string, error) {
	p.plans.mu.Lock()
	defer p.plans.mu.Unlock()
	key := strings.ToLower(owner)
	if plan, ok := p.plans.plans[key]; ok {
		return plan, nil
	}
	var resp struct {
		Login string `json:"login"`
		Plan  *struct {
			Name string `json:"name"`
		} `json:"plan"`
	}
	out, err := p.runner.Run(ctx, "api", "orgs/"+owner)
	if errors.Is(err, gh.ErrNotFound) {
		// A user: only the user's own plan can be read.
		out, err = p.runner.Run(ctx, "api", "user")
		if err == nil {
			if err := json.Unmarshal(out, &resp); err != nil {
				return "", fmt.Errorf("parse the authenticated user: %w", err)
			}
			if !strings.EqualFold(resp.Login, owner) {
				resp.Plan = nil
			}
		}
	} else if err == nil {
		if err := json.Unmarshal(out, &resp); err != nil {
			return "", fmt.Errorf("parse organization %s: %w", owner, err)
		}
	}
	if err != nil {
		return "", fmt.Errorf("read the plan of %s: %w", owner, err)
	}
	plan := ""
	if resp.Plan != nil {
		plan = strings.ToLower(resp.Plan.Name)
	}
	p.plans.plans[key] = plan
	return plan, nil
}

// findEnvironment returns the manifest's environment named name, ignoring
// case as GitHub does.
func findEnvironment(envs []manifest.Environment, name string) *manifest.Environment {
	for i := range envs {
		if strings.EqualFold(envs[i].Name, name) {
			return &envs[i]
		}
	}
	return nil
}

// applyEnvironment applies one child of an environment change.
func (p *Processor) applyEnvironment(ctx context.Context, c Change, repo *manifest.Repository) error {
	owner, name := repo.Metadata.Owner, repo.Metadata.Name
	fullName := owner + "/" + name
	_, envName, _ := splitApplyResource(c.Resource)
	path := environmentPath(owner, name, envName)
	target := fmt.Sprintf("environment:%s", envName)

	switch {
	case c.Field == envFieldSettings:
		env := findEnvironment(repo.Spec.Environments, envName)
		if env == nil {
			return wrapError(fmt.Errorf("environment %q is not in the manifest", envName), fullName, target)
		}
		fields, _ := c.NewValue.([]string)
		body, sent, err := p.environmentPayload(ctx, repo, *env, fields, isNewRepository(ctx))
		if err != nil {
			return wrapError(err, fullName, target)
		}
		out, err := p.runner.RunWithStdin(ctx, body, "api", path, "--method", "PUT", "--input", "-")
		if err != nil {
			return wrapError(err, fullName, target)
		}
		return wrapError(verifyEnvironmentResponse(out, sent), fullName, target)

	case c.Field == envFieldBranchRule && c.Type == ChangeCreate:
		r, _ := c.NewValue.(manifest.DeploymentBranchRule)
		_, err := p.runner.Run(ctx, "api", path+"/deployment-branch-policies", "--method", "POST",
			"-f", "name="+r.Name, "-f", "type="+r.RuleType())
		return wrapError(err, fullName, target+":"+formatBranchRule(r.Name, r.RuleType()))

	case c.Field == envFieldVariable || c.Field == envFieldSecret:
		env := findEnvironment(repo.Spec.Environments, envName)
		key, _ := c.NewValue.(string)
		value, ok := environmentValue(env, c.Field, key)
		if !ok {
			return wrapError(fmt.Errorf("%s %q of environment %q is not in the manifest", c.Field, key, envName), fullName, target)
		}
		_, err := p.runner.Run(ctx, c.Field, "set", key, "--repo", fullName, "--env", envName, "--body", value)
		return wrapError(err, fullName, target+":"+c.Field+":"+key)

	case c.Field == envFieldBranchRule && c.Type == ChangeDelete:
		r, _ := c.OldValue.(CurrentBranchRule)
		_, err := p.runner.Run(ctx, "api", fmt.Sprintf("%s/deployment-branch-policies/%d", path, r.ID), "--method", "DELETE")
		return wrapError(err, fullName, target+":"+formatBranchRule(r.Name, r.Type))
	}
	return fmt.Errorf("unknown environment change: %s %s", c.Type, c.Field)
}

// environmentValue returns the value of an environment's variable or secret
// in the manifest, never one of the repository with the same name.
func environmentValue(env *manifest.Environment, kind, key string) (string, bool) {
	if env == nil {
		return "", false
	}
	if kind == envFieldSecret {
		for _, s := range env.Secrets {
			if s.Name == key {
				return s.Value, true
			}
		}
		return "", false
	}
	for _, v := range env.Variables {
		if v.Name == key {
			return v.Value, true
		}
	}
	return "", false
}

// sentEnvironment is what a PUT sent, to check GitHub saved it.
type sentEnvironment struct {
	waitTimer    *int
	adminsBypass *bool
	selfReview   *bool
	reviewers    []manifest.ResolvedEnvironmentReviewer
	hasReviewers bool
	branchPolicy string
}

// environmentPayload builds the PUT body of the fields the plan sends.
// GitHub keeps the fields a PUT leaves out.
func (p *Processor) environmentPayload(ctx context.Context, repo *manifest.Repository, env manifest.Environment, fields []string, newRepo bool) ([]byte, sentEnvironment, error) {
	body := map[string]any{}
	var sent sentEnvironment
	for _, f := range fields {
		switch f {
		case envFieldWaitTimer:
			body["wait_timer"] = *env.WaitTimer
			sent.waitTimer = env.WaitTimer
		case envFieldAdminsBypass:
			body["can_admins_bypass"] = *env.CanAdminsBypass
			sent.adminsBypass = env.CanAdminsBypass
		case envFieldSelfReview:
			body["prevent_self_review"] = *env.PreventSelfReview
			sent.selfReview = env.PreventSelfReview
		case envFieldReviewers:
			kept, _ := keptReviewers(repo.Metadata.Owner, env, newRepo)
			resolver := p.resolver.For(repo.Metadata.Owner)
			if resolver == nil {
				return nil, sent, fmt.Errorf("internal error: no resolver for environment reviewers")
			}
			reviewers := make([]map[string]any, 0, len(kept))
			for _, rv := range kept {
				r, err := resolver.ResolveEnvironmentReviewer(ctx, rv)
				if err != nil {
					return nil, sent, err
				}
				reviewers = append(reviewers, map[string]any{"type": r.Type, "id": r.ID})
				sent.reviewers = append(sent.reviewers, r)
			}
			body["reviewers"] = reviewers
			sent.hasReviewers = true
		case envFieldBranchPolicy:
			mode := env.DeploymentBranchPolicy.Mode()
			switch mode {
			case manifest.DeploymentBranchesAll:
				body["deployment_branch_policy"] = nil
			case manifest.DeploymentBranchesProtected:
				body["deployment_branch_policy"] = map[string]bool{"protected_branches": true, "custom_branch_policies": false}
			case manifest.DeploymentBranchesCustom:
				body["deployment_branch_policy"] = map[string]bool{"protected_branches": false, "custom_branch_policies": true}
			}
			sent.branchPolicy = mode
		}
	}
	data, err := json.Marshal(body)
	return data, sent, err
}

// verifyEnvironmentResponse checks that GitHub saved what the PUT sent. It
// accepts a reviewer who cannot read the repository and drops it, for one.
func verifyEnvironmentResponse(out []byte, sent sentEnvironment) error {
	var e apiEnvironment
	if err := json.Unmarshal(out, &e); err != nil {
		return fmt.Errorf("parse the environment GitHub returned: %w", err)
	}
	got := e.current()
	var lost []string
	if sent.waitTimer != nil && *sent.waitTimer != got.WaitTimer {
		lost = append(lost, fmt.Sprintf("wait_timer (sent %d, got %d)", *sent.waitTimer, got.WaitTimer))
	}
	if sent.adminsBypass != nil && *sent.adminsBypass != got.CanAdminsBypass {
		lost = append(lost, fmt.Sprintf("can_admins_bypass (sent %t, got %t)", *sent.adminsBypass, got.CanAdminsBypass))
	}
	if sent.selfReview != nil && (*sent.selfReview != (got.HasReviewersRule && got.PreventSelfReview)) {
		lost = append(lost, fmt.Sprintf("prevent_self_review (sent %t)", *sent.selfReview))
	}
	if sent.hasReviewers {
		want := make(map[string]bool, len(sent.reviewers))
		for _, r := range sent.reviewers {
			want[fmt.Sprintf("%s:%d", r.Type, r.ID)] = true
		}
		have := make(map[string]bool, len(got.Reviewers))
		for _, r := range got.Reviewers {
			have[fmt.Sprintf("%s:%d", r.Type, r.ID)] = true
		}
		if !maps.Equal(want, have) {
			lost = append(lost, fmt.Sprintf("reviewers (sent %d, got %d; GitHub drops a reviewer who cannot read the repository)", len(want), len(have)))
		}
	}
	if sent.branchPolicy != "" && sent.branchPolicy != got.BranchPolicy {
		lost = append(lost, fmt.Sprintf("deployment_branch_policy (sent %s, got %s)", sent.branchPolicy, got.BranchPolicy))
	}
	if len(lost) > 0 {
		return fmt.Errorf("GitHub did not save %s", strings.Join(lost, ", "))
	}
	return nil
}
