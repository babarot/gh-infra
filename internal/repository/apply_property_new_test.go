package repository

import (
	"context"
	"slices"
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
		// noSecretScanning: the owner lacks GitHub Secret Protection.
		noSecretScanning bool
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
		// Push protection requires secret scanning: the create enables
		// scanning first.
		{name: "secret scanning", spec: manifest.RepositorySpec{
			Visibility: ptr("public"),
			Security:   &manifest.Security{SecretScanning: bptr(true), SecretScanningPushProtection: bptr(true)},
		}},
		// A new repository has both off, so nothing is sent: where GitHub
		// does not offer them, it refuses even a disable.
		{name: "secret scanning off where GitHub does not offer it", noSecretScanning: true, spec: manifest.RepositorySpec{
			Visibility: ptr("private"),
			Security:   &manifest.Security{SecretScanning: bptr(false), SecretScanningPushProtection: bptr(false)},
		}},
		// A new public repository has both on, so turning them off is sent.
		{name: "secret scanning off on a public repository", spec: manifest.RepositorySpec{
			Visibility: ptr("public"),
			Security:   &manifest.Security{SecretScanning: bptr(false), SecretScanningPushProtection: bptr(false)},
		}},
		{name: "push protection off on a public repository", spec: manifest.RepositorySpec{
			Visibility: ptr("public"),
			Security:   &manifest.Security{SecretScanningPushProtection: bptr(false)},
		}},
		// A new private repository has the settings off: the manifest turns
		// some on, and the first has to be set for them to be saved.
		{name: "fork PR workflows", spec: manifest.RepositorySpec{
			Visibility: ptr("private"),
			Actions: &manifest.Actions{Enabled: bptr(true), ForkPRWorkflows: &manifest.ForkPRWorkflows{
				RunWorkflowsFromForkPullRequests: bptr(true), RequireApprovalForForkPRWorkflows: bptr(true),
			}},
		}},
		{name: "fork PR workflows off", spec: manifest.RepositorySpec{
			Visibility: ptr("internal"),
			Actions: &manifest.Actions{Enabled: bptr(true), ForkPRWorkflows: &manifest.ForkPRWorkflows{
				RunWorkflowsFromForkPullRequests: bptr(false), SendWriteTokensToWorkflows: bptr(false),
				SendSecretsAndVariables: bptr(false), RequireApprovalForForkPRWorkflows: bptr(false),
			}},
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
			fake.s.NoSecretScanning = tt.noSecretScanning
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

// Whether GitHub offers secret scanning on a repository that is not public
// is known only once it exists: the plan says so, and apply fails where it
// does not.
func TestCreateSecretScanningNotOffered(t *testing.T) {
	ctx := context.Background()
	fake := newMissingFakeGitHub("o", "r", nil)
	fake.s.NoSecretScanning = true
	p := NewProcessor(fake, manifest.NewResolver(fake))
	repo := &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec:     manifest.RepositorySpec{Security: &manifest.Security{SecretScanning: manifest.Ptr(true)}},
	}
	about := func(n string) bool { return strings.Contains(n, "secret scanning") }
	if notes := CreateNotes(repo); !slices.ContainsFunc(notes, about) {
		t.Errorf("create notes %v do not say that secret scanning may not be offered", notes)
	}

	changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := p.Apply(ctx, changes, targets, nil)
	if !slices.ContainsFunc(results, func(r ApplyResult) bool { return r.Err != nil && strings.Contains(r.Err.Error(), "secret_scanning") }) {
		t.Errorf("apply results %+v, want secret scanning failed", results)
	}

	// A disable is not sent where GitHub may not offer secret scanning, so
	// an owner's security configuration that turns it on shows in the next
	// plan.
	repo.Spec.Security.SecretScanning = manifest.Ptr(false)
	if notes := CreateNotes(repo); !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "disabling it is not sent") }) {
		t.Errorf("create notes %v do not say that disabling secret scanning is not sent", notes)
	}

	repo.Spec.Visibility = manifest.Ptr(manifest.VisibilityPublic)
	if notes := CreateNotes(repo); slices.ContainsFunc(notes, about) {
		t.Errorf("create notes %v for a public repository, which has secret scanning", notes)
	}
}
