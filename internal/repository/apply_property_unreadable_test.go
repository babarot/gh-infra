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
		{name: "fork PR approval 422, not set", spec: manifest.RepositorySpec{Actions: enabled(manifest.Actions{})}, cmd: forkPRCmd, err: invalid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &unreadableRunner{Runner: newFakeGitHub("o", "r", propState()), cmd: tt.cmd, err: tt.err}
			if tt.err == nil {
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
