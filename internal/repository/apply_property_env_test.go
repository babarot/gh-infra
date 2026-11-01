package repository

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// propEnvState is a public repository with two environments: production,
// limited to patterns with a reviewer, and github-pages, which GitHub made
// and the manifests here leave out.
func propEnvState() fakeState {
	s := propState()
	s.Org = fakeOrg{
		Teams: map[string]int{"deployers": 30, "outsiders": 31},
		Users: map[string]int{"o": 1, "alice": 2, "bob": 3, "eve": 4},
	}
	s.Collaborators = []string{"alice", "bob"}
	s.TeamRepos = []string{"deployers"}
	s.Environments = []*fakeEnvironment{
		{Name: "production", CanAdminsBypass: true, WaitTimer: 10, ReviewersRule: true, PreventSelfReview: true,
			Reviewers: []fakeReviewer{{Type: "User", ID: 2}}, Policy: manifest.DeploymentBranchesCustom,
			Rules: []fakeBranchRule{{ID: 1, Name: "main", Type: "branch"}, {ID: 2, Name: "v*", Type: "tag"}}},
		{Name: "github-pages", CanAdminsBypass: true, Policy: manifest.DeploymentBranchesCustom,
			Rules: []fakeBranchRule{{ID: 3, Name: "main", Type: "branch"}}},
	}
	s.NextBranchRuleID = 3
	return s
}

func TestApplyPropertiesEnvironments(t *testing.T) {
	bptr := manifest.Ptr[bool]
	iptr := manifest.Ptr[int]
	authoritative := &manifest.RepositoryReconcile{DeploymentBranchPolicies: manifest.Ptr(manifest.CollectionReconcileAuthoritative)}
	custom := func(rules ...manifest.DeploymentBranchRule) *manifest.DeploymentBranchPolicy {
		return &manifest.DeploymentBranchPolicy{CustomBranchPolicies: rules, CustomBranchPoliciesSet: true}
	}
	env := func(e manifest.Environment) manifest.RepositorySpec {
		return manifest.RepositorySpec{Environments: []manifest.Environment{e}}
	}
	for _, tt := range []struct {
		name   string
		state  func(*fakeState)
		spec   manifest.RepositorySpec
		recon  *manifest.RepositoryReconcile
		noPlan bool
	}{
		{name: "the same as GitHub", noPlan: true, spec: env(manifest.Environment{
			Name: "production", WaitTimer: iptr(10), PreventSelfReview: bptr(true),
			Reviewers:              []manifest.EnvironmentReviewer{{User: "Alice"}},
			DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "main"}, manifest.DeploymentBranchRule{Name: "v*", Type: "tag"}),
		})},
		{name: "a name in another case is the same environment", noPlan: true, spec: env(manifest.Environment{Name: "Production", WaitTimer: iptr(10)})},
		{name: "only the name", noPlan: true, spec: env(manifest.Environment{Name: "production"})},
		{name: "new environment with every setting", spec: env(manifest.Environment{
			Name: "staging", WaitTimer: iptr(5), CanAdminsBypass: bptr(false), PreventSelfReview: bptr(true),
			Reviewers:              []manifest.EnvironmentReviewer{{User: "bob"}, {Team: "deployers"}},
			DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "main"}, manifest.DeploymentBranchRule{Name: "main", Type: "tag"}),
		})},
		{name: "new environment with only a name", spec: env(manifest.Environment{Name: "preview"})},
		{name: "new environment for protected branches", spec: env(manifest.Environment{
			Name: "qa", DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{ProtectedBranches: bptr(true)},
		})},
		{name: "new environment that no branch can deploy to", spec: env(manifest.Environment{Name: "frozen", DeploymentBranchPolicy: custom()})},
		{name: "settings changed", spec: env(manifest.Environment{
			Name: "production", WaitTimer: iptr(0), CanAdminsBypass: bptr(false), PreventSelfReview: bptr(false),
			Reviewers: []manifest.EnvironmentReviewer{{User: "alice"}, {Team: "deployers"}},
		})},
		{name: "reviewers removed", spec: env(manifest.Environment{
			Name: "production", Reviewers: []manifest.EnvironmentReviewer{}, ReviewersSet: true,
		})},
		{name: "prevent_self_review alone", spec: env(manifest.Environment{Name: "production", PreventSelfReview: bptr(false)})},
		{name: "patterns added, the rest kept", spec: env(manifest.Environment{
			Name: "production", DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "release/*"}),
		})},
		{name: "patterns authoritative", recon: authoritative, spec: env(manifest.Environment{
			Name: "production", DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "main"}, manifest.DeploymentBranchRule{Name: "release/*"}),
		})},
		{name: "no pattern, authoritative", recon: authoritative, spec: env(manifest.Environment{Name: "production", DeploymentBranchPolicy: custom()})},
		{name: "a pattern changes type", recon: authoritative, spec: env(manifest.Environment{
			Name: "production", DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "main"}, manifest.DeploymentBranchRule{Name: "v*"}),
		})},
		{name: "custom to all branches", spec: env(manifest.Environment{Name: "production", DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{}})},
		{name: "custom to protected branches", spec: env(manifest.Environment{
			Name: "production", DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{ProtectedBranches: bptr(true)},
		})},
		{name: "all branches to custom", state: func(s *fakeState) {
			s.Environments[0].Policy, s.Environments[0].Rules = manifest.DeploymentBranchesAll, nil
		}, spec: env(manifest.Environment{Name: "production", DeploymentBranchPolicy: custom(manifest.DeploymentBranchRule{Name: "main"})})},
		{name: "a reviewers rule GitHub emptied", state: func(s *fakeState) {
			s.Environments[0].Reviewers = nil
		}, spec: env(manifest.Environment{Name: "production", Reviewers: []manifest.EnvironmentReviewer{{User: "bob"}}})},
		{name: "a slash in the name", spec: env(manifest.Environment{Name: "deploy/eu", WaitTimer: iptr(1)})},
		{name: "private repository on Enterprise", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan = "private", "enterprise"
		}, spec: env(manifest.Environment{Name: "staging", WaitTimer: iptr(5), Reviewers: []manifest.EnvironmentReviewer{{User: "bob"}}})},
		{name: "private repository whose environment already has the rule", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan = "private", "enterprise"
			s.Environments[0].WaitTimer = 1
			s.Org.PlanHidden = true // the plan cannot be read, but the rule shows it allows it
		}, spec: env(manifest.Environment{Name: "production", WaitTimer: iptr(30)})},
		{name: "a new environment with no wait timer on a private repository on Pro", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan = "private", "pro"
			s.Environments[0].WaitTimer = 0 // no rule that shows the plan allows wait timers
		}, spec: env(manifest.Environment{Name: "staging", WaitTimer: iptr(0), CanAdminsBypass: bptr(false)})},
		{name: "branch policy on a private repository on Pro", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan = "private", "pro"
			s.Environments[0].WaitTimer, s.Environments[0].ReviewersRule, s.Environments[0].Reviewers = 0, false, nil
		}, spec: env(manifest.Environment{Name: "production", WaitTimer: iptr(0), CanAdminsBypass: bptr(false),
			DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{ProtectedBranches: bptr(true)}})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propEnvState()
			if tt.state != nil {
				tt.state(&s)
			}
			repo := &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Reconcile: tt.recon,
				Spec:      tt.spec,
			}
			checkApplyProperties(t, s, repo, tt.noPlan)
		})
	}
}

// What GitHub would reject or drop fails the plan, before apply.
func TestPlanEnvironmentsRejected(t *testing.T) {
	iptr := manifest.Ptr[int]
	bptr := manifest.Ptr[bool]
	for _, tt := range []struct {
		name    string
		state   func(*fakeState)
		env     manifest.Environment
		wantErr string
	}{
		{name: "a user who cannot read the repository",
			env:     manifest.Environment{Name: "production", Reviewers: []manifest.EnvironmentReviewer{{User: "alice"}, {User: "eve"}}},
			wantErr: "eve cannot read o/r"},
		{name: "a team without access",
			env:     manifest.Environment{Name: "staging", Reviewers: []manifest.EnvironmentReviewer{{Team: "outsiders"}}},
			wantErr: "team:outsiders cannot read o/r"},
		{name: "a user GitHub does not know",
			env:     manifest.Environment{Name: "staging", Reviewers: []manifest.EnvironmentReviewer{{User: "nobody"}}},
			wantErr: `resolve user "nobody"`},
		{name: "prevent_self_review without reviewers", state: func(s *fakeState) {
			s.Environments[0].ReviewersRule, s.Environments[0].Reviewers = false, nil
		}, env: manifest.Environment{Name: "production", PreventSelfReview: bptr(true)},
			wantErr: "needs at least one reviewer"},
		{name: "prevent_self_review: false on a new environment without reviewers",
			env:     manifest.Environment{Name: "staging", PreventSelfReview: bptr(false)},
			wantErr: "needs at least one reviewer"},
		{name: "a wait timer on a private repository on Team", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan = "private", "team"
			s.Environments[0].WaitTimer = 0
		}, env: manifest.Environment{Name: "production", WaitTimer: iptr(5)},
			wantErr: "GitHub's team plan allows it only on public repositories"},
		{name: "reviewers on a private repository whose plan cannot be read", state: func(s *fakeState) {
			s.Repo.Visibility, s.Org.Plan, s.Org.PlanHidden = "private", "enterprise", true
			for _, e := range s.Environments {
				e.ReviewersRule, e.Reviewers, e.WaitTimer = false, nil, 0
			}
		}, env: manifest.Environment{Name: "staging", Reviewers: []manifest.EnvironmentReviewer{{User: "bob"}}},
			wantErr: "cannot tell whether the plan of o allows it"},
		{name: "a user's own private repository without the user scope", state: func(s *fakeState) {
			s.Repo.Visibility = "private"
			s.Org.UserOwner, s.Org.AuthUser, s.Org.Plan = true, "someone-else", "pro"
			s.Environments[0].WaitTimer = 0
		}, env: manifest.Environment{Name: "production", WaitTimer: iptr(5)},
			wantErr: "cannot tell whether the plan of o allows it"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propEnvState()
			if tt.state != nil {
				tt.state(&s)
			}
			fake := newFakeGitHub("o", "r", s)
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := &manifest.Repository{
				Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Spec:     manifest.RepositorySpec{Environments: []manifest.Environment{tt.env}},
			}
			_, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if len(fake.writes) > 0 {
				t.Errorf("plan wrote: %v", fake.writes)
			}
		})
	}
}

// A user's own private repository: the plan is read with the user scope.
func TestPlanEnvironmentsUserPlan(t *testing.T) {
	s := propEnvState()
	s.Repo.Visibility = "private"
	s.Org.UserOwner, s.Org.AuthUser, s.Org.Plan = true, "O", "pro"
	s.Environments[0].WaitTimer = 0
	fake := newFakeGitHub("o", "r", s)
	p := NewProcessor(fake, manifest.NewResolver(fake))
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec:     manifest.RepositorySpec{Environments: []manifest.Environment{{Name: "production", WaitTimer: manifest.Ptr(5)}}},
	}
	_, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "GitHub's pro plan") {
		t.Fatalf("err = %v, want the pro plan to reject it", err)
	}
}

// The plan shows the patterns GitHub deletes with a custom policy, and warns
// about changes that let more deploy.
func TestDiffEnvironmentsShowsWhatGitHubDoes(t *testing.T) {
	fake := newFakeGitHub("o", "r", propEnvState())
	p := NewProcessor(fake, manifest.NewResolver(fake))
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec: manifest.RepositorySpec{Environments: []manifest.Environment{{
			Name: "production", CanAdminsBypass: manifest.Ptr(true),
			Reviewers: []manifest.EnvironmentReviewer{}, ReviewersSet: true,
			DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{},
		}}},
	}
	changes, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Resource != "Environment[production]" {
		t.Fatalf("changes = %s", formatChanges(changes))
	}
	var removed []string
	for _, d := range changes[0].Details {
		if d.Type == ChangeDelete {
			s, _ := d.OldValue.(string)
			removed = append(removed, s)
		}
	}
	if strings.Join(removed, ",") != "main"+removedByGitHub+",tag:v*"+removedByGitHub {
		t.Errorf("removed patterns = %v", removed)
	}
	notes := strings.Join(ChangeNotes(changes), "\n")
	for _, want := range []string{"any branch or tag can deploy", "no longer need a review"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q lack %q", notes, want)
		}
	}
	if strings.Contains(notes, "admins can bypass") {
		t.Errorf("notes warn about can_admins_bypass, which does not change: %q", notes)
	}
}

// A new repository: GitHub drops a reviewer who cannot read it, and only its
// owner can, so the create adds the owner and leaves the rest, with
// prevent_self_review when no reviewer is left, to a later plan.
func TestApplyEnvironmentsNewRepository(t *testing.T) {
	bptr := manifest.Ptr[bool]
	for _, tt := range []struct {
		name string
		env  manifest.Environment
		// deferred: the next plan shows the reviewers, once they can read
		// the repository.
		deferred bool
	}{
		{name: "the owner as reviewer", env: manifest.Environment{
			Name: "production", PreventSelfReview: bptr(true), Reviewers: []manifest.EnvironmentReviewer{{User: "O"}},
			DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{CustomBranchPolicies: []manifest.DeploymentBranchRule{{Name: "main"}}},
		}},
		{name: "a team as reviewer", deferred: true, env: manifest.Environment{
			Name: "production", PreventSelfReview: bptr(true), Reviewers: []manifest.EnvironmentReviewer{{Team: "deployers"}},
		}},
		{name: "the owner and a team", deferred: true, env: manifest.Environment{
			Name: "production", PreventSelfReview: bptr(true), Reviewers: []manifest.EnvironmentReviewer{{User: "o"}, {Team: "deployers"}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fake := newMissingFakeGitHub("o", "r", githubDefaultLabels)
			fake.s.Org = propEnvState().Org
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := &manifest.Repository{
				Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Spec:     manifest.RepositorySpec{Visibility: manifest.Ptr("public"), Environments: []manifest.Environment{tt.env}},
			}
			if notes := environmentCreateNotes(repo); (len(notes) > 0) != tt.deferred {
				t.Errorf("create notes = %q, deferred = %v", notes, tt.deferred)
			}
			changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range p.Apply(ctx, changes, targets, nil) {
				if r.Err != nil {
					t.Fatalf("apply %s: %v\nwrites: %s", r.Change.Resource, r.Err, strings.Join(fake.writes, "\n  "))
				}
			}

			// The team gets access; the next plan adds it.
			fake.s.TeamRepos = []string{"deployers"}
			next, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if (len(next) > 0) != tt.deferred {
				t.Fatalf("next plan = %s, deferred = %v", formatChanges(next), tt.deferred)
			}
			for _, r := range p.Apply(ctx, next, targets, nil) {
				if r.Err != nil {
					t.Fatalf("apply %s: %v", r.Change.Resource, r.Err)
				}
			}
			if again, _, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil); err != nil || len(again) > 0 {
				t.Errorf("plan after the reviewers = %s, %v", formatChanges(again), err)
			}
		})
	}
}

// Patterns the manifest leaves out are deleted only with authoritative
// reconcile; the environments it leaves out are never touched.
func TestApplyEnvironmentPatternsReconcile(t *testing.T) {
	for _, tt := range []struct {
		name  string
		recon *manifest.RepositoryReconcile
		want  string
	}{
		{name: "additive", want: "branch:main,branch:release/*,tag:v*"},
		{name: "authoritative", recon: &manifest.RepositoryReconcile{DeploymentBranchPolicies: manifest.Ptr(manifest.CollectionReconcileAuthoritative)},
			want: "branch:main,branch:release/*"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fake := newFakeGitHub("o", "r", propEnvState())
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Reconcile: tt.recon,
				Spec: manifest.RepositorySpec{Environments: []manifest.Environment{{
					Name: "production",
					DeploymentBranchPolicy: &manifest.DeploymentBranchPolicy{CustomBranchPolicies: []manifest.DeploymentBranchRule{
						{Name: "main"}, {Name: "release/*"},
					}},
				}}},
			}
			changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range p.Apply(ctx, changes, targets, nil) {
				if r.Err != nil {
					t.Fatal(r.Err)
				}
			}
			s := fake.state()
			var got []string
			for _, r := range s.Environments[0].Rules {
				got = append(got, r.Type+":"+r.Name)
			}
			slices.Sort(got)
			if strings.Join(got, ",") != tt.want {
				t.Errorf("patterns = %v, want %s", got, tt.want)
			}
			if pages := s.Environments[1]; len(pages.Rules) != 1 || pages.Policy != manifest.DeploymentBranchesCustom {
				t.Errorf("github-pages changed: %+v", pages)
			}
		})
	}
}

// GitHub accepts a PUT and saves less than it sent: a reviewer who cannot
// read the repository is dropped. apply reports it as failed.
func TestVerifyEnvironmentResponse(t *testing.T) {
	sent := sentEnvironment{
		waitTimer:    manifest.Ptr(5),
		hasReviewers: true,
		reviewers:    []manifest.ResolvedEnvironmentReviewer{{Type: "User", ID: 2}, {Type: "User", ID: 4}},
		branchPolicy: manifest.DeploymentBranchesCustom,
	}
	saved := `{"name":"production","can_admins_bypass":true,"protection_rules":[
		{"type":"wait_timer","wait_timer":5},
		{"type":"required_reviewers","prevent_self_review":false,"reviewers":[{"type":"User","reviewer":{"id":2,"login":"alice"}}]},
		{"type":"branch_policy"}],
		"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}`
	err := verifyEnvironmentResponse([]byte(saved), sent)
	if err == nil || !strings.Contains(err.Error(), "reviewers (sent 2, got 1") {
		t.Fatalf("err = %v, want the dropped reviewer reported", err)
	}
	sent.reviewers = sent.reviewers[:1]
	if err := verifyEnvironmentResponse([]byte(saved), sent); err != nil {
		t.Fatalf("err = %v, want none", err)
	}
}
