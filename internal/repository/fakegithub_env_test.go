package repository

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// The environments API, as seen on GitHub (2026-10, with throwaway
// repositories): a PUT keeps the fields it leaves out; reviewers [] or null
// deletes the reviewers rule, and with it prevent_self_review, which a new
// rule starts as false; prevent_self_review without a reviewers rule is a
// 422; a reviewer who cannot read the repository is dropped without an error;
// wait_timer 0 deletes the wait timer rule; a policy that stops being custom
// loses its patterns; a PUT does not rename an environment whose name differs
// in case; and a private repository on a plan other than Enterprise rejects
// wait timers and reviewers.

type fakeEnvironment struct {
	Name              string            `json:"name"`
	CanAdminsBypass   bool              `json:"can_admins_bypass"`
	WaitTimer         int               `json:"wait_timer"`
	ReviewersRule     bool              `json:"reviewers_rule"`
	PreventSelfReview bool              `json:"prevent_self_review"`
	Reviewers         []fakeReviewer    `json:"reviewers"`
	Policy            string            `json:"policy"` // all, protected or custom
	Rules             []fakeBranchRule  `json:"rules"`
	Secrets           map[string]string `json:"secrets"` // GitHub returns names only
	Variables         map[string]string `json:"variables"`
}

type fakeReviewer struct {
	Type string `json:"type"`
	ID   int    `json:"id"`
}

type fakeBranchRule struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// newFakeEnvironment is an environment as GitHub creates it.
func newFakeEnvironment(name string) *fakeEnvironment {
	return &fakeEnvironment{Name: name, CanAdminsBypass: true, Policy: manifest.DeploymentBranchesAll}
}

func (f *fakeGitHub) findEnvironment(name string) *fakeEnvironment {
	for _, e := range f.s.Environments {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return nil
}

func (f *fakeGitHub) userLogin(id int) string {
	for login, uid := range f.s.Org.Users {
		if uid == id {
			return login
		}
	}
	return ""
}

func (f *fakeGitHub) teamSlug(id int) string {
	for slug, tid := range f.s.Org.Teams {
		if tid == id {
			return slug
		}
	}
	return ""
}

// canRead reports whether a reviewer can read the repository.
func (f *fakeGitHub) canRead(r fakeReviewer) bool {
	if r.Type == manifest.EnvironmentReviewerTeam {
		return slices.Contains(f.s.TeamRepos, f.teamSlug(r.ID))
	}
	login := f.userLogin(r.ID)
	return strings.EqualFold(login, f.owner) || slices.ContainsFunc(f.s.Collaborators, func(c string) bool { return strings.EqualFold(c, login) })
}

func (f *fakeGitHub) renderEnvironment(e *fakeEnvironment) map[string]any {
	var rules []map[string]any
	if e.ReviewersRule {
		reviewers := []map[string]any{}
		for _, r := range e.Reviewers {
			reviewer := map[string]any{"id": r.ID}
			if r.Type == manifest.EnvironmentReviewerTeam {
				reviewer["slug"] = f.teamSlug(r.ID)
			} else {
				reviewer["login"] = f.userLogin(r.ID)
			}
			reviewers = append(reviewers, map[string]any{"type": r.Type, "reviewer": reviewer})
		}
		rules = append(rules, map[string]any{"type": "required_reviewers", "prevent_self_review": e.PreventSelfReview, "reviewers": reviewers})
	}
	if e.WaitTimer > 0 {
		rules = append(rules, map[string]any{"type": "wait_timer", "wait_timer": e.WaitTimer})
	}
	var policy any
	if e.Policy != manifest.DeploymentBranchesAll {
		rules = append(rules, map[string]any{"type": "branch_policy"})
		policy = map[string]bool{
			"protected_branches":     e.Policy == manifest.DeploymentBranchesProtected,
			"custom_branch_policies": e.Policy == manifest.DeploymentBranchesCustom,
		}
	}
	if rules == nil {
		rules = []map[string]any{}
	}
	return map[string]any{
		"name":                     e.Name,
		"can_admins_bypass":        e.CanAdminsBypass,
		"protection_rules":         rules,
		"deployment_branch_policy": policy,
	}
}

func (f *fakeGitHub) apiEnvironments(sub, method string, body []byte, rest []string) ([]byte, bool, error) {
	if sub == "environments" {
		if method != "GET" || !slices.Contains(rest, "--paginate") || !slices.Contains(rest, "--slurp") {
			return nil, false, errUnknown
		}
		envs := []map[string]any{}
		for _, e := range f.s.Environments {
			envs = append(envs, f.renderEnvironment(e))
		}
		return mustJSON([]map[string]any{{"total_count": len(envs), "environments": envs}}), false, nil
	}

	escaped, policies, hasPolicies := strings.Cut(strings.TrimPrefix(sub, "environments/"), "/")
	name, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, false, errUnknown
	}
	if hasPolicies {
		return f.apiBranchPolicies(name, policies, method, rest)
	}
	if method != "PUT" {
		return nil, false, errUnknown
	}

	var p map[string]json.RawMessage
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, false, err
	}
	private := f.s.Repo.Visibility == manifest.VisibilityPrivate && f.s.Org.Plan != "enterprise"
	e := f.findEnvironment(name)
	isNew := e == nil
	if isNew {
		e = newFakeEnvironment(name)
	} else {
		c := *e
		e = &c
	}
	for k, v := range p {
		switch k {
		case "wait_timer":
			if err := json.Unmarshal(v, &e.WaitTimer); err != nil {
				return nil, false, err
			}
			if private && e.WaitTimer > 0 {
				return nil, false, fakeStatus(gh.ErrValidation, "Please ensure the billing plan supports the wait timer protection rule")
			}
		case "can_admins_bypass":
			if err := json.Unmarshal(v, &e.CanAdminsBypass); err != nil {
				return nil, false, err
			}
		case "reviewers", "prevent_self_review", "deployment_branch_policy":
		default:
			return nil, false, errUnknown
		}
	}
	if v, ok := p["reviewers"]; ok {
		var rvs []fakeReviewer
		if err := json.Unmarshal(v, &rvs); err != nil {
			return nil, false, err
		}
		if len(rvs) == 0 {
			e.ReviewersRule, e.PreventSelfReview, e.Reviewers = false, false, nil
		} else {
			if private {
				return nil, false, fakeStatus(gh.ErrValidation, "Please ensure the billing plan supports the required reviewers protection rule")
			}
			if !e.ReviewersRule {
				e.ReviewersRule, e.PreventSelfReview = true, false
			}
			e.Reviewers = nil
			for _, r := range rvs {
				if f.canRead(r) {
					e.Reviewers = append(e.Reviewers, r)
				}
			}
		}
	}
	if v, ok := p["prevent_self_review"]; ok {
		if !e.ReviewersRule {
			return nil, false, fakeStatus(gh.ErrValidation, "Required reviewers must have at least one reviewer to set prevent_self_review")
		}
		if err := json.Unmarshal(v, &e.PreventSelfReview); err != nil {
			return nil, false, err
		}
	}
	if v, ok := p["deployment_branch_policy"]; ok {
		var policy *struct {
			ProtectedBranches    bool `json:"protected_branches"`
			CustomBranchPolicies bool `json:"custom_branch_policies"`
		}
		if err := json.Unmarshal(v, &policy); err != nil {
			return nil, false, err
		}
		switch {
		case policy == nil:
			e.Policy, e.Rules = manifest.DeploymentBranchesAll, nil
		case policy.ProtectedBranches == policy.CustomBranchPolicies:
			return nil, false, fakeStatus(gh.ErrValidation, "protected_branches and custom_branch_policies must differ")
		case policy.ProtectedBranches:
			e.Policy, e.Rules = manifest.DeploymentBranchesProtected, nil
		default:
			if e.Policy != manifest.DeploymentBranchesCustom {
				e.Rules = nil
			}
			e.Policy = manifest.DeploymentBranchesCustom
		}
	}
	if isNew {
		f.s.Environments = append(f.s.Environments, e)
	} else {
		*f.findEnvironment(name) = *e
	}
	return mustJSON(f.renderEnvironment(e)), true, nil
}

// apiBranchPolicies answers the name patterns of an environment, which exist
// only while its policy is custom.
func (f *fakeGitHub) apiBranchPolicies(env, sub, method string, rest []string) ([]byte, bool, error) {
	e := f.findEnvironment(env)
	if e == nil || e.Policy != manifest.DeploymentBranchesCustom {
		return nil, false, fakeStatus(gh.ErrNotFound, "Not Found")
	}
	switch {
	case sub == "deployment-branch-policies" && method == "GET":
		if !slices.Contains(rest, "--paginate") || !slices.Contains(rest, "--slurp") {
			return nil, false, errUnknown
		}
		rules := e.Rules
		if rules == nil {
			rules = []fakeBranchRule{}
		}
		return mustJSON([]map[string]any{{"total_count": len(rules), "branch_policies": rules}}), false, nil
	case sub == "deployment-branch-policies" && method == "POST":
		fields := fakeFields(rest)
		r := fakeBranchRule{Name: fields["name"], Type: fields["type"]}
		if r.Name == "" || len(fields) != 2 {
			return nil, false, errUnknown
		}
		// The same pattern again does not make a second one.
		for _, x := range e.Rules {
			if x.Name == r.Name && x.Type == r.Type {
				return mustJSON(x), false, nil
			}
		}
		f.s.NextBranchRuleID++
		r.ID = f.s.NextBranchRuleID
		e.Rules = append(e.Rules, r)
		return mustJSON(r), true, nil
	case strings.HasPrefix(sub, "deployment-branch-policies/") && method == "DELETE":
		id, err := strconv.Atoi(strings.TrimPrefix(sub, "deployment-branch-policies/"))
		if err != nil {
			return nil, false, errUnknown
		}
		i := slices.IndexFunc(e.Rules, func(r fakeBranchRule) bool { return r.ID == id })
		if i < 0 {
			return nil, false, fakeStatus(gh.ErrNotFound, "Not Found")
		}
		e.Rules = slices.Delete(e.Rules, i, i+1)
		return nil, true, nil
	}
	return nil, false, errUnknown
}

// apiCollaborator answers whether a user can read the repository.
func (f *fakeGitHub) apiCollaborator(login, method string) ([]byte, bool, error) {
	if method != "GET" {
		return nil, false, errUnknown
	}
	if strings.EqualFold(login, f.owner) || slices.ContainsFunc(f.s.Collaborators, func(c string) bool { return strings.EqualFold(c, login) }) {
		return nil, false, nil
	}
	return nil, false, fakeStatus(gh.ErrNotFound, "Not Found")
}

// apiAccount answers users/{login}, user, and the plan of orgs/{owner}.
func (f *fakeGitHub) apiAccount(path, method string) ([]byte, bool, bool, error) {
	if method != "GET" {
		return nil, false, false, nil
	}
	org := f.s.Org
	plan := func() any {
		if org.PlanHidden {
			return nil
		}
		name := org.Plan
		if name == "" {
			name = "free"
		}
		return map[string]any{"name": name}
	}
	switch {
	case strings.HasPrefix(path, "users/"):
		login := strings.TrimPrefix(path, "users/")
		for l, id := range org.Users {
			if strings.EqualFold(l, login) {
				return mustJSON(map[string]any{"login": l, "id": id}), false, true, nil
			}
		}
		return nil, false, true, fakeStatus(gh.ErrNotFound, "user "+login)
	case path == "user":
		resp := map[string]any{"login": org.AuthUser}
		if org.UserOwner && strings.EqualFold(org.AuthUser, f.owner) {
			resp["plan"] = plan()
		}
		return mustJSON(resp), false, true, nil
	case path == "orgs/"+f.owner:
		if org.UserOwner {
			return nil, false, true, fakeStatus(gh.ErrNotFound, "Not Found")
		}
		resp := map[string]any{"login": f.owner}
		if p := plan(); p != nil {
			resp["plan"] = p
		}
		return mustJSON(resp), false, true, nil
	case strings.HasPrefix(path, "orgs/"+f.owner+"/teams/") && strings.HasSuffix(path, "/repos/"+f.fullName()):
		slug := strings.TrimSuffix(strings.TrimPrefix(path, "orgs/"+f.owner+"/teams/"), "/repos/"+f.fullName())
		if slices.Contains(f.s.TeamRepos, slug) {
			return nil, false, true, nil
		}
		return nil, false, true, fakeStatus(gh.ErrNotFound, "Not Found")
	}
	return nil, false, false, nil
}

// envValues returns the secrets or variables a gh secret / gh variable
// command reads or writes: the repository's, or with --env, those of the
// environment, which must exist.
func (f *fakeGitHub) envValues(args []string, repo map[string]string, pick func(*fakeEnvironment) *map[string]string) (map[string]string, error) {
	name, ok := fakeFlag(args, "--env")
	if !ok {
		return repo, nil
	}
	e := f.findEnvironment(name)
	if e == nil {
		return nil, fakeStatus(gh.ErrNotFound, "environment "+name)
	}
	m := pick(e)
	if *m == nil {
		*m = map[string]string{}
	}
	return *m, nil
}

// fakeFields returns the -f key=value fields of a gh api command.
func fakeFields(args []string) map[string]string {
	fields := map[string]string{}
	for i, a := range args {
		if (a == "-f" || a == "--raw-field") && i+1 < len(args) {
			k, v, _ := strings.Cut(args[i+1], "=")
			fields[k] = v
		}
	}
	return fields
}

// flattenEnvironments lists environments keyed by lowercase name, as GitHub
// matches them.
func flattenEnvironments(out map[string]string, envs []*fakeEnvironment) {
	for _, e := range envs {
		key := "environments." + strings.ToLower(e.Name) + "."
		out[key+"name"] = e.Name
		out[key+"can_admins_bypass"] = fmt.Sprint(e.CanAdminsBypass)
		out[key+"wait_timer"] = fmt.Sprint(e.WaitTimer)
		out[key+"reviewers_rule"] = fmt.Sprint(e.ReviewersRule)
		out[key+"prevent_self_review"] = fmt.Sprint(e.PreventSelfReview)
		out[key+"policy"] = e.Policy
		var rvs []string
		for _, r := range e.Reviewers {
			rvs = append(rvs, fmt.Sprintf("%s:%d", r.Type, r.ID))
		}
		slices.Sort(rvs)
		out[key+"reviewers"] = strings.Join(rvs, ",")
		for _, r := range e.Rules {
			out[key+"rules."+r.Type+":"+r.Name] = "1"
		}
		for n, v := range e.Secrets {
			out[key+"secrets."+n] = v
		}
		for n, v := range e.Variables {
			out[key+"variables."+n] = v
		}
	}
}
