package importer

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// keepUnread replaces each setting of imported that could not be read from
// GitHub with the manifest's (local), so import --into does not read it as
// none or at its default and remove or replace it in the manifest. unread is
// keyed as repository.CurrentState.Unread: a manifest key ("labels",
// "actions", "release_immutability", "security.vulnerability_alerts",
// "merge_strategy.allow_auto_merge", "features.pull_requests"), or
// "environments.<name>.variables" and
// "environments.<name>.deployment_branch_policy" for a part of an
// environment.
func keepUnread(imported *manifest.RepositorySpec, local manifest.RepositorySpec, unread map[string]error) {
	if len(unread) == 0 {
		return
	}
	cloned := false
	for key := range unread {
		switch key {
		case "branch_protection":
			imported.BranchProtection, imported.BranchProtectionSet = local.BranchProtection, local.BranchProtectionSet
		case "rulesets":
			imported.Rulesets, imported.RulesetsSet = local.Rulesets, local.RulesetsSet
		case "variables":
			imported.Variables, imported.VariablesSet = local.Variables, local.VariablesSet
		case "labels":
			imported.Labels, imported.LabelsSet = local.Labels, local.LabelsSet
		case "milestones":
			imported.Milestones, imported.MilestonesSet = local.Milestones, local.MilestonesSet
		case "environments":
			imported.Environments, imported.EnvironmentsSet = local.Environments, local.EnvironmentsSet
		case "actions":
			imported.Actions = local.Actions
		case "release_immutability":
			imported.ReleaseImmutability = local.ReleaseImmutability
		case "features.pull_requests":
			f := cloneOrZero(imported.Features)
			if local.Features != nil {
				f.PullRequests = local.Features.PullRequests
			} else {
				f.PullRequests = nil
			}
			imported.Features = nilIfZero(f)
		default:
			if field, ok := strings.CutPrefix(key, "security."); ok {
				imported.Security = keepUnreadSecurity(imported.Security, local.Security, field)
				continue
			}
			if field, ok := strings.CutPrefix(key, "merge_strategy."); ok {
				m := cloneOrZero(imported.MergeStrategy)
				var l manifest.MergeStrategy
				if local.MergeStrategy != nil {
					l = *local.MergeStrategy
				}
				switch field {
				case "allow_auto_merge":
					m.AllowAutoMerge = l.AllowAutoMerge
				case "merge_commit_title":
					m.MergeCommitTitle = l.MergeCommitTitle
				case "merge_commit_message":
					m.MergeCommitMessage = l.MergeCommitMessage
				case "squash_merge_commit_title":
					m.SquashMergeCommitTitle = l.SquashMergeCommitTitle
				case "squash_merge_commit_message":
					m.SquashMergeCommitMessage = l.SquashMergeCommitMessage
				}
				imported.MergeStrategy = nilIfZero(m)
				continue
			}
			env, part, ok := environmentPart(key)
			if !ok {
				continue
			}
			if !cloned {
				// The environments are shared with the other manifests of
				// the repository.
				imported.Environments = slices.Clone(imported.Environments)
				cloned = true
			}
			keepUnreadEnvironmentPart(&imported.Environments, local.Environments, env, part)
		}
	}
}

// keepUnreadSecurity returns imported's security settings with field set as
// local has it; none when no field is left.
func keepUnreadSecurity(imported, local *manifest.Security, field string) *manifest.Security {
	s := cloneOrZero(imported)
	var l manifest.Security
	if local != nil {
		l = *local
	}
	switch field {
	case "vulnerability_alerts":
		s.VulnerabilityAlerts = l.VulnerabilityAlerts
	case "automated_security_fixes":
		s.AutomatedSecurityFixes = l.AutomatedSecurityFixes
	case "private_vulnerability_reporting":
		s.PrivateVulnerabilityReporting = l.PrivateVulnerabilityReporting
	}
	return nilIfZero(s)
}

// nilIfZero returns a pointer to v, or nil when no field of it is set: an
// empty object would be written as {}.
func nilIfZero[T any](v T) *T {
	if reflect.ValueOf(v).IsZero() {
		return nil
	}
	return &v
}

// cloneOrZero copies *p, which the other manifests of the repository share,
// or returns the zero value for nil.
func cloneOrZero[T any](p *T) T {
	var v T
	if p != nil {
		v = *p
	}
	return v
}

// keepUnreadEnvironmentPart gives the environment env of imported the
// manifest's part of it. An environment the manifest does not declare keeps
// the part unset, as import leaves it. An environment imported leaves out (a
// RepositorySet override that matches the defaults) is added back with the
// part when the manifest sets it, so the manifest keeps it.
func keepUnreadEnvironmentPart(imported *[]manifest.Environment, local []manifest.Environment, env, part string) {
	var l manifest.Environment
	declared := false
	if j := slices.IndexFunc(local, func(e manifest.Environment) bool { return strings.EqualFold(e.Name, env) }); j >= 0 {
		l, declared = local[j], true
	}
	i := slices.IndexFunc(*imported, func(e manifest.Environment) bool { return strings.EqualFold(e.Name, env) })
	if i < 0 {
		sets := (part == "variables" && (l.VariablesSet || len(l.Variables) > 0)) ||
			(part == "deployment_branch_policy" && l.DeploymentBranchPolicy != nil)
		if !declared || !sets {
			return
		}
		*imported = append(*imported, manifest.Environment{Name: l.Name})
		i = len(*imported) - 1
	}
	switch part {
	case "variables":
		(*imported)[i].Variables, (*imported)[i].VariablesSet = l.Variables, l.VariablesSet
	case "deployment_branch_policy":
		(*imported)[i].DeploymentBranchPolicy = l.DeploymentBranchPolicy
	}
}

// environmentPart splits an Unread key of a part of an environment.
func environmentPart(key string) (env, part string, ok bool) {
	rest, found := strings.CutPrefix(key, "environments.")
	if !found {
		return "", "", false
	}
	for _, p := range []string{"variables", "deployment_branch_policy"} {
		if env, found := strings.CutSuffix(rest, "."+p); found && env != "" {
			return env, p, true
		}
	}
	return "", "", false
}

// unreadWarnings tells, for import --into, what could not be read and was
// left as in the manifest.
func unreadWarnings(unread map[string]error) []RepoWarning {
	var out []RepoWarning
	for _, key := range UnreadKeys(unread) {
		out = append(out, RepoWarning{
			Field:   key,
			Message: fmt.Sprintf("could not be read (%s); left as in the manifest", unreadReason(unread[key])),
		})
	}
	return out
}

// UnreadKeys returns the keys of unread in order.
func UnreadKeys(unread map[string]error) []string {
	keys := make([]string, 0, len(unread))
	for k := range unread {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// unreadReason says briefly why a setting could not be read.
func unreadReason(err error) string {
	switch {
	case errors.Is(err, gh.ErrForbidden):
		return "HTTP 403"
	case errors.Is(err, gh.ErrNotFound):
		return "HTTP 404"
	case errors.Is(err, gh.ErrValidation):
		return "HTTP 422"
	case err != nil && strings.Contains(err.Error(), "parse response"):
		return "the response does not parse"
	case err != nil:
		return err.Error()
	}
	return "unknown"
}
