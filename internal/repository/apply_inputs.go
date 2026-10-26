package repository

import (
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// ApplyInputs returns the values from spec that apply sends for changes but
// the plan shows only in summary or not at all: the bypass actors and rules
// of a ruleset it creates or updates (shown as "N actors" and "enabled"),
// and the value of a secret it sets, of the repository or an environment
// (shown as "(new)"). Two repositories
// whose changes read the same can still differ here.
//
// Only the values of what changes are returned. Fields a change leaves as
// they are, and identifiers that differ per repository by nature (the ID a
// ruleset delete carries), are left out, so the result differs only when
// apply would do something different. The result is for comparing; it can
// hold secret values, so it must never be printed.
func ApplyInputs(spec manifest.RepositorySpec, changes []Change) map[string]any {
	inputs := make(map[string]any)
	for _, c := range changes {
		switch {
		case c.Type == ChangeDelete || c.Type == ChangeNoOp:
		case strings.HasPrefix(c.Resource, manifest.ResourceRuleset+"["):
			name := strings.TrimSuffix(strings.TrimPrefix(c.Resource, manifest.ResourceRuleset+"["), "]")
			rs := findRuleset(spec.Rulesets, name)
			if rs == nil {
				continue
			}
			for _, d := range c.Details {
				if d.Type == ChangeDelete {
					continue
				}
				if v, ok := summarizedRulesetValue(rs, d.Field); ok {
					inputs[c.Resource+"."+d.Field] = v
				}
			}
		case c.Resource == manifest.ResourceSecret:
			for _, s := range spec.Secrets {
				if s.Name == c.Field {
					inputs[c.Resource+"."+c.Field] = s.Value
				}
			}
		case strings.HasPrefix(c.Resource, manifest.ResourceEnvironment+"["):
			_, envName, _ := splitApplyResource(c.Resource)
			env := findEnvironment(spec.Environments, envName)
			for _, child := range c.Children {
				if key, ok := child.NewValue.(string); ok && child.Field == envFieldSecret {
					if v, ok := environmentValue(env, envFieldSecret, key); ok {
						inputs[c.Resource+".secrets."+key] = v
					}
				}
			}
		}
	}
	return inputs
}

func findRuleset(rulesets []manifest.Ruleset, name string) *manifest.Ruleset {
	for i := range rulesets {
		if rulesets[i].Name == name {
			return &rulesets[i]
		}
	}
	return nil
}

// summarizedRulesetValue returns the value behind a ruleset detail the plan
// summarizes (see diffRulesets): the actors behind "N actors", and the
// settings of a rule behind "enabled".
func summarizedRulesetValue(rs *manifest.Ruleset, field string) (any, bool) {
	switch field {
	case "bypass_actors":
		return rs.BypassActors, true
	case "rules.pull_request":
		return rs.Rules.PullRequest, true
	case "rules.required_status_checks":
		return rs.Rules.RequiredStatusChecks, true
	case "rules.update":
		return rs.Rules.Update, true
	}
	return nil, false
}
