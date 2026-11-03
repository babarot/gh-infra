package repository

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

func pagesRepo(p *manifest.Pages) *manifest.Repository {
	return &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"},
		Spec:     manifest.RepositorySpec{Pages: p},
	}
}

// https_enforced: true waits for GitHub to issue the site's certificate,
// which it does in its own time after the site has its domain. The plan
// shows it with a note, apply sends nothing for it, and plans keep showing
// it until the certificate is issued; then it applies and converges.
func TestPagesHTTPSWaitsForTheCertificate(t *testing.T) {
	ptr := manifest.Ptr[string]
	ctx := context.Background()
	for _, tt := range []struct {
		name  string
		state func(*fakeState)
		pages *manifest.Pages
	}{
		{name: "a new site", pages: &manifest.Pages{BuildType: ptr("workflow"), CNAME: ptr("docs.example.com"), HTTPSEnforced: manifest.Ptr(true)}},
		{name: "a new domain", state: func(s *fakeState) {
			s.Pages = propPages()
			s.Pages.HTTPSEnforced = false
		}, pages: &manifest.Pages{CNAME: ptr("www.example.com"), HTTPSEnforced: manifest.Ptr(true)}},
		{name: "a certificate not issued yet", state: func(s *fakeState) {
			s.Pages = propPages()
			s.Pages.HTTPSEnforced, s.Pages.CertificateState = false, "authorization_pending"
		}, pages: &manifest.Pages{HTTPSEnforced: manifest.Ptr(true)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propState()
			if tt.state != nil {
				tt.state(&s)
			}
			fake := newFakeGitHub("o", "r", s)
			p := NewProcessor(fake, manifest.NewResolver(fake))
			repo := pagesRepo(tt.pages)

			deferred := func(changes []Change) bool {
				return len(changes) == 1 && hasDetail(changes[0], func(d Change) bool {
					return d.Field == pagesFieldHTTPSEnforced && d.NewValue == pagesHTTPSDeferred
				})
			}
			changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !deferred(changes) {
				t.Fatalf("plan does not show https_enforced waiting: %s", formatPagesChanges(changes))
			}
			if notes := ChangeNotes(changes); len(notes) != 1 || !strings.Contains(notes[0], "certificate") {
				t.Errorf("notes = %v, want one about the certificate", notes)
			}
			for _, r := range p.Apply(ctx, changes, targets, nil) {
				if r.Err != nil {
					t.Fatalf("apply: %v", r.Err)
				}
			}
			if fake.state().Pages.HTTPSEnforced {
				t.Fatal("apply enforced HTTPS before the certificate was issued")
			}

			// Until GitHub issues the certificate, only https_enforced is left.
			again, _, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !deferred(again) || len(again[0].Details) != 1 {
				t.Fatalf("plan after apply: %s, want https_enforced waiting only", formatPagesChanges(again))
			}

			fake.mu.Lock()
			fake.s.Pages.CertificateState = certificateApproved
			fake.mu.Unlock()
			checkApplyProperties(t, fake.state(), repo, false)
		})
	}
}

func formatPagesChanges(changes []Change) string {
	var parts []string
	for _, c := range changes {
		for _, d := range c.Details {
			parts = append(parts, d.Field+"="+strings.TrimSpace(strings.ReplaceAll(formatChanges([]Change{d}), "\n", " ")))
		}
	}
	return strings.Join(parts, "; ")
}

// What GitHub would refuse fails the plan, not the apply.
func TestPagesPlanErrors(t *testing.T) {
	ptr := manifest.Ptr[string]
	for _, tt := range []struct {
		name  string
		state func(*fakeState)
		pages *manifest.Pages
		want  string // "" for no error
	}{
		{name: "enabling without a build type", pages: &manifest.Pages{CNAME: ptr("docs.example.com")}, want: "set pages.build_type"},
		{name: "a source branch that does not exist", state: func(s *fakeState) { s.Branches = []string{"main"} },
			pages: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages"}}, want: `branch "gh-pages" does not exist`},
		{name: "a source branch that does not exist, unchanged", state: func(s *fakeState) {
			s.Pages = &fakePages{BuildType: "legacy", Source: fakePagesSource{Branch: "gh-pages", Path: "/"}}
		}, pages: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages"}}},
		{name: "private on GitHub Free", state: func(s *fakeState) {
			s.Repo.Visibility = "private"
			s.Org.Plan = "free"
		}, pages: &manifest.Pages{BuildType: ptr("workflow")}, want: "allows Pages only on public repositories"},
		{name: "private with a plan that cannot be read", state: func(s *fakeState) {
			s.Repo.Visibility = "private"
			s.Org.PlanHidden = true
		}, pages: &manifest.Pages{BuildType: ptr("workflow")}, want: "cannot tell whether the plan"},
		{name: "private on GitHub Pro", state: func(s *fakeState) {
			s.Repo.Visibility = "private"
			s.Org.Plan = "pro"
		}, pages: &manifest.Pages{BuildType: ptr("workflow")}},
		// A site that exists shows the plan allows it: the plan is not read.
		{name: "private with a site", state: func(s *fakeState) {
			s.Repo.Visibility = "private"
			s.Org.PlanHidden = true
			s.Pages = propPages()
		}, pages: &manifest.Pages{CNAME: ptr("www.example.com")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propState()
			if tt.state != nil {
				tt.state(&s)
			}
			fake := newFakeGitHub("o", "r", s)
			p := NewProcessor(fake, manifest.NewResolver(fake))
			changes, targets, err := p.Plan(context.Background(), []*manifest.Repository{pagesRepo(tt.pages)}, PlanOptions{}, nil)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("plan error = %v, want %q", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range p.Apply(context.Background(), changes, targets, nil) {
				if r.Err != nil {
					t.Errorf("apply: %v", r.Err)
				}
			}
		})
	}
}

// A repository the run creates has no commits to build a site from: its
// Pages are left to a later plan, which the create says.
func TestPagesOfANewRepository(t *testing.T) {
	fake := newMissingFakeGitHub("o", "r", nil)
	p := NewProcessor(fake, manifest.NewResolver(fake))
	repo := pagesRepo(&manifest.Pages{BuildType: manifest.Ptr("workflow")})
	changes, _, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(changes, func(c Change) bool { return c.Resource == manifest.ResourcePages }) {
		t.Errorf("plan enables Pages of a repository it creates: %s", formatChanges(changes))
	}
	if notes := CreateNotes(repo); !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "pages") }) {
		t.Errorf("create notes %v do not say when Pages is enabled", notes)
	}
}

// A site that cannot be read fails the repository at plan rather than being
// planned as missing; import leaves it out.
func TestPagesUnreadable(t *testing.T) {
	forbidden := fakeStatus(gh.ErrForbidden, "Resource not accessible by personal access token")
	runner := &unreadableRunner{Runner: newFakeGitHub("o", "r", propState()), cmd: "api repos/o/r/pages", err: forbidden}
	p := NewProcessor(runner, manifest.NewResolver(runner))
	tracker := &errorTracker{errors: map[string]error{}}
	changes, _, err := p.Plan(context.Background(), []*manifest.Repository{pagesRepo(&manifest.Pages{BuildType: manifest.Ptr("workflow")})}, PlanOptions{}, tracker)
	if err != nil {
		t.Fatal(err)
	}
	if e := tracker.errors["o/r"]; e == nil || !strings.Contains(e.Error(), "Pages permission") || len(changes) > 0 {
		t.Errorf("error %v, changes %s; want the repository reported", e, formatChanges(changes))
	}

	current, err := NewProcessor(runner, nil).FetchRepository(context.Background(), "o", "r", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if current.Unread["pages"] == nil {
		t.Errorf("unread %v, want pages", current.Unread)
	}
	if spec := ToManifest(context.Background(), current, nil).Spec; spec.Pages != nil {
		t.Errorf("import writes the site it could not read: %+v", spec.Pages)
	}
}
