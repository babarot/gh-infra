package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/babarot/gh-infra/internal/manifest"
)

// ruleIntent is what the manifest wants for one rule type: remove it, or
// have it present with the parameters the manifest sets. A rule type that is
// absent from the intents is left as it is on GitHub.
type ruleIntent struct {
	remove bool
	params map[string]any // parameters the manifest sets; nil for none
}

// ruleOrder is the order rules are added in, for a stable payload.
var ruleOrder = []string{
	"pull_request", "required_status_checks", "update",
	"non_fast_forward", "deletion", "creation", "required_linear_history", "required_signatures",
}

// pullRequestDefaults are the parameters the rulesets API requires for a
// pull_request rule, with GitHub's defaults.
func pullRequestDefaults() map[string]any {
	d := manifest.RulesetPullRequest{}.WithDefaults()
	return map[string]any{
		"required_approving_review_count":   *d.RequiredApprovingReviewCount,
		"dismiss_stale_reviews_on_push":     *d.DismissStaleReviewsOnPush,
		"require_code_owner_review":         *d.RequireCodeOwnerReview,
		"require_last_push_approval":        *d.RequireLastPushApproval,
		"required_review_thread_resolution": *d.RequiredReviewThreadResolution,
	}
}

// ruleIntents returns what the manifest says for each rule type it sets.
func ruleIntents(ctx context.Context, rs *manifest.Ruleset, resolver *manifest.Resolver) (map[string]ruleIntent, error) {
	intents := make(map[string]ruleIntent)

	switch {
	case rs.Rules.PullRequestDisabled():
		intents["pull_request"] = ruleIntent{remove: true}
	case rs.Rules.PullRequestEnabled():
		pr := rs.Rules.PullRequest
		params := map[string]any{}
		if pr.RequiredApprovingReviewCount != nil {
			params["required_approving_review_count"] = *pr.RequiredApprovingReviewCount
		}
		if pr.DismissStaleReviewsOnPush != nil {
			params["dismiss_stale_reviews_on_push"] = *pr.DismissStaleReviewsOnPush
		}
		if pr.RequireCodeOwnerReview != nil {
			params["require_code_owner_review"] = *pr.RequireCodeOwnerReview
		}
		if pr.RequireLastPushApproval != nil {
			params["require_last_push_approval"] = *pr.RequireLastPushApproval
		}
		if pr.RequiredReviewThreadResolution != nil {
			params["required_review_thread_resolution"] = *pr.RequiredReviewThreadResolution
		}
		intents["pull_request"] = ruleIntent{params: params}
	}

	switch {
	case rs.Rules.StatusChecksDisabled():
		intents["required_status_checks"] = ruleIntent{remove: true}
	case rs.Rules.StatusChecksEnabled() && resolver != nil:
		sc := rs.Rules.RequiredStatusChecks
		params := map[string]any{}
		if sc.Contexts != nil {
			resolvedChecks, err := resolver.ResolveStatusChecks(ctx, sc.Contexts)
			if err != nil {
				return nil, fmt.Errorf("resolve status checks: %w", err)
			}
			checks := make([]map[string]any, len(resolvedChecks))
			for i, rc := range resolvedChecks {
				check := map[string]any{"context": rc.Context}
				if rc.IntegrationID != 0 {
					check["integration_id"] = rc.IntegrationID
				}
				checks[i] = check
			}
			params["required_status_checks"] = checks
		}
		if sc.StrictRequiredStatusChecksPolicy != nil {
			params["strict_required_status_checks_policy"] = *sc.StrictRequiredStatusChecksPolicy
		}
		intents["required_status_checks"] = ruleIntent{params: params}
	}

	if update := rs.Rules.Update; update != nil && update.Enabled != nil {
		if *update.Enabled {
			var params map[string]any
			if update.AllowsFetchAndMerge != nil {
				params = map[string]any{"update_allows_fetch_and_merge": *update.AllowsFetchAndMerge}
			}
			intents["update"] = ruleIntent{params: params}
		} else {
			intents["update"] = ruleIntent{remove: true}
		}
	}

	toggles := map[string]*bool{
		"non_fast_forward":        rs.Rules.NonFastForward,
		"deletion":                rs.Rules.Deletion,
		"creation":                rs.Rules.Creation,
		"required_linear_history": rs.Rules.RequiredLinearHistory,
		"required_signatures":     rs.Rules.RequiredSignatures,
	}
	for ruleType, enabled := range toggles {
		if enabled != nil {
			intents[ruleType] = ruleIntent{remove: !*enabled}
		}
	}
	return intents, nil
}

// newRule builds a rule that does not exist yet. A new pull_request rule
// gets the defaults for the parameters the manifest leaves out.
func newRule(ruleType string, intent ruleIntent) map[string]any {
	rule := map[string]any{"type": ruleType}
	params := intent.params
	if ruleType == "pull_request" {
		params = mergeParams(pullRequestDefaults(), params)
	}
	if params != nil {
		rule["parameters"] = params
	}
	return rule
}

// mergeParams returns base with override applied on top. Neither is changed.
func mergeParams(base, override map[string]any) map[string]any {
	if base == nil && override == nil {
		return nil
	}
	out := make(map[string]any, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func bypassActorsPayload(ctx context.Context, rs *manifest.Ruleset, resolver *manifest.Resolver) ([]map[string]any, error) {
	actors := []map[string]any{}
	if len(rs.BypassActors) == 0 || resolver == nil {
		return actors, nil
	}
	resolved, err := resolver.ResolveBypassActors(ctx, rs.BypassActors)
	if err != nil {
		return nil, fmt.Errorf("resolve bypass actors: %w", err)
	}
	for _, a := range resolved {
		actors = append(actors, map[string]any{
			"actor_id":    a.ActorID,
			"actor_type":  a.ActorType,
			"bypass_mode": a.BypassMode,
		})
	}
	return actors, nil
}

func conditionsPayload(rs *manifest.Ruleset) map[string]any {
	if rs.Conditions == nil || rs.Conditions.RefName == nil {
		return nil
	}
	exclude := rs.Conditions.RefName.Exclude
	if exclude == nil {
		exclude = []string{}
	}
	return map[string]any{
		"ref_name": map[string]any{
			"include": rs.Conditions.RefName.Include,
			"exclude": exclude,
		},
	}
}

// buildRulesetPayload builds the payload to create a ruleset from the
// manifest alone; there is nothing on GitHub to keep.
func buildRulesetPayload(ctx context.Context, rs *manifest.Ruleset, resolver *manifest.Resolver) (map[string]any, error) {
	target := "branch"
	if rs.Target != nil {
		target = *rs.Target
	}
	enforcement := "active"
	if rs.Enforcement != nil {
		enforcement = *rs.Enforcement
	}

	actors, err := bypassActorsPayload(ctx, rs, resolver)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"name":          rs.Name,
		"target":        target,
		"enforcement":   enforcement,
		"bypass_actors": actors,
	}
	if cond := conditionsPayload(rs); cond != nil {
		payload["conditions"] = cond
	}

	intents, err := ruleIntents(ctx, rs, resolver)
	if err != nil {
		return nil, err
	}
	rules := []map[string]any{}
	for _, ruleType := range ruleOrder {
		if intent, ok := intents[ruleType]; ok && !intent.remove {
			rules = append(rules, newRule(ruleType, intent))
		}
	}
	payload["rules"] = rules
	return payload, nil
}

// currentRulesetJSON is the part of a GET ruleset response that an update
// sends back. Read-only fields (id, source, _links, timestamps, ...) are left
// out.
type currentRulesetJSON struct {
	Target       string            `json:"target"`
	Enforcement  string            `json:"enforcement"`
	BypassActors json.RawMessage   `json:"bypass_actors"`
	Conditions   json.RawMessage   `json:"conditions"`
	Rules        []json.RawMessage `json:"rules"`
}

// buildRulesetUpdatePayload builds the payload to update an existing ruleset.
// The PUT replaces the whole ruleset, so it starts from the current one and
// applies only what the manifest sets: settings, rules, and rule parameters
// the manifest leaves out keep their current values, including rule types
// and parameters gh-infra does not model.
func buildRulesetUpdatePayload(ctx context.Context, rs *manifest.Ruleset, current []byte, resolver *manifest.Resolver) (map[string]any, error) {
	var cur currentRulesetJSON
	if err := json.Unmarshal(current, &cur); err != nil {
		return nil, fmt.Errorf("parse current ruleset %q: %w", rs.Name, err)
	}

	payload := map[string]any{
		"name":        rs.Name,
		"target":      cur.Target,
		"enforcement": cur.Enforcement,
	}
	if rs.Target != nil {
		payload["target"] = *rs.Target
	}
	if rs.Enforcement != nil {
		payload["enforcement"] = *rs.Enforcement
	}

	if rs.HasBypassActors() {
		actors, err := bypassActorsPayload(ctx, rs, resolver)
		if err != nil {
			return nil, err
		}
		payload["bypass_actors"] = actors
	} else if isJSONValue(cur.BypassActors) {
		payload["bypass_actors"] = cur.BypassActors
	} else {
		payload["bypass_actors"] = []map[string]any{}
	}

	if cond := conditionsPayload(rs); cond != nil {
		payload["conditions"] = cond
	} else if isJSONValue(cur.Conditions) {
		payload["conditions"] = cur.Conditions
	}

	intents, err := ruleIntents(ctx, rs, resolver)
	if err != nil {
		return nil, err
	}
	rules := []any{}
	seen := make(map[string]bool)
	for _, raw := range cur.Rules {
		var rule struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil {
			return nil, fmt.Errorf("parse current rule of ruleset %q: %w", rs.Name, err)
		}
		intent, managed := intents[rule.Type]
		if !managed || seen[rule.Type] {
			rules = append(rules, raw)
			continue
		}
		seen[rule.Type] = true
		if intent.remove {
			continue
		}
		params := mergeParams(rule.Parameters, intent.params)
		if rule.Type == "pull_request" {
			// Keep the rule valid if the current one lacks a required parameter.
			params = mergeParams(pullRequestDefaults(), params)
		}
		merged := map[string]any{"type": rule.Type}
		if params != nil {
			merged["parameters"] = params
		}
		rules = append(rules, merged)
	}
	for _, ruleType := range ruleOrder {
		if intent, ok := intents[ruleType]; ok && !intent.remove && !seen[ruleType] {
			rules = append(rules, newRule(ruleType, intent))
		}
	}
	payload["rules"] = rules
	return payload, nil
}

// isJSONValue reports whether raw holds a value other than null.
func isJSONValue(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}
