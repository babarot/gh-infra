package repository

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// recordingRunner answers like substringRunner and records the commands.
type recordingRunner struct {
	substringRunner
	mu     sync.Mutex
	called []string
}

func (r *recordingRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.called = append(r.called, strings.Join(args, " "))
	r.mu.Unlock()
	return r.substringRunner.Run(ctx, args...)
}

func (r *recordingRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func (r *recordingRunner) teamLookups() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.called {
		if strings.Contains(c, "/teams") {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

// Two organizations, each with a ruleset whose bypass actor is a team of its
// own organization; both rulesets match GitHub.
func multiOwnerRunner() *recordingRunner {
	ruleset := func(teamID string) string {
		return `{"id":1,"name":"main","target":"branch","enforcement":"active","source_type":"Repository",
			"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[],
			"bypass_actors":[{"actor_id":` + teamID + `,"actor_type":"Team","bypass_mode":"always"}]}`
	}
	return &recordingRunner{substringRunner: substringRunner{rules: [][2]string{
		{"repo view", `{"visibility":"PUBLIC","defaultBranchRef":{"name":"main"}}`},
		{"repos/org-a/app/rulesets/1", ruleset("11")},
		{"repos/org-b/svc/rulesets/1", ruleset("22")},
		{"/rulesets", `[{"id":1,"name":"main","source_type":"Repository"}]`},
		{"orgs/org-a/teams/platform", `{"id":11}`},
		{"orgs/org-b/teams/sre", `{"id":22}`},
		{"orgs/org-a/teams", `[{"id":11,"slug":"platform"}]`},
		{"orgs/org-b/teams", `[{"id":22,"slug":"sre"}]`},
	}}}
}

func multiOwnerRepos() []*manifest.Repository {
	repo := func(owner, name, team string) *manifest.Repository {
		return &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Owner: owner, Name: name},
			Spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
				Name:         "main",
				BypassActors: []manifest.RulesetBypassActor{{Team: team, BypassMode: "always"}},
			}}},
		}
	}
	return []*manifest.Repository{repo("org-a", "app", "platform"), repo("org-b", "svc", "sre")}
}

func TestPlan_ResolvesNamesAgainstEachRepositoryOwner(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		runner := multiOwnerRunner()
		repos := multiOwnerRepos()
		if reversed {
			slices.Reverse(repos)
		}
		p := NewProcessor(runner, manifest.NewResolver(runner))
		changes, _, err := p.Plan(context.Background(), repos, PlanOptions{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 0 {
			t.Errorf("reversed=%v: rulesets match GitHub, got changes %+v", reversed, changes)
		}
		want := []string{"api orgs/org-a/teams/platform", "api orgs/org-b/teams/sre"}
		if got := runner.teamLookups(); !slices.Equal(got, want) {
			t.Errorf("reversed=%v: team lookups = %v, want %v", reversed, got, want)
		}
	}
}

func TestToManifest_ReversesNamesAgainstTheRepositoryOwner(t *testing.T) {
	runner := multiOwnerRunner()
	current := &CurrentState{Owner: "org-b", Name: "svc", Rulesets: map[string]*CurrentRuleset{
		"main": {Name: "main", BypassActors: []CurrentRulesetBypassActor{{ActorID: 22, ActorType: "Team", BypassMode: "always"}}},
	}}
	m := ToManifest(context.Background(), current, manifest.NewResolver(runner))
	if got := m.Spec.Rulesets[0].BypassActors[0].Team; got != "sre" {
		t.Errorf("team = %q, want sre from org-b", got)
	}
	if got := runner.teamLookups(); len(got) != 1 || !strings.HasPrefix(got[0], "api orgs/org-b/teams") {
		t.Errorf("team lookups = %v, want only org-b", got)
	}
}
