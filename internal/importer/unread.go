package importer

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// keepUnread replaces each setting of imported that could not be read from
// GitHub with the manifest's (local), so import --into does not read it as
// none and remove it from the manifest. unread is keyed as
// repository.CurrentState.Unread: a manifest key ("labels", "actions"), or
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
		default:
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
			keepUnreadEnvironmentPart(imported.Environments, local.Environments, env, part)
		}
	}
}

// keepUnreadEnvironmentPart gives the imported environment env the
// manifest's part of it. An environment the manifest does not declare keeps
// the part unset, as import leaves it.
func keepUnreadEnvironmentPart(imported, local []manifest.Environment, env, part string) {
	i := slices.IndexFunc(imported, func(e manifest.Environment) bool { return strings.EqualFold(e.Name, env) })
	if i < 0 {
		return
	}
	var l manifest.Environment
	if j := slices.IndexFunc(local, func(e manifest.Environment) bool { return strings.EqualFold(e.Name, env) }); j >= 0 {
		l = local[j]
	}
	switch part {
	case "variables":
		imported[i].Variables, imported[i].VariablesSet = l.Variables, l.VariablesSet
	case "deployment_branch_policy":
		imported[i].DeploymentBranchPolicy = l.DeploymentBranchPolicy
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

// withoutUnread leaves out the warnings about a setting that could not be
// read, or a part of it: they compared the manifest's value, put in its
// place, rather than GitHub's.
func withoutUnread(warnings []RepoWarning, unread map[string]error) []RepoWarning {
	if len(unread) == 0 {
		return warnings
	}
	return slices.DeleteFunc(warnings, func(w RepoWarning) bool {
		for key := range unread {
			if w.Field == key || strings.HasPrefix(w.Field, key+".") {
				return true
			}
		}
		return false
	})
}
