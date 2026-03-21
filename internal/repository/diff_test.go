package repository

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// baseDesired returns a minimal desired manifest with owner/name set.
func baseDesired() *manifest.Repository {
	return &manifest.Repository{
		Metadata: manifest.RepositoryMetadata{
			Owner: "org",
			Name:  "repo",
		},
	}
}

// baseState returns a minimal current state with owner/name set.
func baseState() *CurrentState {
	return &CurrentState{
		Owner:            "org",
		Name:             "repo",
		BranchProtection: map[string]*CurrentBranchProtection{},
		Variables:        map[string]string{},
	}
}

func TestDiff_Noop(t *testing.T) {
	desired := baseDesired()
	current := baseState()

	changes := Diff(desired, current)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestDiff_RepoSettings(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(d *manifest.Repository, c *CurrentState)
		wantCount int
		wantField string
		wantType  ChangeType
		wantOld   interface{}
		wantNew   interface{}
	}{
		{
			name: "description change",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Description = manifest.Ptr("new desc")
				c.Description = "old desc"
			},
			wantCount: 1,
			wantField: "description",
			wantType:  ChangeUpdate,
			wantOld:   "old desc",
			wantNew:   "new desc",
		},
		{
			name: "description same no change",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Description = manifest.Ptr("same")
				c.Description = "same"
			},
			wantCount: 0,
		},
		{
			name: "homepage change",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Homepage = manifest.Ptr("https://new.example.com")
				c.Homepage = "https://old.example.com"
			},
			wantCount: 1,
			wantField: "homepage",
			wantType:  ChangeUpdate,
			wantOld:   "https://old.example.com",
			wantNew:   "https://new.example.com",
		},
		{
			name: "visibility change",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Visibility = manifest.Ptr("private")
				c.Visibility = "public"
			},
			wantCount: 1,
			wantField: "visibility",
			wantType:  ChangeUpdate,
			wantOld:   "public",
			wantNew:   "private",
		},
		{
			name: "topics add new topic",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Topics = []string{"go", "cli", "new"}
				c.Topics = []string{"go", "cli"}
			},
			wantCount: 1,
			wantField: "topics",
			wantType:  ChangeUpdate,
		},
		{
			name: "topics remove topic",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Topics = []string{"go"}
				c.Topics = []string{"go", "cli"}
			},
			wantCount: 1,
			wantField: "topics",
			wantType:  ChangeUpdate,
		},
		{
			name: "topics reorder is noop (sorted comparison)",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Topics = []string{"cli", "go"}
				c.Topics = []string{"go", "cli"}
			},
			wantCount: 0,
		},
		{
			name: "topics both empty is noop",
			setup: func(d *manifest.Repository, c *CurrentState) {
				d.Spec.Topics = nil
				c.Topics = nil
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			c := baseState()
			tt.setup(d, c)

			changes := diffRepoSettings("org/repo", d, c)
			if len(changes) != tt.wantCount {
				t.Fatalf("expected %d changes, got %d: %v", tt.wantCount, len(changes), changes)
			}
			if tt.wantCount == 0 {
				return
			}
			ch := changes[0]
			if ch.Field != tt.wantField {
				t.Errorf("expected field %q, got %q", tt.wantField, ch.Field)
			}
			if ch.Type != tt.wantType {
				t.Errorf("expected type %q, got %q", tt.wantType, ch.Type)
			}
			if tt.wantOld != nil && ch.OldValue != tt.wantOld {
				t.Errorf("expected old %v, got %v", tt.wantOld, ch.OldValue)
			}
			if tt.wantNew != nil && ch.NewValue != tt.wantNew {
				t.Errorf("expected new %v, got %v", tt.wantNew, ch.NewValue)
			}
		})
	}
}

func TestDiff_Features_NilFeatures(t *testing.T) {
	d := baseDesired()
	d.Spec.Features = nil
	c := baseState()

	changes := diffFeatures("org/repo", d, c)
	if len(changes) != 0 {
		t.Errorf("expected no changes when features is nil, got %d", len(changes))
	}
}

func TestDiff_Features_BoolFlags(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(f *manifest.Features, c *CurrentState)
		wantField string
	}{
		{
			name: "issues enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.Issues = manifest.Ptr(true)
				c.Features.Issues = false
			},
			wantField: "issues",
		},
		{
			name: "wiki disabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.Wiki = manifest.Ptr(false)
				c.Features.Wiki = true
			},
			wantField: "wiki",
		},
		{
			name: "projects enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.Projects = manifest.Ptr(true)
				c.Features.Projects = false
			},
			wantField: "projects",
		},
		{
			name: "discussions enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.Discussions = manifest.Ptr(true)
				c.Features.Discussions = false
			},
			wantField: "discussions",
		},
		{
			name: "merge_commit disabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.MergeCommit = manifest.Ptr(false)
				c.Features.MergeCommit = true
			},
			wantField: "merge_commit",
		},
		{
			name: "squash_merge enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.SquashMerge = manifest.Ptr(true)
				c.Features.SquashMerge = false
			},
			wantField: "squash_merge",
		},
		{
			name: "rebase_merge enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.RebaseMerge = manifest.Ptr(true)
				c.Features.RebaseMerge = false
			},
			wantField: "rebase_merge",
		},
		{
			name: "auto_delete_head_branches enabled",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.AutoDeleteHeadBranches = manifest.Ptr(true)
				c.Features.AutoDeleteHeadBranches = false
			},
			wantField: "auto_delete_head_branches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			d.Spec.Features = &manifest.Features{}
			c := baseState()
			tt.setup(d.Spec.Features, c)

			changes := diffFeatures("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
			}
			if changes[0].Field != tt.wantField {
				t.Errorf("expected field %q, got %q", tt.wantField, changes[0].Field)
			}
			if changes[0].Type != ChangeUpdate {
				t.Errorf("expected update, got %q", changes[0].Type)
			}
		})
	}
}

func TestDiff_Features_BoolNoChange(t *testing.T) {
	d := baseDesired()
	d.Spec.Features = &manifest.Features{
		Issues: manifest.Ptr(true),
	}
	c := baseState()
	c.Features.Issues = true

	changes := diffFeatures("org/repo", d, c)
	if len(changes) != 0 {
		t.Errorf("expected no changes when bool matches, got %d: %v", len(changes), changes)
	}
}

func TestDiff_Features_MergeCommitStrings(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(f *manifest.Features, c *CurrentState)
		wantField string
		wantOld   string
		wantNew   string
	}{
		{
			name: "squash_merge_commit_title change",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.SquashMergeCommitTitle = manifest.Ptr("PR_TITLE")
				c.Features.SquashMergeCommitTitle = "COMMIT_OR_PR_TITLE"
			},
			wantField: "squash_merge_commit_title",
			wantOld:   "COMMIT_OR_PR_TITLE",
			wantNew:   "PR_TITLE",
		},
		{
			name: "squash_merge_commit_message change",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.SquashMergeCommitMessage = manifest.Ptr("BLANK")
				c.Features.SquashMergeCommitMessage = "COMMIT_MESSAGES"
			},
			wantField: "squash_merge_commit_message",
			wantOld:   "COMMIT_MESSAGES",
			wantNew:   "BLANK",
		},
		{
			name: "merge_commit_title change",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.MergeCommitTitle = manifest.Ptr("PR_TITLE")
				c.Features.MergeCommitTitle = "MERGE_MESSAGE"
			},
			wantField: "merge_commit_title",
			wantOld:   "MERGE_MESSAGE",
			wantNew:   "PR_TITLE",
		},
		{
			name: "merge_commit_message change",
			setup: func(f *manifest.Features, c *CurrentState) {
				f.MergeCommitMessage = manifest.Ptr("BLANK")
				c.Features.MergeCommitMessage = "PR_BODY"
			},
			wantField: "merge_commit_message",
			wantOld:   "PR_BODY",
			wantNew:   "BLANK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			d.Spec.Features = &manifest.Features{}
			c := baseState()
			tt.setup(d.Spec.Features, c)

			changes := diffFeatures("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
			}
			ch := changes[0]
			if ch.Field != tt.wantField {
				t.Errorf("expected field %q, got %q", tt.wantField, ch.Field)
			}
			if ch.OldValue != tt.wantOld {
				t.Errorf("expected old %q, got %v", tt.wantOld, ch.OldValue)
			}
			if ch.NewValue != tt.wantNew {
				t.Errorf("expected new %q, got %v", tt.wantNew, ch.NewValue)
			}
		})
	}
}

func TestDiff_BranchProtection(t *testing.T) {
	t.Run("create new branch protection", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", RequiredReviews: manifest.Ptr(2)},
		}
		c := baseState()

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %q", changes[0].Type)
		}
		if changes[0].Resource != "BranchProtection[main]" {
			t.Errorf("expected resource BranchProtection[main], got %q", changes[0].Resource)
		}
	})

	t.Run("update required_reviews", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", RequiredReviews: manifest.Ptr(3)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{
			Pattern:         "main",
			RequiredReviews: 1,
		}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "required_reviews" {
			t.Errorf("expected field required_reviews, got %q", changes[0].Field)
		}
		if changes[0].OldValue != 1 {
			t.Errorf("expected old 1, got %v", changes[0].OldValue)
		}
		if changes[0].NewValue != 3 {
			t.Errorf("expected new 3, got %v", changes[0].NewValue)
		}
	})

	t.Run("update dismiss_stale_reviews", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", DismissStaleReviews: manifest.Ptr(true)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main", DismissStaleReviews: false}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "dismiss_stale_reviews" {
			t.Errorf("expected field dismiss_stale_reviews, got %q", changes[0].Field)
		}
	})

	t.Run("update enforce_admins", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", EnforceAdmins: manifest.Ptr(true)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main", EnforceAdmins: false}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "enforce_admins" {
			t.Errorf("expected field enforce_admins, got %q", changes[0].Field)
		}
	})

	t.Run("update allow_force_pushes", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", AllowForcePushes: manifest.Ptr(true)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main", AllowForcePushes: false}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "allow_force_pushes" {
			t.Errorf("expected field allow_force_pushes, got %q", changes[0].Field)
		}
	})

	t.Run("update allow_deletions", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", AllowDeletions: manifest.Ptr(true)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main", AllowDeletions: false}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "allow_deletions" {
			t.Errorf("expected field allow_deletions, got %q", changes[0].Field)
		}
	})

	t.Run("create status checks when current has none", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{
				Pattern: "main",
				RequireStatusChecks: &manifest.StatusChecks{
					Strict:   true,
					Contexts: []string{"ci/test"},
				},
			},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{
			Pattern:             "main",
			RequireStatusChecks: nil,
		}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %q", changes[0].Type)
		}
		if changes[0].Field != "require_status_checks" {
			t.Errorf("expected field require_status_checks, got %q", changes[0].Field)
		}
	})

	t.Run("update status checks strict", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{
				Pattern: "main",
				RequireStatusChecks: &manifest.StatusChecks{
					Strict:   true,
					Contexts: []string{"ci/test"},
				},
			},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{
			Pattern: "main",
			RequireStatusChecks: &CurrentStatusChecks{
				Strict:   false,
				Contexts: []string{"ci/test"},
			},
		}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "require_status_checks.strict" {
			t.Errorf("expected field require_status_checks.strict, got %q", changes[0].Field)
		}
	})

	t.Run("update status checks contexts", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{
				Pattern: "main",
				RequireStatusChecks: &manifest.StatusChecks{
					Strict:   true,
					Contexts: []string{"ci/test", "ci/lint"},
				},
			},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{
			Pattern: "main",
			RequireStatusChecks: &CurrentStatusChecks{
				Strict:   true,
				Contexts: []string{"ci/test"},
			},
		}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Field != "require_status_checks.contexts" {
			t.Errorf("expected field require_status_checks.contexts, got %q", changes[0].Field)
		}
	})

	t.Run("no change when existing protection matches", func(t *testing.T) {
		d := baseDesired()
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", RequiredReviews: manifest.Ptr(2), EnforceAdmins: manifest.Ptr(true)},
		}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{
			Pattern:         "main",
			RequiredReviews: 2,
			EnforceAdmins:   true,
		}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 0 {
			t.Errorf("expected no changes, got %d: %v", len(changes), changes)
		}
	})
}

func TestDiff_Secrets(t *testing.T) {
	t.Run("new secret", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Secrets = []manifest.Secret{
			{Name: "API_KEY", Value: "secret-value"},
		}
		c := baseState()
		c.Secrets = []string{}

		changes := diffSecrets("org/repo", d, c, false)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %q", changes[0].Type)
		}
		if changes[0].Field != "API_KEY" {
			t.Errorf("expected field API_KEY, got %q", changes[0].Field)
		}
		if changes[0].NewValue != "(new)" {
			t.Errorf("expected new value (new), got %v", changes[0].NewValue)
		}
	})

	t.Run("existing secret skipped without force", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Secrets = []manifest.Secret{
			{Name: "API_KEY", Value: "new-value"},
		}
		c := baseState()
		c.Secrets = []string{"API_KEY"}

		changes := diffSecrets("org/repo", d, c, false)
		if len(changes) != 0 {
			t.Errorf("expected no changes without force, got %d: %v", len(changes), changes)
		}
	})

	t.Run("existing secret updated with force", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Secrets = []manifest.Secret{
			{Name: "API_KEY", Value: "new-value"},
		}
		c := baseState()
		c.Secrets = []string{"API_KEY"}

		changes := diffSecrets("org/repo", d, c, true)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeUpdate {
			t.Errorf("expected update, got %q", changes[0].Type)
		}
	})

	t.Run("no desired secrets produces no changes", func(t *testing.T) {
		d := baseDesired()
		c := baseState()
		c.Secrets = []string{"EXISTING"}

		changes := diffSecrets("org/repo", d, c, false)
		if len(changes) != 0 {
			t.Errorf("expected no changes, got %d", len(changes))
		}
	})
}

func TestDiff_Variables(t *testing.T) {
	t.Run("new variable", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Variables = []manifest.Variable{
			{Name: "ENV", Value: "production"},
		}
		c := baseState()

		changes := diffVariables("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %q", changes[0].Type)
		}
		if changes[0].Field != "ENV" {
			t.Errorf("expected field ENV, got %q", changes[0].Field)
		}
		if changes[0].NewValue != "production" {
			t.Errorf("expected new value production, got %v", changes[0].NewValue)
		}
	})

	t.Run("update variable value", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Variables = []manifest.Variable{
			{Name: "ENV", Value: "production"},
		}
		c := baseState()
		c.Variables["ENV"] = "staging"

		changes := diffVariables("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeUpdate {
			t.Errorf("expected update, got %q", changes[0].Type)
		}
		if changes[0].OldValue != "staging" {
			t.Errorf("expected old staging, got %v", changes[0].OldValue)
		}
		if changes[0].NewValue != "production" {
			t.Errorf("expected new production, got %v", changes[0].NewValue)
		}
	})

	t.Run("variable same value no change", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Variables = []manifest.Variable{
			{Name: "ENV", Value: "production"},
		}
		c := baseState()
		c.Variables["ENV"] = "production"

		changes := diffVariables("org/repo", d, c)
		if len(changes) != 0 {
			t.Errorf("expected no changes, got %d", len(changes))
		}
	})
}

func TestDiff_FullIntegration(t *testing.T) {
	t.Run("multiple changes across categories", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Description = manifest.Ptr("updated")
		d.Spec.Visibility = manifest.Ptr("private")
		d.Spec.Features = &manifest.Features{
			Issues: manifest.Ptr(false),
		}
		d.Spec.BranchProtection = []manifest.BranchProtection{
			{Pattern: "main", RequiredReviews: manifest.Ptr(2)},
		}
		d.Spec.Secrets = []manifest.Secret{
			{Name: "TOKEN", Value: "val"},
		}
		d.Spec.Variables = []manifest.Variable{
			{Name: "REGION", Value: "us-east-1"},
		}

		c := baseState()
		c.Description = "old"
		c.Visibility = "public"
		c.Features.Issues = true

		changes := Diff(d, c)

		// description + visibility + issues + branch protection (create) + secret (create) + variable (create)
		if len(changes) != 6 {
			t.Errorf("expected 6 changes, got %d: %v", len(changes), changes)
		}
	})
}

// ---------------------------------------------------------------------------
// Change.String() tests
// ---------------------------------------------------------------------------

func TestChange_String(t *testing.T) {
	tests := []struct {
		name string
		c    Change
		want string
	}{
		{
			name: "create",
			c:    Change{Type: ChangeCreate, Field: "description", NewValue: "new"},
			want: "+ description",
		},
		{
			name: "delete",
			c:    Change{Type: ChangeDelete, Field: "homepage", OldValue: "old"},
			want: "- homepage",
		},
		{
			name: "update",
			c:    Change{Type: ChangeUpdate, Field: "visibility", OldValue: "public", NewValue: "private"},
			want: "~ visibility: public → private",
		},
		{
			name: "noop returns empty",
			c:    Change{Type: ChangeNoOp, Field: "description"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.c.String()
			if got != tt.want {
				t.Errorf("Change.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Result.HasChanges() tests
// ---------------------------------------------------------------------------

func TestResult_HasChanges(t *testing.T) {
	t.Run("with changes", func(t *testing.T) {
		r := &Result{
			Changes: []Change{
				{Type: ChangeNoOp},
				{Type: ChangeUpdate, Field: "description"},
			},
		}
		if !r.HasChanges() {
			t.Error("expected HasChanges() = true")
		}
	})

	t.Run("all noop", func(t *testing.T) {
		r := &Result{
			Changes: []Change{
				{Type: ChangeNoOp},
				{Type: ChangeNoOp},
			},
		}
		if r.HasChanges() {
			t.Error("expected HasChanges() = false")
		}
	})

	t.Run("empty", func(t *testing.T) {
		r := &Result{}
		if r.HasChanges() {
			t.Error("expected HasChanges() = false for empty result")
		}
	})
}

// ---------------------------------------------------------------------------
// Result.Summary() tests
// ---------------------------------------------------------------------------

func TestResult_Summary(t *testing.T) {
	r := &Result{
		Changes: []Change{
			{Type: ChangeCreate},
			{Type: ChangeCreate},
			{Type: ChangeUpdate},
			{Type: ChangeDelete},
			{Type: ChangeNoOp},
		},
	}

	creates, updates, deletes := r.Summary()
	if creates != 2 {
		t.Errorf("creates = %d, want 2", creates)
	}
	if updates != 1 {
		t.Errorf("updates = %d, want 1", updates)
	}
	if deletes != 1 {
		t.Errorf("deletes = %d, want 1", deletes)
	}
}

func TestResult_Summary_Empty(t *testing.T) {
	r := &Result{}
	creates, updates, deletes := r.Summary()
	if creates != 0 || updates != 0 || deletes != 0 {
		t.Errorf("expected all zeros, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}
}

func TestStringSliceEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []string{}, []string{}, true},
		{"nil vs empty", nil, []string{}, true},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different order", []string{"b", "a"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a", "c"}, []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSliceEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("stringSliceEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
