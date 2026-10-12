package repository

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// Flattening and the plan-to-state table for branch protection and
// rulesets. An object (a protection, its reviews or status checks, a
// ruleset, a rule) is listed as "<path>" = "present", with its fields under
// "<path>.". The read-only fields GitHub sets on a ruleset (id, updated_at,
// ...) are left out: they change on every update without being planned.

const present = "present"

func canonicalJSON(v any) string {
	if list, ok := v.([]any); ok {
		items := make([]string, 0, len(list))
		for _, x := range list {
			items = append(items, canonicalJSON(x))
		}
		slices.Sort(items)
		return "[" + strings.Join(items, ",") + "]"
	}
	data, _ := json.Marshal(v)
	return string(data)
}

func flattenProtections(out map[string]string, protections map[string]*fakeProtection) {
	for branch, p := range protections {
		base := "branch_protection." + branch
		out[base] = present
		for k, v := range map[string]bool{
			"enforce_admins":                   p.EnforceAdmins,
			"required_linear_history":          p.LinearHistory,
			"allow_force_pushes":               p.ForcePushes,
			"allow_deletions":                  p.Deletions,
			"block_creations":                  p.BlockCreations,
			"required_conversation_resolution": p.ConversationResolution,
			"lock_branch":                      p.LockBranch,
			"allow_fork_syncing":               p.ForkSyncing,
			"required_signatures":              p.Signatures,
		} {
			out[base+"."+k] = fmt.Sprint(v)
		}
		if sc := p.StatusChecks; sc != nil {
			out[base+".required_status_checks"] = present
			out[base+".required_status_checks.strict"] = fmt.Sprint(sc.Strict)
			for _, c := range sc.Checks {
				app := "any"
				if c.AppID != nil {
					app = fmt.Sprint(*c.AppID)
				}
				out[base+".required_status_checks.checks."+c.Context] = app
			}
		}
		if r := p.Reviews; r != nil {
			rb := base + ".required_pull_request_reviews"
			out[rb] = present
			out[rb+".required_approving_review_count"] = fmt.Sprint(r.Count)
			out[rb+".dismiss_stale_reviews"] = fmt.Sprint(r.DismissStale)
			out[rb+".require_code_owner_reviews"] = fmt.Sprint(r.CodeOwners)
			out[rb+".require_last_push_approval"] = fmt.Sprint(r.LastPushApproval)
			if r.DismissalRestrictions != nil {
				out[rb+".dismissal_restrictions"] = canonicalJSON(r.DismissalRestrictions)
			}
			if r.BypassAllowances != nil {
				out[rb+".bypass_pull_request_allowances"] = canonicalJSON(r.BypassAllowances)
			}
		}
		if p.Restrictions != nil {
			out[base+".restrictions"] = canonicalJSON(p.Restrictions)
		}
	}
}

func flattenRulesets(out map[string]string, rulesets []*fakeRuleset) {
	for _, rs := range rulesets {
		base := "rulesets." + rs.Name
		out[base] = present
		out[base+".target"] = rs.Target
		out[base+".enforcement"] = rs.Enforcement
		actors := make([]any, 0, len(rs.BypassActors))
		for _, a := range rs.BypassActors {
			actors = append(actors, a)
		}
		out[base+".bypass_actors"] = canonicalJSON(actors)
		if rs.Conditions != nil {
			out[base+".conditions"] = canonicalJSON(rs.Conditions)
		}
		for _, rule := range rs.Rules {
			rb := fmt.Sprintf("%s.rules.%v", base, rule["type"])
			out[rb] = present
			params, _ := rule["parameters"].(map[string]any)
			for k, v := range params {
				out[rb+"."+k] = canonicalJSON(v)
			}
		}
	}
}

// branchProtectionPaths maps a planned branch protection field to its state.
var branchProtectionPaths = map[string]string{
	"required_reviews":               ".required_pull_request_reviews.required_approving_review_count",
	"dismiss_stale_reviews":          ".required_pull_request_reviews.dismiss_stale_reviews",
	"require_code_owner_reviews":     ".required_pull_request_reviews.require_code_owner_reviews",
	"enforce_admins":                 ".enforce_admins",
	"allow_force_pushes":             ".allow_force_pushes",
	"allow_deletions":                ".allow_deletions",
	"restrict_pushes":                ".restrictions",
	"require_status_checks.strict":   ".required_status_checks.strict",
	"require_status_checks.contexts": ".required_status_checks.checks.",
}

// rulesetPaths maps a planned ruleset field, other than adding or removing
// a rule, to its state.
var rulesetPaths = map[string]string{
	"enforcement":                           ".enforcement",
	"target":                                ".target",
	"bypass_actors":                         ".bypass_actors",
	"conditions":                            ".conditions",
	"rules.update.allows_fetch_and_merge":   ".rules.update.update_allows_fetch_and_merge",
	"rules.required_status_checks.strict":   ".rules.required_status_checks.strict_required_status_checks_policy",
	"rules.required_status_checks.contexts": ".rules.required_status_checks.required_status_checks",
}

// addKeyedPaths adds the state a branch protection or ruleset change may
// change. Its fields are in Details. A create or delete covers the whole
// object.
func addKeyedPaths(set pathSet, resource, key string, c Change) {
	var base string
	switch resource {
	case manifest.ResourceBranchProtection:
		base = "branch_protection." + key
	case manifest.ResourceRuleset:
		base = "rulesets." + key
	default:
		return
	}
	if c.Type == ChangeCreate || c.Type == ChangeDelete {
		set[base] = true
		set[base+"."] = true
		return
	}
	for _, d := range c.Details {
		if resource == manifest.ResourceBranchProtection {
			if p, ok := branchProtectionPaths[d.Field]; ok {
				set[base+p] = true
			}
			continue
		}
		if p, ok := rulesetPaths[d.Field]; ok {
			set[base+p] = true
			continue
		}
		rule, param, hasParam := strings.Cut(strings.TrimPrefix(d.Field, "rules."), ".")
		if !strings.HasPrefix(d.Field, "rules.") {
			continue
		}
		if hasParam {
			set[base+".rules."+rule+"."+param] = true
			continue
		}
		// A rule added or removed.
		set[base+".rules."+rule] = true
		set[base+".rules."+rule+"."] = true
	}
}

// coversNewObject reports whether path is inside an object that apply added
// or removed for a planned change: GitHub fills the other fields of a new
// object with its defaults, and a removed one takes its fields along.
func (s pathSet) coversNewObject(path string, before, after map[string]string) bool {
	for obj := path; ; {
		if (before[obj] == present) != (after[obj] == present) && s.touches(obj) {
			return true
		}
		i := strings.LastIndex(obj, ".")
		if i < 0 {
			return false
		}
		obj = obj[:i]
	}
}

// touches reports whether a planned change covers obj or something in it.
func (s pathSet) touches(obj string) bool {
	for p := range s {
		if p == obj || strings.HasPrefix(p, obj+".") || (strings.HasSuffix(p, ".") && strings.HasPrefix(obj+".", p)) {
			return true
		}
	}
	return false
}

// propRulesState extends propState with protected branches and rulesets,
// each with settings gh-infra does not model.
func propRulesState() fakeState {
	s := propState()
	app7, app8 := 7, 8
	s.Branches = []string{"main", "develop", "release"}
	s.Protections = map[string]*fakeProtection{
		"main": {
			StatusChecks:  &fakeStatusChecks{Strict: true, Checks: []fakeCheck{{Context: "ci", AppID: &app7}, {Context: "lint"}}},
			EnforceAdmins: true,
			Reviews: &fakeReviews{Count: 1, DismissStale: true, LastPushApproval: true,
				DismissalRestrictions: &fakeActors{Users: []string{"alice"}, Teams: []string{}, Apps: []string{}},
				BypassAllowances:      &fakeActors{Users: []string{}, Teams: []string{}, Apps: []string{"ci"}}},
			Restrictions:  &fakeActors{Users: []string{}, Teams: []string{"platform"}, Apps: []string{}},
			LinearHistory: true, ConversationResolution: true, ForkSyncing: true, Signatures: true,
		},
		"develop": {
			StatusChecks:   &fakeStatusChecks{Strict: false, Checks: []fakeCheck{{Context: "build", AppID: &app8}}},
			BlockCreations: true,
		},
	}
	s.Rulesets = []*fakeRuleset{
		{ID: 1, Name: "protect-main", Target: "branch", Enforcement: "active", SourceType: "Repository",
			BypassActors: []map[string]any{{"actor_id": 11, "actor_type": "Team", "bypass_mode": "always"}},
			Conditions:   map[string]any{"ref_name": map[string]any{"include": []any{"~DEFAULT_BRANCH"}, "exclude": []any{}}},
			Rules: []map[string]any{
				{"type": "deletion"},
				{"type": "non_fast_forward"},
				{"type": "pull_request", "parameters": map[string]any{
					"required_approving_review_count": 1, "dismiss_stale_reviews_on_push": true, "require_code_owner_review": false,
					"require_last_push_approval": false, "required_review_thread_resolution": true, "allowed_merge_methods": []any{"squash"},
				}},
				{"type": "required_status_checks", "parameters": map[string]any{
					"required_status_checks":               []any{map[string]any{"context": "ci", "integration_id": 7}},
					"strict_required_status_checks_policy": true, "do_not_enforce_on_create": true,
				}},
				{"type": "merge_queue", "parameters": map[string]any{"merge_method": "SQUASH", "max_entries_to_build": 5}},
			}},
		{ID: 2, Name: "tags", Target: "tag", Enforcement: "active", SourceType: "Repository",
			BypassActors: []map[string]any{},
			Conditions:   map[string]any{"ref_name": map[string]any{"include": []any{"refs/tags/v*"}, "exclude": []any{}}},
			Rules:        []map[string]any{{"type": "creation"}, {"type": "deletion"}}},
		{ID: 3, Name: "org-wide", Target: "branch", Enforcement: "active", SourceType: "Organization",
			BypassActors: []map[string]any{}, Rules: []map[string]any{{"type": "deletion"}}},
	}
	s.Org = fakeOrg{
		Teams:       map[string]int{"platform": 11, "sre": 12},
		Apps:        map[string]int{"ci": 7, "bot": 8},
		CustomRoles: map[string]int{"triage-plus": 99},
	}
	// The JSON round trip gives the numbers the types a GitHub response has.
	var c fakeState
	if err := json.Unmarshal(mustJSON(s), &c); err != nil {
		panic(err)
	}
	return c
}

func TestApplyPropertiesBranchProtectionAndRulesets(t *testing.T) {
	ptr := manifest.Ptr[string]
	bptr := manifest.Ptr[bool]
	iptr := manifest.Ptr[int]
	authoritative := manifest.Ptr(manifest.CollectionReconcileAuthoritative)
	for _, tt := range []struct {
		name   string
		spec   manifest.RepositorySpec
		recon  *manifest.RepositoryReconcile
		noPlan bool
	}{
		{name: "the same as GitHub", noPlan: true, spec: manifest.RepositorySpec{
			BranchProtection: []manifest.BranchProtection{{Pattern: "main", RequiredReviews: iptr(1), EnforceAdmins: bptr(true)}},
			Rulesets:         []manifest.Ruleset{{Name: "protect-main", Enforcement: ptr("active")}},
		}},
		{name: "a new protection", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "release", RequiredReviews: iptr(2), EnforceAdmins: bptr(true), RestrictPushes: bptr(true),
			RequireStatusChecks: &manifest.StatusChecks{Strict: true, Contexts: []string{"ci"}},
		}}}},
		{name: "required reviews only", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "main", RequiredReviews: iptr(2),
		}}}},
		{name: "reviews added to a protection without them", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "develop", RequiredReviews: iptr(1), RequireCodeOwnerReviews: bptr(true),
		}}}},
		{name: "status check contexts", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "main", RequireStatusChecks: &manifest.StatusChecks{Strict: true, Contexts: []string{"ci", "test"}},
		}}}},
		{name: "status checks strict only", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "main", RequireStatusChecks: &manifest.StatusChecks{Strict: false},
		}}}},
		{name: "push restrictions off, force pushes on", spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{
			Pattern: "main", RestrictPushes: bptr(false), AllowForcePushes: bptr(true),
		}}}},
		{name: "branch protection authoritative", recon: &manifest.RepositoryReconcile{BranchProtection: authoritative},
			spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{Pattern: "main", RequiredReviews: iptr(1)}}}},
		{name: "a new ruleset", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "release", Target: ptr("branch"), Enforcement: ptr("evaluate"),
			BypassActors: []manifest.RulesetBypassActor{
				{Team: "platform", BypassMode: "always"}, {App: "ci", BypassMode: "pull_request"},
				{CustomRole: "triage-plus", BypassMode: "always"}, {OrgAdmin: bptr(true), BypassMode: "always"},
				{Role: "write", BypassMode: "pull_request"},
			},
			Conditions: &manifest.RulesetConditions{RefName: &manifest.RulesetRefCondition{Include: []string{"refs/heads/release/*"}}},
			Rules: manifest.RulesetRules{
				PullRequest:          &manifest.RulesetPullRequest{RequiredApprovingReviewCount: iptr(1)},
				RequiredStatusChecks: &manifest.RulesetStatusChecks{Contexts: []manifest.RulesetStatusCheck{{Context: "ci", App: "ci"}}},
				Deletion:             bptr(true),
			},
		}}}},
		{name: "ruleset enforcement only", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Enforcement: ptr("evaluate"),
		}}}},
		{name: "a pull request parameter", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Rules: manifest.RulesetRules{PullRequest: &manifest.RulesetPullRequest{RequiredApprovingReviewCount: iptr(2)}},
		}}}},
		{name: "rules removed", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Rules: manifest.RulesetRules{Deletion: bptr(false), PullRequest: &manifest.RulesetPullRequest{Disabled: true}},
		}}}},
		{name: "a rule added", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Rules: manifest.RulesetRules{
				Update:                &manifest.RulesetUpdate{Enabled: bptr(true), AllowsFetchAndMerge: bptr(true)},
				RequiredLinearHistory: bptr(true),
			},
		}}}},
		{name: "bypass actors replaced", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", BypassActors: []manifest.RulesetBypassActor{{Team: "sre", BypassMode: "always"}}, BypassActorsSet: true,
		}}}},
		{name: "ruleset status checks", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Rules: manifest.RulesetRules{RequiredStatusChecks: &manifest.RulesetStatusChecks{
				StrictRequiredStatusChecksPolicy: bptr(false),
				Contexts:                         []manifest.RulesetStatusCheck{{Context: "ci", App: "ci"}, {Context: "lint"}},
			}},
		}}}},
		{name: "rulesets authoritative", recon: &manifest.RepositoryReconcile{Rulesets: authoritative},
			spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{Name: "protect-main", Enforcement: ptr("active")}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkApplyProperties(t, propRulesState(), &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Reconcile: tt.recon,
				Spec:      tt.spec,
			}, tt.noPlan)
		})
	}
}
