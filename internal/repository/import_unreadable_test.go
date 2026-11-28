package repository

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// Import (a fetch without a spec) leaves what it cannot read unread: unset in
// the state and the exported manifest, and named in Unread, so it is neither
// written as none nor read as none by import --into. What GitHub does not
// have, or what the repository does not have the feature for, is not unread.
func TestImportUnreadableSettings(t *testing.T) {
	const (
		labelsCmd     = "label list --repo o/r --json name,color,description --limit 1000"
		variablesCmd  = "variable list --repo o/r --json name,value"
		milestonesCmd = "api repos/o/r/milestones?state=all&per_page=100 --paginate"
		rulesetsCmd   = "api repos/o/r/rulesets --paginate"
		protectedCmd  = "api repos/o/r/branches?protected=true&per_page=100 --paginate --jq .[].name"
		envsCmd       = "api repos/o/r/environments --paginate --slurp"
		envVarsCmd    = "variable list --repo o/r --env production --json name,value"
		envRulesCmd   = "api repos/o/r/environments/production/deployment-branch-policies --paginate --slurp"
		actionsCmd    = "api repos/o/r/actions/permissions"
		workflowCmd   = "api repos/o/r/actions/permissions/workflow"
		selectedCmd   = "api repos/o/r/actions/permissions/selected-actions"
		forkPRCmd     = "api repos/o/r/actions/permissions/fork-pr-contributor-approval"
		forkWFCmd     = "api repos/o/r/actions/permissions/fork-pr-workflows-private-repos"
		repoCmd       = "api repos/o/r --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}"
		immutableCmd  = "api repos/o/r/immutable-releases"
		alertsCmd     = "api repos/o/r/vulnerability-alerts"
		fixesCmd      = "api repos/o/r/automated-security-fixes"
		reportingCmd  = "api repos/o/r/private-vulnerability-reporting"
		analysisCmd   = "api repos/o/r --jq {admin: .permissions.admin, security_and_analysis}"
	)
	forbidden := fakeStatus(gh.ErrForbidden, "Resource not accessible by personal access token")
	notFound := fakeStatus(gh.ErrNotFound, "Not Found")
	invalid := fakeStatus(gh.ErrValidation, "Validation Failed")
	serverError := fmt.Errorf("HTTP 502: Bad Gateway")

	for _, tt := range []struct {
		name   string
		cmd    string
		err    error  // nil: out, or a response that does not parse
		out    []byte // the response when err is nil
		unread []string
		// absent reports whether the exported manifest leaves the setting
		// out.
		absent func(manifest.RepositorySpec) bool
	}{
		{name: "labels 403", cmd: labelsCmd, err: forbidden, unread: []string{"labels"},
			absent: func(s manifest.RepositorySpec) bool { return s.Labels == nil }},
		{name: "labels unparsable", cmd: labelsCmd, unread: []string{"labels"},
			absent: func(s manifest.RepositorySpec) bool { return s.Labels == nil }},
		{name: "variables 404", cmd: variablesCmd, err: notFound, unread: []string{"variables"},
			absent: func(s manifest.RepositorySpec) bool { return s.Variables == nil }},
		{name: "milestones 422", cmd: milestonesCmd, err: invalid, unread: []string{"milestones"},
			absent: func(s manifest.RepositorySpec) bool { return s.Milestones == nil }},
		{name: "rulesets 403 (GitHub Free private)", cmd: rulesetsCmd, err: forbidden, unread: []string{"rulesets"},
			absent: func(s manifest.RepositorySpec) bool { return s.Rulesets == nil }},
		{name: "branch protection 403", cmd: protectedCmd, err: forbidden, unread: []string{"branch_protection"},
			absent: func(s manifest.RepositorySpec) bool { return s.BranchProtection == nil }},
		{name: "environments 404", cmd: envsCmd, err: notFound, unread: []string{"environments"},
			absent: func(s manifest.RepositorySpec) bool { return !s.EnvironmentsSet && s.Environments == nil }},
		{name: "an environment's variables 403", cmd: envVarsCmd, err: forbidden, unread: []string{"environments.production.variables"},
			absent: func(s manifest.RepositorySpec) bool { return environment(s, "production").Variables == nil }},
		{name: "an environment's branch patterns 502", cmd: envRulesCmd, err: serverError, unread: []string{"environments.production.deployment_branch_policy"},
			absent: func(s manifest.RepositorySpec) bool {
				return environment(s, "production").DeploymentBranchPolicy == nil
			}},
		{name: "actions 403", cmd: actionsCmd, err: forbidden, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		{name: "actions 404", cmd: actionsCmd, err: notFound, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		{name: "actions unparsable", cmd: actionsCmd, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		{name: "workflow permissions 404", cmd: workflowCmd, err: notFound, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		{name: "selected actions 403", cmd: selectedCmd, err: forbidden, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		{name: "fork PR approval 403", cmd: forkPRCmd, err: forbidden, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		// A private repository does not have the fork PR approval: the rest
		// of the Actions settings are written.
		{name: "fork PR approval 422", cmd: forkPRCmd, err: invalid,
			absent: func(s manifest.RepositorySpec) bool { return s.Actions != nil && s.Actions.ForkPRApproval == nil }},
		{name: "fork PR workflows 403", cmd: forkWFCmd, err: forbidden, unread: []string{"actions"},
			absent: func(s manifest.RepositorySpec) bool { return s.Actions == nil }},
		// A public repository does not have the fork PR workflow settings.
		{name: "fork PR workflows 422", cmd: forkWFCmd, err: invalid,
			absent: func(s manifest.RepositorySpec) bool { return s.Actions != nil && s.Actions.ForkPRWorkflows == nil }},
		// The settings read one by one are left out rather than written at
		// their defaults.
		{name: "commit message settings 403", cmd: repoCmd, err: forbidden, unread: []string{
			"features.pull_requests", "merge_strategy.allow_auto_merge",
			"merge_strategy.merge_commit_message", "merge_strategy.merge_commit_title",
			"merge_strategy.squash_merge_commit_message", "merge_strategy.squash_merge_commit_title",
		}, absent: func(s manifest.RepositorySpec) bool {
			m := s.MergeStrategy
			return s.Features.PullRequests == nil && m.AllowAutoMerge == nil && m.MergeCommitTitle == nil &&
				m.MergeCommitMessage == nil && m.SquashMergeCommitTitle == nil && m.SquashMergeCommitMessage == nil
		}},
		{name: "release immutability 403", cmd: immutableCmd, err: forbidden, unread: []string{"release_immutability"},
			absent: func(s manifest.RepositorySpec) bool { return s.ReleaseImmutability == nil }},
		{name: "release immutability unparsable", cmd: immutableCmd, unread: []string{"release_immutability"},
			absent: func(s manifest.RepositorySpec) bool { return s.ReleaseImmutability == nil }},
		{name: "vulnerability alerts 403", cmd: alertsCmd, err: forbidden, unread: []string{"security.vulnerability_alerts"},
			absent: func(s manifest.RepositorySpec) bool { return s.Security.VulnerabilityAlerts == nil }},
		{name: "automated security fixes 403", cmd: fixesCmd, err: forbidden, unread: []string{"security.automated_security_fixes"},
			absent: func(s manifest.RepositorySpec) bool { return s.Security.AutomatedSecurityFixes == nil }},
		{name: "private vulnerability reporting 403", cmd: reportingCmd, err: forbidden, unread: []string{"security.private_vulnerability_reporting"},
			absent: func(s manifest.RepositorySpec) bool { return s.Security.PrivateVulnerabilityReporting == nil }},
		// GitHub returns security_and_analysis only to admins (and
		// organization owners and security managers).
		{name: "secret scanning not returned to a non-admin", cmd: analysisCmd, out: []byte(`{"admin":false,"security_and_analysis":null}` + "\n"),
			unread: []string{"security.secret_scanning", "security.secret_scanning_push_protection"},
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.SecretScanning == nil && s.Security.SecretScanningPushProtection == nil
			}},
		{name: "secret scanning status unknown", cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"paused"},"secret_scanning_push_protection":{"status":"enabled"}}}`),
			unread: []string{"security.secret_scanning", "security.secret_scanning_push_protection"},
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.SecretScanning == nil && s.Security.SecretScanningPushProtection == nil
			}},
		// On a public repository GitHub offers both, so to an admin it
		// leaves out neither.
		{name: "secret scanning not returned on a public repository", cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":null}` + "\n"),
			unread: []string{"security.secret_scanning", "security.secret_scanning_push_protection"},
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.SecretScanning == nil && s.Security.SecretScanningPushProtection == nil
			}},
		{name: "push protection not returned on a public repository", cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`),
			unread: []string{"security.secret_scanning", "security.secret_scanning_push_protection"},
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.SecretScanning == nil && s.Security.SecretScanningPushProtection == nil
			}},
		{name: "secret scanning unparsable", cmd: analysisCmd,
			unread: []string{"security.secret_scanning", "security.secret_scanning_push_protection"},
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.SecretScanning == nil && s.Security.SecretScanningPushProtection == nil
			}},
		// GitHub documents a 404 of the vulnerability alerts as disabled.
		{name: "vulnerability alerts 404", cmd: alertsCmd, err: notFound,
			absent: func(s manifest.RepositorySpec) bool {
				return s.Security.VulnerabilityAlerts != nil && !*s.Security.VulnerabilityAlerts
			}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &unreadableRunner{Runner: newFakeGitHub("o", "r", propEnvState()), cmd: tt.cmd, err: tt.err, out: tt.out}
			if tt.err == nil && tt.out == nil {
				runner.out = []byte("not json")
			}
			current, err := NewProcessor(runner, nil).FetchRepository(context.Background(), "o", "r", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Sorted(maps.Keys(current.Unread)); !slices.Equal(got, tt.unread) {
				t.Errorf("unread %v, want %v", got, tt.unread)
			}
			if spec := ToManifest(context.Background(), current, nil).Spec; !tt.absent(spec) {
				t.Errorf("the export writes what could not be read: %+v", spec)
			}
		})
	}

	t.Run("everything read", func(t *testing.T) {
		current, err := NewProcessor(newFakeGitHub("o", "r", propEnvState()), nil).FetchRepository(context.Background(), "o", "r", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(current.Unread) > 0 {
			t.Errorf("unread %v", current.Unread)
		}
		spec := ToManifest(context.Background(), current, nil).Spec
		if spec.Labels == nil || spec.Variables == nil || spec.Milestones == nil || spec.Actions == nil ||
			environment(spec, "production").DeploymentBranchPolicy == nil {
			t.Errorf("the export leaves out what was read: %+v", spec)
		}
	})
}

// On a private repository GitHub does not offer secret scanning on, an admin
// is returned no security_and_analysis, or one without push protection: what
// is left out is off, not unread, and the export leaves it out.
func TestImportSecretScanningNotOffered(t *testing.T) {
	for _, tt := range []struct {
		name         string
		out          string
		wantScanning *bool
	}{
		{name: "secret scanning", out: `{"admin":true,"security_and_analysis":null}` + "\n"},
		{name: "push protection", out: `{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`, wantScanning: manifest.Ptr(true)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propEnvState()
			s.Repo.Visibility = "private"
			runner := &unreadableRunner{Runner: newFakeGitHub("o", "r", s), cmd: "api repos/o/r --jq {admin: .permissions.admin, security_and_analysis}", out: []byte(tt.out)}
			current, err := NewProcessor(runner, nil).FetchRepository(context.Background(), "o", "r", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(current.Unread) > 0 {
				t.Errorf("unread %v", current.Unread)
			}
			sec := ToManifest(context.Background(), current, nil).Spec.Security
			if !reflect.DeepEqual(sec.SecretScanning, tt.wantScanning) || sec.SecretScanningPushProtection != nil {
				t.Errorf("exported secret scanning %v, push protection %v; want %v, none", sec.SecretScanning, sec.SecretScanningPushProtection, tt.wantScanning)
			}
		})
	}
}

// environment returns the environment name of spec, or a zero one.
func environment(s manifest.RepositorySpec, name string) manifest.Environment {
	for _, e := range s.Environments {
		if strings.EqualFold(e.Name, name) {
			return e
		}
	}
	return manifest.Environment{}
}
