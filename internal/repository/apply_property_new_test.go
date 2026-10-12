package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// A repository that does not exist yet: every planned change applies, and the
// next plan shows only what CreateNotes leaves to it (branch protection, and
// the deletion of default labels under authoritative reconcile).
func TestApplyPropertiesNewRepository(t *testing.T) {
	ptr := manifest.Ptr[string]
	bptr := manifest.Ptr[bool]
	iptr := manifest.Ptr[int]
	authoritative := manifest.Ptr(manifest.CollectionReconcileAuthoritative)
	orgLabels := []fakeLabel{{Name: "triage", Color: "000000", Description: "org default"}}

	for _, tt := range []struct {
		name          string
		spec          manifest.RepositorySpec
		recon         *manifest.RepositoryReconcile
		defaultLabels []fakeLabel
		// deferred is what the second plan may still show.
		deferred func(Change) bool
	}{
		{name: "settings only", spec: manifest.RepositorySpec{
			Visibility: ptr("private"), Description: ptr("new"), Topics: []string{"go"},
			Features: &manifest.Features{Wiki: bptr(false), Discussions: bptr(true)},
		}},
		{name: "labels next to the defaults", spec: manifest.RepositorySpec{Labels: []manifest.Label{
			{Name: "bug", Color: "ff0000"},                                    // a default: updated, description kept
			{Name: "Documentation", Color: "0075ca", Description: ptr("")},    // a default in another case: renamed, description cleared
			{Name: "feature", Color: "00ff00", Description: ptr("new thing")}, // not a default: created
		}}},
		{name: "labels authoritative", recon: &manifest.RepositoryReconcile{Labels: authoritative},
			spec:     manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "ff0000"}}},
			deferred: func(c Change) bool { return c.Resource == manifest.ResourceLabel && c.Type == ChangeDelete }},
		{name: "an organization's default labels", defaultLabels: orgLabels,
			spec: manifest.RepositorySpec{Labels: []manifest.Label{{Name: "triage", Color: "111111"}, {Name: "bug", Color: "d73a4a"}}}},
		{name: "milestones", spec: manifest.RepositorySpec{
			Features:   &manifest.Features{Issues: bptr(false)},
			Milestones: []manifest.Milestone{{Title: "v1"}, {Title: "v2", Description: ptr("second"), DueOn: ptr("2026-12-31")}},
		}},
		{name: "secrets and variables", spec: manifest.RepositorySpec{
			Secrets:   []manifest.Secret{{Name: "TOKEN", Value: "s3cret"}},
			Variables: []manifest.Variable{{Name: "ENV", Value: "dev"}},
		}},
		{name: "rulesets", spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
			Name: "protect-main", Enforcement: ptr("active"),
			BypassActors: []manifest.RulesetBypassActor{{Role: "admin", BypassMode: "always"}}, BypassActorsSet: true,
			Rules: manifest.RulesetRules{
				Deletion:    bptr(true),
				PullRequest: &manifest.RulesetPullRequest{RequiredApprovingReviewCount: iptr(1)},
				RequiredStatusChecks: &manifest.RulesetStatusChecks{
					Contexts: []manifest.RulesetStatusCheck{{Context: "ci", App: "ci"}},
				},
			},
		}}}},
		{name: "branch protection waits for a branch",
			spec: manifest.RepositorySpec{BranchProtection: []manifest.BranchProtection{{Pattern: "main", RequiredReviews: iptr(1)}}},
			deferred: func(c Change) bool {
				return strings.HasPrefix(c.Resource, manifest.ResourceBranchProtection) && c.Type == ChangeCreate
			}},
		{name: "archived", spec: manifest.RepositorySpec{
			Archived: bptr(true), Labels: []manifest.Label{{Name: "feature", Color: "00ff00"}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			labels := tt.defaultLabels
			if labels == nil {
				labels = githubDefaultLabels
			}
			fake := newMissingFakeGitHub("o", "r", labels)
			fake.s.Org = propRulesState().Org
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Reconcile: tt.recon,
				Spec:      tt.spec,
			}

			changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) == 0 || !isRepositoryCreate(changes[0]) {
				t.Fatalf("plan = %s, want the create first", formatChanges(changes))
			}
			// An archive goes last, after everything else of the run.
			var others, archives []Change
			for _, c := range changes {
				if IsArchive(c) {
					archives = append(archives, c)
				} else {
					others = append(others, c)
				}
			}
			results := append(p.Apply(ctx, others, targets, nil), p.Apply(ctx, archives, targets, nil)...)
			for _, r := range results {
				if r.Err != nil {
					t.Errorf("apply %s %s: %v\nwrites: %s", r.Change.Resource, r.Change.Field, r.Err, strings.Join(fake.writes, "\n  "))
				}
			}

			next, _, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range next {
				if tt.deferred == nil || !tt.deferred(c) {
					t.Errorf("the next plan shows %s %s %s, which the create should have applied", c.Type, c.Resource, c.Field)
				}
			}
			if tt.deferred != nil && len(next) == 0 {
				t.Error("the next plan shows nothing, but the create leaves something to it")
			}
		})
	}
}
