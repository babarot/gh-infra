package repository

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
)

// Branch protection and rulesets in fakeGitHub. Both are replaced as a whole
// by PUT, so what a PUT leaves out is reset here as on GitHub: apply must send
// back everything it does not change, including settings gh-infra does not
// model.

// fakeProtection is a branch's protection, as set by PUT.
type fakeProtection struct {
	StatusChecks  *fakeStatusChecks `json:"status_checks"`
	EnforceAdmins bool              `json:"enforce_admins"`
	Reviews       *fakeReviews      `json:"reviews"`
	Restrictions  *fakeActors       `json:"restrictions"`
	// Optional in the PUT input; left out means false.
	LinearHistory          bool `json:"linear_history"`
	ForcePushes            bool `json:"force_pushes"`
	Deletions              bool `json:"deletions"`
	BlockCreations         bool `json:"block_creations"`
	ConversationResolution bool `json:"conversation_resolution"`
	LockBranch             bool `json:"lock_branch"`
	ForkSyncing            bool `json:"fork_syncing"`
	// Set through its own endpoint; a PUT does not change it.
	Signatures bool `json:"signatures"`
}

type fakeStatusChecks struct {
	Strict bool        `json:"strict"`
	Checks []fakeCheck `json:"checks"`
}

// fakeCheck is a required check. AppID nil means any app.
type fakeCheck struct {
	Context string `json:"context"`
	AppID   *int   `json:"app_id"`
}

type fakeReviews struct {
	Count                 int         `json:"count"`
	DismissStale          bool        `json:"dismiss_stale"`
	CodeOwners            bool        `json:"code_owners"`
	LastPushApproval      bool        `json:"last_push_approval"`
	DismissalRestrictions *fakeActors `json:"dismissal_restrictions"`
	BypassAllowances      *fakeActors `json:"bypass_allowances"`
}

type fakeActors struct {
	Users []string `json:"users"`
	Teams []string `json:"teams"`
	Apps  []string `json:"apps"`
}

// fakeRuleset is a ruleset as GitHub stores it; rules and their parameters
// are kept as sent, so ones gh-infra does not model survive.
type fakeRuleset struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	Target       string           `json:"target"`
	Enforcement  string           `json:"enforcement"`
	SourceType   string           `json:"source_type"`
	BypassActors []map[string]any `json:"bypass_actors"`
	Conditions   map[string]any   `json:"conditions"`
	Rules        []map[string]any `json:"rules"`
	UpdatedAt    int              `json:"updated_at"`
}

// fakeOrg answers the resolver's name lookups.
type fakeOrg struct {
	Teams       map[string]int `json:"teams"`
	Apps        map[string]int `json:"apps"`
	CustomRoles map[string]int `json:"custom_roles"`
	Users       map[string]int `json:"users"`
	// UserOwner: the owner is a user, not an organization. Plan is the
	// owner's plan ("" for Free), readable by an organization owner or the
	// user themselves (AuthUser); PlanHidden: the token cannot read it.
	UserOwner  bool   `json:"user_owner"`
	Plan       string `json:"plan"`
	PlanHidden bool   `json:"plan_hidden"`
	AuthUser   string `json:"auth_user"`
}

// unknownAppID is the app GitHub binds a required check to when the PUT
// names it without an app: the one that reported it last.
const unknownAppID = 999

func (f *fakeGitHub) apiOrg(path, method string) ([]byte, bool, error) {
	if method != "GET" {
		return nil, false, errUnknown
	}
	org := f.s.Org
	if slug, ok := strings.CutPrefix(path, "apps/"); ok {
		if id, ok := org.Apps[slug]; ok {
			return mustJSON(map[string]any{"id": id, "slug": slug}), false, nil
		}
		return nil, false, fakeStatus(gh.ErrNotFound, "app "+slug)
	}
	if path == "orgs/"+f.owner+"/custom-repository-roles" {
		var roles []map[string]any
		for _, name := range fakeSortedKeysInt(org.CustomRoles) {
			roles = append(roles, map[string]any{"id": org.CustomRoles[name], "name": name})
		}
		return mustJSON(map[string]any{"total_count": len(roles), "custom_roles": roles}), false, nil
	}
	if slug, ok := strings.CutPrefix(path, "orgs/"+f.owner+"/teams/"); ok {
		if id, ok := org.Teams[slug]; ok {
			return mustJSON(map[string]any{"id": id, "slug": slug}), false, nil
		}
		return nil, false, fakeStatus(gh.ErrNotFound, "team "+slug)
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) apiBranches(sub, method string, body []byte, rest []string) ([]byte, bool, error) {
	if sub == "branches?protected=true&per_page=100" {
		if method != "GET" || !slices.Contains(rest, "--paginate") {
			return nil, false, errUnknown
		}
		if jq, _ := fakeFlag(rest, "--jq"); jq != ".[].name" {
			return nil, false, errUnknown
		}
		var names []string
		for _, b := range f.s.Branches {
			if f.s.Protections[b] != nil {
				names = append(names, b)
			}
		}
		return fakeLines(names), false, nil
	}
	branch, ok := strings.CutSuffix(strings.TrimPrefix(sub, "branches/"), "/protection")
	if !ok || !slices.Contains(f.s.Branches, branch) {
		return nil, false, fakeStatus(gh.ErrNotFound, "branch "+branch)
	}
	switch method {
	case "GET":
		p := f.s.Protections[branch]
		if p == nil {
			return nil, false, fakeStatus(gh.ErrNotFound, "branch not protected")
		}
		return mustJSON(protectionResponse(p)), false, nil
	case "PUT":
		p, err := parseProtectionPut(body)
		if err != nil {
			return nil, false, err
		}
		if old := f.s.Protections[branch]; old != nil {
			p.Signatures = old.Signatures
		}
		f.s.Protections[branch] = p
		return mustJSON(protectionResponse(p)), true, nil
	case "DELETE":
		if f.s.Protections[branch] == nil {
			return nil, false, fakeStatus(gh.ErrNotFound, "branch not protected")
		}
		delete(f.s.Protections, branch)
		return nil, true, nil
	}
	return nil, false, errUnknown
}

func enabled(v bool) map[string]any { return map[string]any{"enabled": v} }

func actorsResponse(a *fakeActors) map[string]any {
	users, teams, apps := []map[string]string{}, []map[string]string{}, []map[string]string{}
	for _, u := range a.Users {
		users = append(users, map[string]string{"login": u})
	}
	for _, t := range a.Teams {
		teams = append(teams, map[string]string{"slug": t})
	}
	for _, s := range a.Apps {
		apps = append(apps, map[string]string{"slug": s})
	}
	return map[string]any{"users": users, "teams": teams, "apps": apps}
}

// protectionResponse renders p in the shape of the GET response: settings
// are {enabled} objects, and actors are objects.
func protectionResponse(p *fakeProtection) map[string]any {
	out := map[string]any{
		"enforce_admins":                   enabled(p.EnforceAdmins),
		"required_linear_history":          enabled(p.LinearHistory),
		"allow_force_pushes":               enabled(p.ForcePushes),
		"allow_deletions":                  enabled(p.Deletions),
		"block_creations":                  enabled(p.BlockCreations),
		"required_conversation_resolution": enabled(p.ConversationResolution),
		"lock_branch":                      enabled(p.LockBranch),
		"allow_fork_syncing":               enabled(p.ForkSyncing),
		"required_signatures":              enabled(p.Signatures),
	}
	if sc := p.StatusChecks; sc != nil {
		contexts := []string{}
		checks := []map[string]any{}
		for _, c := range sc.Checks {
			contexts = append(contexts, c.Context)
			checks = append(checks, map[string]any{"context": c.Context, "app_id": c.AppID})
		}
		out["required_status_checks"] = map[string]any{"strict": sc.Strict, "contexts": contexts, "checks": checks}
	}
	if r := p.Reviews; r != nil {
		reviews := map[string]any{
			"required_approving_review_count": r.Count,
			"dismiss_stale_reviews":           r.DismissStale,
			"require_code_owner_reviews":      r.CodeOwners,
			"require_last_push_approval":      r.LastPushApproval,
		}
		if r.DismissalRestrictions != nil {
			reviews["dismissal_restrictions"] = actorsResponse(r.DismissalRestrictions)
		}
		if r.BypassAllowances != nil {
			reviews["bypass_pull_request_allowances"] = actorsResponse(r.BypassAllowances)
		}
		out["required_pull_request_reviews"] = reviews
	}
	if p.Restrictions != nil {
		out["restrictions"] = actorsResponse(p.Restrictions)
	}
	return out
}

// parseActors reads the PUT form of actors: all three lists are required.
func parseActors(v any) (*fakeActors, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fakeStatus(gh.ErrValidation, "actors must be an object")
	}
	var a fakeActors
	for key, dst := range map[string]*[]string{"users": &a.Users, "teams": &a.Teams, "apps": &a.Apps} {
		list, ok := m[key].([]any)
		if !ok {
			return nil, fakeStatus(gh.ErrValidation, key+" is required")
		}
		*dst = []string{}
		for _, x := range list {
			s, ok := x.(string)
			if !ok {
				return nil, fakeStatus(gh.ErrValidation, key+" must be logins or slugs")
			}
			*dst = append(*dst, s)
		}
	}
	return &a, nil
}

// parseProtectionPut reads a PUT branch protection body as GitHub documents
// it: four keys are required (null allowed), contexts and checks are not
// accepted together, and an optional setting left out is false.
func parseProtectionPut(body []byte) (*fakeProtection, error) {
	var in map[string]any
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, errUnknown
	}
	for _, key := range []string{"required_status_checks", "enforce_admins", "required_pull_request_reviews", "restrictions"} {
		if _, ok := in[key]; !ok {
			return nil, fakeStatus(gh.ErrValidation, key+" is required")
		}
	}
	p := &fakeProtection{}
	var err error
	boolOf := func(key string) bool {
		v, ok := in[key]
		if !ok || v == nil {
			return false
		}
		b, isBool := v.(bool)
		if !isBool {
			err = fakeStatus(gh.ErrValidation, key+" is not a boolean")
		}
		return b
	}
	p.EnforceAdmins = boolOf("enforce_admins")
	p.LinearHistory = boolOf("required_linear_history")
	p.ForcePushes = boolOf("allow_force_pushes")
	p.Deletions = boolOf("allow_deletions")
	p.BlockCreations = boolOf("block_creations")
	p.ConversationResolution = boolOf("required_conversation_resolution")
	p.LockBranch = boolOf("lock_branch")
	p.ForkSyncing = boolOf("allow_fork_syncing")
	if err != nil {
		return nil, err
	}

	if v := in["required_status_checks"]; v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fakeStatus(gh.ErrValidation, "required_status_checks must be an object")
		}
		strict, _ := m["strict"].(bool)
		sc := &fakeStatusChecks{Strict: strict, Checks: []fakeCheck{}}
		contexts, hasContexts := m["contexts"]
		checks, hasChecks := m["checks"]
		if hasContexts && hasChecks {
			return nil, fakeStatus(gh.ErrValidation, "contexts and checks cannot both be set")
		}
		if hasContexts {
			list, _ := contexts.([]any)
			for _, c := range list {
				s, _ := c.(string)
				sc.Checks = append(sc.Checks, fakeCheck{Context: s})
			}
		}
		if hasChecks {
			list, _ := checks.([]any)
			for _, c := range list {
				cm, _ := c.(map[string]any)
				s, _ := cm["context"].(string)
				check := fakeCheck{Context: s}
				switch id, ok := cm["app_id"].(float64); {
				case !ok:
					// Left out, the check is bound to the app that reported it last.
					app := unknownAppID
					check.AppID = &app
				case id == -1:
					// -1 means any app.
				default:
					app := int(id)
					check.AppID = &app
				}
				sc.Checks = append(sc.Checks, check)
			}
		}
		p.StatusChecks = sc
	}

	if v := in["required_pull_request_reviews"]; v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fakeStatus(gh.ErrValidation, "required_pull_request_reviews must be an object")
		}
		r := &fakeReviews{}
		if n, ok := m["required_approving_review_count"].(float64); ok {
			r.Count = int(n)
		}
		r.DismissStale, _ = m["dismiss_stale_reviews"].(bool)
		r.CodeOwners, _ = m["require_code_owner_reviews"].(bool)
		r.LastPushApproval, _ = m["require_last_push_approval"].(bool)
		if a, ok := m["dismissal_restrictions"]; ok {
			if r.DismissalRestrictions, err = parseActors(a); err != nil {
				return nil, err
			}
		}
		if a, ok := m["bypass_pull_request_allowances"]; ok {
			if r.BypassAllowances, err = parseActors(a); err != nil {
				return nil, err
			}
		}
		p.Reviews = r
	}

	if v := in["restrictions"]; v != nil {
		if p.Restrictions, err = parseActors(v); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// rulesetReadOnly are the fields GitHub sets on a ruleset; a PUT that sends
// them back is not rejected here (unverified), they are ignored.
var rulesetReadOnly = []string{"id", "node_id", "source", "source_type", "_links", "created_at", "updated_at", "current_user_can_bypass"}

func (f *fakeGitHub) rulesetResponse(rs *fakeRuleset, full bool) map[string]any {
	out := map[string]any{
		"id":          rs.ID,
		"name":        rs.Name,
		"target":      rs.Target,
		"enforcement": rs.Enforcement,
		"source_type": rs.SourceType,
		"source":      f.fullName(),
		"node_id":     fmt.Sprintf("RRS_%d", rs.ID),
		"_links":      map[string]any{"self": map[string]string{"href": "https://api.github.com/repos/" + f.fullName() + "/rulesets/" + strconv.Itoa(rs.ID)}},
		"created_at":  "2025-01-01T00:00:00Z",
		"updated_at":  fmt.Sprintf("2025-01-01T00:00:%02dZ", rs.UpdatedAt),
	}
	if rs.SourceType == "Organization" {
		out["source"] = f.owner
	}
	if full {
		out["bypass_actors"] = rs.BypassActors
		out["conditions"] = rs.Conditions
		out["rules"] = rs.Rules
		out["current_user_can_bypass"] = "always"
	}
	return out
}

func (f *fakeGitHub) findRuleset(id int) *fakeRuleset {
	for _, rs := range f.s.Rulesets {
		if rs.ID == id {
			return rs
		}
	}
	return nil
}

func (f *fakeGitHub) apiRulesets(sub, method string, body []byte) ([]byte, bool, error) {
	if sub == "rulesets" {
		switch method {
		case "GET":
			list := []map[string]any{}
			for _, rs := range f.s.Rulesets {
				list = append(list, f.rulesetResponse(rs, false))
			}
			return mustJSON(list), false, nil
		case "POST":
			rs, err := parseRuleset(body)
			if err != nil {
				return nil, false, err
			}
			f.s.NextRulesetID++
			rs.ID = f.s.NextRulesetID
			rs.SourceType = "Repository"
			f.s.Rulesets = append(f.s.Rulesets, rs)
			return mustJSON(f.rulesetResponse(rs, true)), true, nil
		}
		return nil, false, errUnknown
	}
	id, err := strconv.Atoi(strings.TrimPrefix(sub, "rulesets/"))
	if err != nil {
		return nil, false, errUnknown
	}
	rs := f.findRuleset(id)
	if rs == nil {
		return nil, false, fakeStatus(gh.ErrNotFound, "ruleset "+strconv.Itoa(id))
	}
	if method == "GET" {
		return mustJSON(f.rulesetResponse(rs, true)), false, nil
	}
	// An organization ruleset is not changed through a repository.
	if rs.SourceType != "Repository" {
		return nil, false, fakeStatus(gh.ErrNotFound, "ruleset "+strconv.Itoa(id)+" is not a repository ruleset")
	}
	switch method {
	case "PUT":
		next, err := parseRuleset(body)
		if err != nil {
			return nil, false, err
		}
		next.ID, next.SourceType, next.UpdatedAt = rs.ID, rs.SourceType, rs.UpdatedAt+1
		*rs = *next
		return mustJSON(f.rulesetResponse(rs, true)), true, nil
	case "DELETE":
		f.s.Rulesets = slices.DeleteFunc(f.s.Rulesets, func(x *fakeRuleset) bool { return x.ID == id })
		return nil, true, nil
	}
	return nil, false, errUnknown
}

// parseRuleset reads a POST or PUT ruleset body. Both replace the whole
// ruleset. GitHub fills defaults for what is left out, and requires the
// parameters of the rules it knows.
func parseRuleset(body []byte) (*fakeRuleset, error) {
	var in map[string]any
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, errUnknown
	}
	for _, k := range rulesetReadOnly {
		delete(in, k)
	}
	rs := &fakeRuleset{Target: "branch", BypassActors: []map[string]any{}, Rules: []map[string]any{}}
	var ok bool
	if rs.Name, ok = in["name"].(string); !ok || rs.Name == "" {
		return nil, fakeStatus(gh.ErrValidation, "name is required")
	}
	if rs.Enforcement, ok = in["enforcement"].(string); !ok {
		return nil, fakeStatus(gh.ErrValidation, "enforcement is required")
	}
	if t, ok := in["target"].(string); ok {
		rs.Target = t
	}
	for key := range in {
		switch key {
		case "name", "target", "enforcement", "bypass_actors", "conditions", "rules":
		default:
			return nil, fakeStatus(gh.ErrValidation, "unknown field "+key)
		}
	}
	if list, ok := in["bypass_actors"].([]any); ok {
		for _, a := range list {
			m, ok := a.(map[string]any)
			if !ok {
				return nil, fakeStatus(gh.ErrValidation, "bypass actor must be an object")
			}
			rs.BypassActors = append(rs.BypassActors, m)
		}
	}
	if c, ok := in["conditions"].(map[string]any); ok {
		if ref, ok := c["ref_name"].(map[string]any); ok {
			if _, ok := ref["exclude"]; !ok {
				ref["exclude"] = []any{}
			}
		}
		rs.Conditions = c
	}
	if list, ok := in["rules"].([]any); ok {
		for _, r := range list {
			rule, ok := r.(map[string]any)
			if !ok {
				return nil, fakeStatus(gh.ErrValidation, "rule must be an object")
			}
			if err := fillRuleDefaults(rule); err != nil {
				return nil, err
			}
			rs.Rules = append(rs.Rules, rule)
		}
	}
	return rs, nil
}

func fillRuleDefaults(rule map[string]any) error {
	params, _ := rule["parameters"].(map[string]any)
	require := func(keys ...string) error {
		for _, k := range keys {
			if _, ok := params[k]; !ok {
				return fakeStatus(gh.ErrValidation, fmt.Sprintf("%v rule requires %s", rule["type"], k))
			}
		}
		return nil
	}
	switch rule["type"] {
	case "pull_request":
		if err := require("required_approving_review_count", "dismiss_stale_reviews_on_push", "require_code_owner_review", "require_last_push_approval", "required_review_thread_resolution"); err != nil {
			return err
		}
		if _, ok := params["allowed_merge_methods"]; !ok {
			params["allowed_merge_methods"] = []any{"merge", "squash", "rebase"}
		}
	case "required_status_checks":
		if err := require("required_status_checks", "strict_required_status_checks_policy"); err != nil {
			return err
		}
		if _, ok := params["do_not_enforce_on_create"]; !ok {
			params["do_not_enforce_on_create"] = false
		}
	}
	return nil
}

func fakeSortedKeysInt(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
