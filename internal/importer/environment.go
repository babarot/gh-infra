package importer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// preserveEnvironmentSecrets keeps the secrets the manifest declares for
// each environment, as for the repository's: GitHub does not return secret
// values, so the imported environments have none.
func preserveEnvironmentSecrets(local, imported []manifest.Environment) []manifest.Environment {
	if len(imported) == 0 {
		return imported
	}
	out := make([]manifest.Environment, len(imported))
	copy(out, imported)
	for i := range out {
		for _, l := range local {
			if strings.EqualFold(l.Name, out[i].Name) {
				out[i].Secrets, out[i].SecretsSet = l.Secrets, l.SecretsSet
			}
		}
	}
	return out
}

func formatEnvironmentReviewers(rvs []manifest.EnvironmentReviewer) string {
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
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// formatBranchPolicy shows a policy as one value: all, protected, or the
// patterns of a custom policy.
func formatBranchPolicy(p *manifest.DeploymentBranchPolicy) string {
	if p == nil {
		return ""
	}
	switch p.Mode() {
	case manifest.DeploymentBranchesCustom:
		rules := make([]string, 0, len(p.CustomBranchPolicies))
		for _, r := range p.CustomBranchPolicies {
			if r.RuleType() == manifest.DeploymentBranchTypeTag {
				rules = append(rules, "tag:"+r.Name)
			} else {
				rules = append(rules, r.Name)
			}
		}
		slices.Sort(rules)
		return "custom: [" + strings.Join(rules, ", ") + "]"
	default:
		return p.Mode()
	}
}

// reviewersOf returns the reviewers as compared: unset when the manifest
// does not set them.
func reviewersOf(e manifest.Environment) *string {
	if !e.HasReviewers() {
		return nil
	}
	s := formatEnvironmentReviewers(e.Reviewers)
	return &s
}

func policyOf(e manifest.Environment) *string {
	if e.DeploymentBranchPolicy == nil {
		return nil
	}
	s := formatBranchPolicy(e.DeploymentBranchPolicy)
	return &s
}

// compareEnvironments compares environments by name, ignoring case as
// GitHub does. Secrets are not compared: the imported ones are the
// manifest's (preserveEnvironmentSecrets).
func compareEnvironments(local, imported []manifest.Environment) []FieldDiff {
	if len(local) == 0 && len(imported) == 0 {
		return nil
	}
	find := func(envs []manifest.Environment, name string) *manifest.Environment {
		for i := range envs {
			if strings.EqualFold(envs[i].Name, name) {
				return &envs[i]
			}
		}
		return nil
	}

	var diffs []FieldDiff
	for _, ie := range imported {
		prefix := fmt.Sprintf("environments.%s.", ie.Name)
		le := find(local, ie.Name)
		if le == nil {
			appendFieldCreate(&diffs, prefix+"wait_timer", ie.WaitTimer)
			appendFieldCreate(&diffs, prefix+"can_admins_bypass", ie.CanAdminsBypass)
			appendFieldCreate(&diffs, prefix+"prevent_self_review", ie.PreventSelfReview)
			appendFieldCreate(&diffs, prefix+"reviewers", reviewersOf(ie))
			appendFieldCreate(&diffs, prefix+"deployment_branch_policy", policyOf(ie))
			diffs = append(diffs, compareEnvironmentVariables(prefix, nil, ie.Variables)...)
			continue
		}
		appendFieldUpdate(&diffs, prefix+"wait_timer", le.WaitTimer, ie.WaitTimer)
		appendFieldUpdate(&diffs, prefix+"can_admins_bypass", le.CanAdminsBypass, ie.CanAdminsBypass)
		appendFieldUpdate(&diffs, prefix+"prevent_self_review", le.PreventSelfReview, ie.PreventSelfReview)
		appendFieldUpdate(&diffs, prefix+"reviewers", reviewersOf(*le), reviewersOf(ie))
		appendFieldUpdate(&diffs, prefix+"deployment_branch_policy", policyOf(*le), policyOf(ie))
		if le.Name != ie.Name {
			diffs = append(diffs, FieldDiff{Field: prefix + "name", Old: le.Name, New: ie.Name})
		}
		diffs = append(diffs, compareEnvironmentVariables(prefix, le.Variables, ie.Variables)...)
	}
	for _, le := range local {
		if find(imported, le.Name) == nil {
			diffs = append(diffs, FieldDiff{Field: fmt.Sprintf("environments.%s.settings", le.Name), Old: "declared", New: nil})
		}
	}
	return diffs
}

func compareEnvironmentVariables(prefix string, local, imported []manifest.Variable) []FieldDiff {
	var diffs []FieldDiff
	for _, d := range compareVariables(local, imported) {
		d.Field = prefix + d.Field
		diffs = append(diffs, d)
	}
	return diffs
}

// minimalEnvironments returns the imported environments as an entry writes
// them over the defaults: an environment the defaults lack is written
// whole, one they have only with the fields that differ.
func minimalEnvironments(defaults, imported []manifest.Environment) []manifest.Environment {
	var result []manifest.Environment
	for _, ie := range imported {
		var de *manifest.Environment
		for i := range defaults {
			if strings.EqualFold(defaults[i].Name, ie.Name) {
				de = &defaults[i]
			}
		}
		if de == nil {
			result = append(result, ie)
			continue
		}
		if e, ok := minimalEnvironment(*de, ie); ok {
			result = append(result, e)
		}
	}
	return result
}

// A field the defaults leave out is unmanaged after the merge, so it differs
// from any value GitHub has.
func minimalEnvironment(d, i manifest.Environment) (manifest.Environment, bool) {
	e := manifest.Environment{Name: i.Name}
	changed := d.Name != i.Name
	if !ptrEqualValue(d.WaitTimer, i.WaitTimer) {
		e.WaitTimer, changed = i.WaitTimer, true
	}
	if !ptrEqualValue(d.CanAdminsBypass, i.CanAdminsBypass) {
		e.CanAdminsBypass, changed = i.CanAdminsBypass, true
	}
	if !ptrEqualValue(d.PreventSelfReview, i.PreventSelfReview) {
		e.PreventSelfReview, changed = i.PreventSelfReview, true
	}
	if !ptrEqualValue(reviewersOf(d), reviewersOf(i)) {
		e.Reviewers, e.ReviewersSet, changed = i.Reviewers, i.HasReviewers(), true
	}
	if !ptrEqualValue(policyOf(d), policyOf(i)) {
		e.DeploymentBranchPolicy, changed = i.DeploymentBranchPolicy, true
	}
	if len(compareVariables(d.Variables, i.Variables)) > 0 {
		e.Variables, changed = i.Variables, true
		// None on GitHub: [] so the entry does not inherit the defaults'.
		if len(i.Variables) == 0 {
			e.Variables, e.VariablesSet = []manifest.Variable{}, true
		}
	}
	if !secretsEqual(d.Secrets, i.Secrets) {
		e.Secrets, e.SecretsSet, changed = i.Secrets, true, true
	}
	return e, changed
}

func secretsEqual(a, b []manifest.Secret) bool {
	return slices.Equal(a, b)
}
