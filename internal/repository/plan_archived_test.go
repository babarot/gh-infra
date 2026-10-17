package repository

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// errTracker records the errors reported per target.
type errTracker struct {
	mu     sync.Mutex
	errors map[string][]error
}

func (t *errTracker) UpdateStatus(string, string) {}
func (t *errTracker) Checkpoint(string, string)   {}
func (t *errTracker) Done(string, PlanSummary)    {}
func (t *errTracker) Error(name string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.errors == nil {
		t.errors = map[string][]error{}
	}
	t.errors[name] = append(t.errors[name], err)
}

// A repository that is already archived refuses every write, so changes to
// it are reported at plan, as a failure of that repository only.
func TestPlan_ChangesToAnArchivedRepository(t *testing.T) {
	ptr := manifest.Ptr[string]
	bptr := manifest.Ptr[bool]
	for _, tt := range []struct {
		name    string
		spec    manifest.RepositorySpec
		wantErr string // "" means planned as usual
	}{
		{name: "a change, archived kept", spec: manifest.RepositorySpec{Archived: bptr(true), Description: ptr("new")},
			wantErr: "description"},
		{name: "a change, archived not managed", spec: manifest.RepositorySpec{Topics: []string{"new"}, TopicsSet: true},
			wantErr: "topics"},
		{name: "many changes", spec: manifest.RepositorySpec{
			Description: ptr("new"), Homepage: ptr(""),
			Labels:    []manifest.Label{{Name: "new", Color: "00ff00"}},
			Variables: []manifest.Variable{{Name: "NEW", Value: "x"}},
		}, wantErr: "and 1 more"},
		{name: "no change", spec: manifest.RepositorySpec{Archived: bptr(true), Description: ptr("a repository")}},
		{name: "unarchived first", spec: manifest.RepositorySpec{Archived: bptr(false), Description: ptr("new")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propState()
			s.Repo.Archived = true
			fake := newFakeGitHub("o", "r", s)
			p := NewProcessor(fake, manifest.NewResolver(fake))
			tracker := &errTracker{}
			repo := &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "r"}, Spec: tt.spec}

			changes, targets, err := p.Plan(context.Background(), []*manifest.Repository{repo}, PlanOptions{}, tracker)
			if err != nil {
				t.Fatalf("plan: %v, want the repository reported, not a failed plan", err)
			}
			errs := tracker.errors["o/r"]
			if tt.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("errors = %v, want none", errs)
				}
				for _, r := range p.Apply(context.Background(), changes, targets, nil) {
					if r.Err != nil {
						t.Errorf("apply %s: %v", r.Change.Field, r.Err)
					}
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "archived") || !strings.Contains(errs[0].Error(), tt.wantErr) {
				t.Errorf("errors = %v, want one about the archived repository naming %q", errs, tt.wantErr)
			}
			if len(changes) != 0 || len(targets) != 0 {
				t.Errorf("planned %s for an archived repository", formatChanges(changes))
			}
			if len(fake.writes) != 0 {
				t.Errorf("wrote to an archived repository: %q", fake.writes)
			}
		})
	}
}
