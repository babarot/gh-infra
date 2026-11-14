package repository

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// unreadableRunner answers one command with err, or with out when err is nil
// (a response that does not parse), and passes the rest to Runner.
type unreadableRunner struct {
	gh.Runner
	cmd string
	err error
	out []byte
}

func (r *unreadableRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if strings.Join(args, " ") == r.cmd {
		return r.out, r.err
	}
	return r.Runner.Run(ctx, args...)
}

// errorTracker records the repositories plan reports as failed.
type errorTracker struct {
	mu     sync.Mutex
	errors map[string]error
}

func (t *errorTracker) UpdateStatus(string, string) {}
func (t *errorTracker) Checkpoint(string, string)   {}
func (t *errorTracker) Done(string, PlanSummary)    {}
func (t *errorTracker) Error(name string, err error) {
	t.mu.Lock()
	t.errors[name] = err
	t.mu.Unlock()
}

// A setting the manifest manages that cannot be read fails the repository at
// plan: read as none or at its zero value, it would be planned against what
// GitHub does not have (an existing secret planned as new and set again).
// What plans nothing whatever GitHub has (an empty, additive collection, an
// Actions endpoint none of whose fields the manifest sets) is not an error.
func TestPlanUnreadableManagedSettings(t *testing.T) {
	const (
		secretsCmd    = "secret list --repo o/r --json name --jq .[].name"
		variablesCmd  = "variable list --repo o/r --json name,value"
		labelsCmd     = "label list --repo o/r --json name,color,description --limit 1000"
		milestonesCmd = "api repos/o/r/milestones?state=all&per_page=100 --paginate"
		rulesetsCmd   = "api repos/o/r/rulesets --paginate"
		actionsCmd    = "api repos/o/r/actions/permissions"
		workflowCmd   = "api repos/o/r/actions/permissions/workflow"
		selectedCmd   = "api repos/o/r/actions/permissions/selected-actions"
		forkPRCmd     = "api repos/o/r/actions/permissions/fork-pr-contributor-approval"
		repoCmd       = "api repos/o/r --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}"
		immutableCmd  = "api repos/o/r/immutable-releases"
		alertsCmd     = "api repos/o/r/vulnerability-alerts"
		analysisCmd   = "api repos/o/r --jq {admin: .permissions.admin, security_and_analysis}"
	)
	notFound := fakeStatus(gh.ErrNotFound, "Not Found")
	invalid := fakeStatus(gh.ErrValidation, "Validation Failed")
	garbage := []byte("not json")
	authoritative := &manifest.RepositoryReconcile{
		Labels:   manifest.Ptr(manifest.CollectionReconcileAuthoritative),
		Rulesets: manifest.Ptr(manifest.CollectionReconcileAuthoritative),
	}
	enabled := func(a manifest.Actions) *manifest.Actions {
		a.Enabled = manifest.Ptr(true)
		return &a
	}

	for _, tt := range []struct {
		name    string
		spec    manifest.RepositorySpec
		recon   *manifest.RepositoryReconcile
		cmd     string
		err     error
		out     []byte // with no err: the response, garbage when nil
		wantErr bool
	}{
		{name: "secrets 404", spec: manifest.RepositorySpec{Secrets: []manifest.Secret{{Name: "TOKEN", Value: "x"}}}, cmd: secretsCmd, err: notFound, wantErr: true},
		{name: "secrets 422", spec: manifest.RepositorySpec{Secrets: []manifest.Secret{{Name: "TOKEN", Value: "x"}}}, cmd: secretsCmd, err: invalid, wantErr: true},
		{name: "variables 404", spec: manifest.RepositorySpec{Variables: []manifest.Variable{{Name: "ENV", Value: "prod"}}}, cmd: variablesCmd, err: notFound, wantErr: true},
		{name: "variables 422", spec: manifest.RepositorySpec{Variables: []manifest.Variable{{Name: "ENV", Value: "prod"}}}, cmd: variablesCmd, err: invalid, wantErr: true},
		{name: "variables unparsable", spec: manifest.RepositorySpec{Variables: []manifest.Variable{{Name: "ENV", Value: "prod"}}}, cmd: variablesCmd, wantErr: true},
		{name: "labels 404", spec: manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "d73a4a"}}}, cmd: labelsCmd, err: notFound, wantErr: true},
		{name: "labels 422", spec: manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "d73a4a"}}}, cmd: labelsCmd, err: invalid, wantErr: true},
		{name: "labels unparsable", spec: manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "d73a4a"}}}, cmd: labelsCmd, wantErr: true},
		{name: "labels [] authoritative 404", spec: manifest.RepositorySpec{LabelsSet: true}, recon: authoritative, cmd: labelsCmd, err: notFound, wantErr: true},
		{name: "labels [] additive 404", spec: manifest.RepositorySpec{LabelsSet: true}, cmd: labelsCmd, err: notFound},
		{name: "milestones 404", spec: manifest.RepositorySpec{Milestones: []manifest.Milestone{{Title: "v2"}}}, cmd: milestonesCmd, err: notFound, wantErr: true},
		{name: "milestones 422", spec: manifest.RepositorySpec{Milestones: []manifest.Milestone{{Title: "v2"}}}, cmd: milestonesCmd, err: invalid, wantErr: true},
		{name: "milestones unparsable", spec: manifest.RepositorySpec{Milestones: []manifest.Milestone{{Title: "v2"}}}, cmd: milestonesCmd, wantErr: true},
		{name: "rulesets 404", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{Name: "main", Enforcement: manifest.Ptr("active")}}}, cmd: rulesetsCmd, err: notFound, wantErr: true},
		{name: "rulesets unparsable", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{Name: "main", Enforcement: manifest.Ptr("active")}}}, cmd: rulesetsCmd, wantErr: true},
		{name: "rulesets [] authoritative 404", spec: manifest.RepositorySpec{RulesetsSet: true}, recon: authoritative, cmd: rulesetsCmd, err: notFound, wantErr: true},
		{name: "rulesets [] additive 404", spec: manifest.RepositorySpec{RulesetsSet: true}, cmd: rulesetsCmd, err: notFound},
		{name: "actions permissions 404", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: actionsCmd, err: notFound, wantErr: true},
		{name: "actions permissions unparsable", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: actionsCmd, wantErr: true},
		{name: "workflow permissions 404", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{WorkflowPermissions: manifest.Ptr("read")})}, cmd: workflowCmd, err: notFound, wantErr: true},
		{name: "workflow permissions unparsable", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{CanApprovePullRequests: manifest.Ptr(false)})}, cmd: workflowCmd, wantErr: true},
		{name: "workflow permissions 404, not set", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: workflowCmd, err: notFound},
		{name: "selected actions 404", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{SelectedActions: &manifest.SelectedActions{GithubOwnedAllowed: manifest.Ptr(true)}})}, cmd: selectedCmd, err: notFound, wantErr: true},
		{name: "selected actions 404, not set", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: selectedCmd, err: notFound},
		// GitHub answers 422 on a private repository; a manifest that sets
		// fork_pr_approval without visibility could not apply it there.
		{name: "fork PR approval 422", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{ForkPRApproval: manifest.Ptr("first_time_contributors")})}, cmd: forkPRCmd, err: invalid, wantErr: true},
		{name: "fork PR approval 404", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{ForkPRApproval: manifest.Ptr("first_time_contributors")})}, cmd: forkPRCmd, err: notFound, wantErr: true},
		{name: "fork PR approval 422, made public", spec: manifest.RepositorySpec{Visibility: manifest.Ptr("public"), Actions: enabled(manifest.Actions{ForkPRApproval: manifest.Ptr("first_time_contributors")})}, cmd: forkPRCmd, err: invalid},
		// A single setting that cannot be read is not compared at its
		// default: a 404 (GHES without the feature) could not be applied.
		{name: "release immutability 404", spec: manifest.RepositorySpec{ReleaseImmutability: manifest.Ptr(true)}, cmd: immutableCmd, err: notFound, wantErr: true},
		{name: "release immutability unparsable", spec: manifest.RepositorySpec{ReleaseImmutability: manifest.Ptr(true)}, cmd: immutableCmd, wantErr: true},
		{name: "commit message settings 404", spec: manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{AllowAutoMerge: manifest.Ptr(false)}}, cmd: repoCmd, err: notFound, wantErr: true},
		// GitHub documents a 404 of the vulnerability alerts as disabled.
		{name: "vulnerability alerts 404", spec: manifest.RepositorySpec{Security: &manifest.Security{VulnerabilityAlerts: manifest.Ptr(false)}}, cmd: alertsCmd, err: notFound},
		// GitHub returns security_and_analysis only to admins (and
		// organization owners and security managers).
		{name: "secret scanning not returned to a non-admin", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(false)}},
			cmd: analysisCmd, out: []byte(`{"admin":false,"security_and_analysis":null}` + "\n"), wantErr: true},
		// On a public repository GitHub offers both, so to an admin it
		// leaves out neither.
		{name: "secret scanning not returned on a public repository", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanningPushProtection: manifest.Ptr(false)}},
			cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":null}` + "\n"), wantErr: true},
		{name: "push protection not returned on a public repository", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
			cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`), wantErr: true},
		{name: "secret scanning status unknown", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
			cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"paused"},"secret_scanning_push_protection":{"status":"enabled"}}}`), wantErr: true},
		// Both are read: plan checks one against the other.
		{name: "push protection status unknown, secret scanning managed", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
			cmd: analysisCmd, out: []byte(`{"admin":true,"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_push_protection":{"status":"paused"}}}`), wantErr: true},
		{name: "secret scanning unparsable", spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}}, cmd: analysisCmd, wantErr: true},
		{name: "fork PR approval 422, not set", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: forkPRCmd, err: invalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &unreadableRunner{Runner: newFakeGitHub("o", "r", propState()), cmd: tt.cmd, err: tt.err, out: tt.out}
			if tt.err == nil && tt.out == nil {
				runner.out = garbage
			}
			p := NewProcessor(runner, manifest.NewResolver(runner))
			repo := &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Spec:      tt.spec,
				Reconcile: tt.recon,
			}
			tracker := &errorTracker{errors: map[string]error{}}
			changes, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, tracker)
			if err != nil {
				t.Fatal(err)
			}
			reported := tracker.errors["o/r"]
			switch {
			case tt.wantErr && reported == nil:
				t.Errorf("not reported; planned %s from what could not be read", formatChanges(changes))
			case tt.wantErr && len(changes) > 0:
				t.Errorf("reported, but planned %s", formatChanges(changes))
			case !tt.wantErr && reported != nil:
				t.Errorf("reported %v for what plans nothing", reported)
			}
		})
	}
}

// Plan rejects what GitHub would not do. Where it does not offer secret
// scanning (a private repository whose owner lacks GitHub Secret
// Protection), it returns no security_and_analysis to an admin: both are off
// and cannot be enabled. It saves no push protection without secret
// scanning, and disabling secret scanning disables push protection too; it
// answers 200 to both.
func TestPlanSecretScanningRejected(t *testing.T) {
	notOffered := func(s *fakeState) { s.Repo.Visibility, s.NoSecretScanning = "private", true }
	for _, tt := range []struct {
		name    string
		state   func(*fakeState)
		missing bool // the repository does not exist yet
		spec    manifest.RepositorySpec
		wantErr string
	}{
		{name: "secret scanning not offered", state: notOffered,
			spec:    manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
			wantErr: "security.secret_scanning: GitHub does not offer it on o/r (a private repository needs GitHub Secret Protection"},
		{name: "push protection not offered", state: notOffered,
			spec:    manifest.RepositorySpec{Security: &manifest.Security{SecretScanningPushProtection: manifest.Ptr(true)}},
			wantErr: "security.secret_scanning_push_protection: GitHub does not offer it on o/r"},
		{name: "not offered, disabled", state: notOffered,
			spec: manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(false), SecretScanningPushProtection: manifest.Ptr(false)}}},
		{name: "not offered while private, made public", state: notOffered,
			spec:    manifest.RepositorySpec{Visibility: manifest.Ptr("public"), Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
			wantErr: "security.secret_scanning: GitHub does not offer it on o/r while it is private (apply the visibility change first"},
		{name: "push protection without secret scanning", state: func(s *fakeState) { s.SecretScanning, s.SecretScanningPushProtection = false, false },
			spec:    manifest.RepositorySpec{Security: &manifest.Security{SecretScanningPushProtection: manifest.Ptr(true)}},
			wantErr: "security.secret_scanning_push_protection: true requires security.secret_scanning to be enabled"},
		{name: "push protection without secret scanning, new repository", missing: true,
			spec:    manifest.RepositorySpec{Visibility: manifest.Ptr("public"), Security: &manifest.Security{SecretScanningPushProtection: manifest.Ptr(true)}},
			wantErr: "security.secret_scanning_push_protection: true requires security.secret_scanning to be enabled"},
		{name: "secret scanning disabled under push protection",
			spec:    manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(false)}},
			wantErr: "security.secret_scanning: false also disables secret_scanning_push_protection on GitHub (set security.secret_scanning_push_protection: false as well)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newMissingFakeGitHub("o", "r", nil)
			if !tt.missing {
				s := propState()
				if tt.state != nil {
					tt.state(&s)
				}
				fake = newFakeGitHub("o", "r", s)
			}
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"}, Spec: tt.spec}
			changes, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
			if tt.wantErr == "" {
				if err != nil || len(changes) > 0 {
					t.Errorf("plan = %s, %v; want no changes", formatChanges(changes), err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("plan error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
