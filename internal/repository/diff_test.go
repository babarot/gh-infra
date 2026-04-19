package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
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
		Rulesets:         map[string]*CurrentRuleset{},
		Variables:        map[string]string{},
		Labels:           map[string]*CurrentLabel{},
		Milestones:       map[string]*CurrentMilestone{},
	}
}

func TestDiff_Noop(t *testing.T) {
	desired := baseDesired()
	current := baseState()

	changes := Diff(context.Background(), desired, current)
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
		wantOld   any
		wantNew   any
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			d.Spec.Features = &manifest.Features{}
			c := baseState()
			tt.setup(d.Spec.Features, c)

			changes := diffFeatures("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 parent change, got %d: %v", len(changes), changes)
			}
			parent := changes[0]
			if parent.Field != "features" {
				t.Fatalf("expected parent field features, got %q", parent.Field)
			}
			if len(parent.Details) != 1 {
				t.Fatalf("expected 1 child, got %d", len(parent.Details))
			}
			if len(parent.Children) != 1 {
				t.Fatalf("expected 1 apply child, got %d", len(parent.Children))
			}
			child := parent.Details[0]
			applyChild := parent.Children[0]
			if child.Field != tt.wantField {
				t.Errorf("expected field %q, got %q", tt.wantField, child.Field)
			}
			if applyChild.Field != tt.wantField {
				t.Errorf("expected apply field %q, got %q", tt.wantField, applyChild.Field)
			}
			if child.Type != ChangeUpdate {
				t.Errorf("expected update, got %q", child.Type)
			}
		})
	}
}

func TestDiff_MergeStrategy_NilMergeStrategy(t *testing.T) {
	d := baseDesired()
	d.Spec.MergeStrategy = nil
	c := baseState()

	changes := diffMergeStrategy("org/repo", d, c)
	if len(changes) != 0 {
		t.Errorf("expected no changes when merge_strategy is nil, got %d", len(changes))
	}
}

func TestDiff_MergeStrategy_BoolFlags(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(ms *manifest.MergeStrategy, c *CurrentState)
		wantField string
	}{
		{
			name: "allow_merge_commit disabled",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.AllowMergeCommit = manifest.Ptr(false)
				c.MergeStrategy.AllowMergeCommit = true
			},
			wantField: "allow_merge_commit",
		},
		{
			name: "allow_squash_merge enabled",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.AllowSquashMerge = manifest.Ptr(true)
				c.MergeStrategy.AllowSquashMerge = false
			},
			wantField: "allow_squash_merge",
		},
		{
			name: "allow_rebase_merge enabled",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.AllowRebaseMerge = manifest.Ptr(true)
				c.MergeStrategy.AllowRebaseMerge = false
			},
			wantField: "allow_rebase_merge",
		},
		{
			name: "allow_auto_merge enabled",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.AllowAutoMerge = manifest.Ptr(true)
				c.MergeStrategy.AllowAutoMerge = false
			},
			wantField: "allow_auto_merge",
		},
		{
			name: "auto_delete_head_branches enabled",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.AutoDeleteHeadBranches = manifest.Ptr(true)
				c.MergeStrategy.AutoDeleteHeadBranches = false
			},
			wantField: "auto_delete_head_branches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			d.Spec.MergeStrategy = &manifest.MergeStrategy{}
			c := baseState()
			tt.setup(d.Spec.MergeStrategy, c)

			changes := diffMergeStrategy("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 parent change, got %d: %v", len(changes), changes)
			}
			parent := changes[0]
			if parent.Field != "merge_strategy" {
				t.Fatalf("expected parent field merge_strategy, got %q", parent.Field)
			}
			if len(parent.Details) != 1 {
				t.Fatalf("expected 1 child, got %d", len(parent.Details))
			}
			child := parent.Details[0]
			if child.Field != tt.wantField {
				t.Errorf("expected field %q, got %q", tt.wantField, child.Field)
			}
			if child.Type != ChangeUpdate {
				t.Errorf("expected update, got %q", child.Type)
			}
		})
	}
}

func TestValidateDependencies(t *testing.T) {
	tests := []struct {
		name        string
		setupSpec   func(s *manifest.Security)
		currentVA   bool
		wantErr     bool
		wantErrText string
	}{
		{
			name:      "no security: ok",
			setupSpec: nil,
			wantErr:   false,
		},
		{
			name:      "fixes nil: ok",
			setupSpec: func(s *manifest.Security) {},
			wantErr:   false,
		},
		{
			name:      "fixes false: ok regardless of alerts",
			setupSpec: func(s *manifest.Security) { s.AutomatedSecurityFixes = manifest.Ptr(false) },
			wantErr:   false,
		},
		{
			name: "fixes true + alerts true (manifest): ok",
			setupSpec: func(s *manifest.Security) {
				s.AutomatedSecurityFixes = manifest.Ptr(true)
				s.VulnerabilityAlerts = manifest.Ptr(true)
			},
			wantErr: false,
		},
		{
			name: "fixes true + alerts false (manifest): error",
			setupSpec: func(s *manifest.Security) {
				s.AutomatedSecurityFixes = manifest.Ptr(true)
				s.VulnerabilityAlerts = manifest.Ptr(false)
			},
			wantErr:     true,
			wantErrText: "automated_security_fixes",
		},
		{
			name: "fixes true + alerts omitted, current=true: ok (effective true)",
			setupSpec: func(s *manifest.Security) {
				s.AutomatedSecurityFixes = manifest.Ptr(true)
			},
			currentVA: true,
			wantErr:   false,
		},
		{
			name: "fixes true + alerts omitted, current=false: error",
			setupSpec: func(s *manifest.Security) {
				s.AutomatedSecurityFixes = manifest.Ptr(true)
			},
			currentVA:   false,
			wantErr:     true,
			wantErrText: "automated_security_fixes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &manifest.Repository{
				Spec: manifest.RepositorySpec{},
			}
			if tt.setupSpec != nil {
				d.Spec.Security = &manifest.Security{}
				tt.setupSpec(d.Spec.Security)
			}
			c := &CurrentState{Security: CurrentSecurity{VulnerabilityAlerts: tt.currentVA}}

			err := ValidateDependencies(d, c)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErrText, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateDependencies_ForkPRApprovalUnsupportedForPrivateRepo(t *testing.T) {
	tests := []struct {
		name       string
		visibility *string
		current    CurrentState
		wantErr    bool
	}{
		{
			name:       "explicit private",
			visibility: manifest.Ptr(manifest.VisibilityPrivate),
			current:    CurrentState{Visibility: manifest.VisibilityPublic},
			wantErr:    true,
		},
		{
			name:    "current private with omitted visibility",
			current: CurrentState{Visibility: manifest.VisibilityPrivate},
			wantErr: true,
		},
		{
			name:    "new repo defaults to private",
			current: CurrentState{IsNew: true},
			wantErr: true,
		},
		{
			name:       "explicit public on current private",
			visibility: manifest.Ptr(manifest.VisibilityPublic),
			current:    CurrentState{Visibility: manifest.VisibilityPrivate},
			wantErr:    false,
		},
		{
			name:    "current public with omitted visibility",
			current: CurrentState{Visibility: manifest.VisibilityPublic},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &manifest.Repository{
				Spec: manifest.RepositorySpec{
					Visibility: tt.visibility,
					Actions: &manifest.Actions{
						ForkPRApproval: manifest.Ptr("first_time_contributors"),
					},
				},
			}

			err := ValidateDependencies(d, &tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "fork_pr_approval") || !strings.Contains(err.Error(), "private") {
					t.Errorf("expected fork_pr_approval/private error, got: %v", err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestDiff_Security(t *testing.T) {
	t.Run("nil security is noop", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Security = nil
		c := baseState()
		if changes := diffSecurity("org/repo", d, c); len(changes) != 0 {
			t.Errorf("expected no changes when security is nil, got %d", len(changes))
		}
	})

	fields := []struct {
		name      string
		setDesire func(s *manifest.Security)
		setCur    func(c *CurrentState)
		field     string
	}{
		{
			name:      "vulnerability_alerts",
			setDesire: func(s *manifest.Security) { s.VulnerabilityAlerts = manifest.Ptr(true) },
			setCur:    func(c *CurrentState) { c.Security.VulnerabilityAlerts = false },
			field:     "vulnerability_alerts",
		},
		{
			name:      "automated_security_fixes",
			setDesire: func(s *manifest.Security) { s.AutomatedSecurityFixes = manifest.Ptr(true) },
			setCur:    func(c *CurrentState) { c.Security.AutomatedSecurityFixes = false },
			field:     "automated_security_fixes",
		},
		{
			name:      "private_vulnerability_reporting",
			setDesire: func(s *manifest.Security) { s.PrivateVulnerabilityReporting = manifest.Ptr(true) },
			setCur:    func(c *CurrentState) { c.Security.PrivateVulnerabilityReporting = false },
			field:     "private_vulnerability_reporting",
		},
	}

	for _, f := range fields {
		t.Run(f.name+" enable", func(t *testing.T) {
			d := baseDesired()
			d.Spec.Security = &manifest.Security{}
			f.setDesire(d.Spec.Security)
			c := baseState()
			f.setCur(c)

			changes := diffSecurity("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 parent change, got %d", len(changes))
			}
			if changes[0].Field != "security" {
				t.Errorf("expected parent field 'security', got %q", changes[0].Field)
			}
			if len(changes[0].Details) != 1 {
				t.Fatalf("expected 1 child, got %d", len(changes[0].Details))
			}
			child := changes[0].Details[0]
			if child.Field != f.field {
				t.Errorf("expected child field %q, got %q", f.field, child.Field)
			}
			if child.NewValue != true {
				t.Errorf("expected NewValue=true, got %v", child.NewValue)
			}
		})
	}

	t.Run("multiple fields produce multiple children", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Security = &manifest.Security{
			VulnerabilityAlerts:           manifest.Ptr(true),
			AutomatedSecurityFixes:        manifest.Ptr(true),
			PrivateVulnerabilityReporting: manifest.Ptr(true),
		}
		c := baseState()

		changes := diffSecurity("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 parent change, got %d", len(changes))
		}
		if got := len(changes[0].Details); got != 3 {
			t.Errorf("expected 3 children, got %d", got)
		}
	})
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

func TestDiff_Features_PullRequests(t *testing.T) {
	d := baseDesired()
	d.Spec.Features = &manifest.Features{
		PullRequests: &manifest.PullRequests{Enabled: manifest.Ptr(false)},
	}
	c := baseState()
	c.Features.PullRequests = true

	changes := diffFeatures("org/repo", d, c)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	child := changes[0].Details
	if len(child) != 1 || child[0].Field != "pull_requests" {
		t.Errorf("expected pull_requests child change, got %+v", child)
	}
}

func TestDiff_Features_PullRequestsCreation(t *testing.T) {
	d := baseDesired()
	d.Spec.Features = &manifest.Features{
		PullRequests: &manifest.PullRequests{
			Enabled:  manifest.Ptr(true),
			Creation: manifest.Ptr(manifest.PullRequestCreationCollaboratorsOnly),
		},
	}
	c := baseState()
	c.Features.PullRequests = true
	c.Features.PullRequestCreation = manifest.PullRequestCreationAll

	changes := diffFeatures("org/repo", d, c)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	child := changes[0].Details
	if len(child) != 1 || child[0].Field != "pull_requests.creation" {
		t.Errorf("expected pull_requests.creation child change, got %+v", child)
	}
}

func TestDiff_Features_PullRequestsNoChange(t *testing.T) {
	d := baseDesired()
	d.Spec.Features = &manifest.Features{
		PullRequests: &manifest.PullRequests{Enabled: manifest.Ptr(true)},
	}
	c := baseState()
	c.Features.PullRequests = true

	changes := diffFeatures("org/repo", d, c)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d", len(changes))
	}
}

func TestDiff_MergeStrategy_CommitStrings(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(ms *manifest.MergeStrategy, c *CurrentState)
		wantField string
		wantOld   string
		wantNew   string
	}{
		{
			name: "squash_merge_commit_title change",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.SquashMergeCommitTitle = manifest.Ptr("PR_TITLE")
				c.MergeStrategy.SquashMergeCommitTitle = "COMMIT_OR_PR_TITLE"
			},
			wantField: "squash_merge_commit_title",
			wantOld:   "COMMIT_OR_PR_TITLE",
			wantNew:   "PR_TITLE",
		},
		{
			name: "squash_merge_commit_message change",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.SquashMergeCommitMessage = manifest.Ptr("BLANK")
				c.MergeStrategy.SquashMergeCommitMessage = "COMMIT_MESSAGES"
			},
			wantField: "squash_merge_commit_message",
			wantOld:   "COMMIT_MESSAGES",
			wantNew:   "BLANK",
		},
		{
			name: "merge_commit_title change",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.MergeCommitTitle = manifest.Ptr("PR_TITLE")
				c.MergeStrategy.MergeCommitTitle = "MERGE_MESSAGE"
			},
			wantField: "merge_commit_title",
			wantOld:   "MERGE_MESSAGE",
			wantNew:   "PR_TITLE",
		},
		{
			name: "merge_commit_message change",
			setup: func(ms *manifest.MergeStrategy, c *CurrentState) {
				ms.MergeCommitMessage = manifest.Ptr("BLANK")
				c.MergeStrategy.MergeCommitMessage = "PR_BODY"
			},
			wantField: "merge_commit_message",
			wantOld:   "PR_BODY",
			wantNew:   "BLANK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := baseDesired()
			d.Spec.MergeStrategy = &manifest.MergeStrategy{}
			c := baseState()
			tt.setup(d.Spec.MergeStrategy, c)

			changes := diffMergeStrategy("org/repo", d, c)
			if len(changes) != 1 {
				t.Fatalf("expected 1 parent change, got %d: %v", len(changes), changes)
			}
			parent := changes[0]
			if parent.Field != "merge_strategy" {
				t.Fatalf("expected parent field merge_strategy, got %q", parent.Field)
			}
			if len(parent.Details) != 1 {
				t.Fatalf("expected 1 child, got %d", len(parent.Details))
			}
			ch := parent.Details[0]
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
		if len(changes[0].Details) != 2 {
			t.Fatalf("expected 2 children (pattern + required_reviews), got %d", len(changes[0].Details))
		}
		if changes[0].Details[0].Field != "pattern" {
			t.Errorf("expected first child field pattern, got %q", changes[0].Details[0].Field)
		}
		if changes[0].Details[0].NewValue != "main" {
			t.Errorf("expected pattern value main, got %v", changes[0].Details[0].NewValue)
		}
		if changes[0].Details[1].Field != "required_reviews" {
			t.Errorf("expected second child field required_reviews, got %q", changes[0].Details[1].Field)
		}
		if changes[0].Details[1].NewValue != 2 {
			t.Errorf("expected child new value 2, got %v", changes[0].Details[1].NewValue)
		}
	})

	// helper to get the first child field from a parent change
	childField := func(t *testing.T, changes []Change, field string) Change {
		t.Helper()
		if len(changes) != 1 {
			t.Fatalf("expected 1 parent change, got %d: %v", len(changes), changes)
		}
		parent := changes[0]
		if parent.Field != "branch_protection" {
			t.Fatalf("expected parent field branch_protection, got %q", parent.Field)
		}
		for _, child := range parent.Details {
			if child.Field == field {
				return child
			}
		}
		t.Fatalf("child field %q not found in %d children", field, len(parent.Details))
		return Change{}
	}

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
		child := childField(t, changes, "required_reviews")
		if child.OldValue != 1 {
			t.Errorf("expected old 1, got %v", child.OldValue)
		}
		if child.NewValue != 3 {
			t.Errorf("expected new 3, got %v", child.NewValue)
		}
	})

	t.Run("additive leaves undeclared branch protection untouched", func(t *testing.T) {
		d := baseDesired()
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main"}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 0 {
			t.Fatalf("expected no changes, got %d: %v", len(changes), changes)
		}
	})

	t.Run("authoritative deletes undeclared branch protection", func(t *testing.T) {
		d := baseDesired()
		mode := manifest.CollectionReconcileAuthoritative
		d.Reconcile = &manifest.RepositoryReconcile{BranchProtection: &mode}
		c := baseState()
		c.BranchProtection["main"] = &CurrentBranchProtection{Pattern: "main"}

		changes := diffBranchProtection("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeDelete {
			t.Fatalf("change type = %q, want %q", changes[0].Type, ChangeDelete)
		}
		if changes[0].Resource != "BranchProtection[main]" {
			t.Fatalf("resource = %q, want BranchProtection[main]", changes[0].Resource)
		}
		if changes[0].NewValue != "not declared; reconcile.branch_protection=authoritative" {
			t.Fatalf("delete reason = %v", changes[0].NewValue)
		}
		if len(changes[0].Details) == 0 {
			t.Fatal("expected display-only delete children")
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
		child := childField(t, changes, "dismiss_stale_reviews")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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
		child := childField(t, changes, "enforce_admins")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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
		child := childField(t, changes, "allow_force_pushes")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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
		child := childField(t, changes, "allow_deletions")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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
		child := childField(t, changes, "require_status_checks.strict")
		if child.Type != ChangeCreate {
			t.Errorf("expected create, got %q", child.Type)
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
		child := childField(t, changes, "require_status_checks.strict")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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
		child := childField(t, changes, "require_status_checks.contexts")
		if child.Type != ChangeUpdate {
			t.Errorf("expected update, got %q", child.Type)
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

func TestDiff_Labels(t *testing.T) {
	t.Run("new label", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "kind/bug", Color: "d73a4a", Description: "A bug"},
		}
		c := baseState()

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncAdditive)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %q", changes[0].Type)
		}
		if changes[0].Field != "kind/bug" {
			t.Errorf("expected field kind/bug, got %q", changes[0].Field)
		}
	})

	t.Run("update label color", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "bug", Color: "FF0000", Description: "A bug"},
		}
		c := baseState()
		c.Labels["bug"] = &CurrentLabel{Name: "bug", Color: "d73a4a", Description: "A bug"}

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncAdditive)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeUpdate {
			t.Errorf("expected update, got %q", changes[0].Type)
		}
		if len(changes[0].Details) != 1 {
			t.Fatalf("expected 1 child, got %d", len(changes[0].Details))
		}
		if changes[0].Details[0].Field != "color" {
			t.Errorf("expected child field color, got %q", changes[0].Details[0].Field)
		}
	})

	t.Run("update label description", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "bug", Color: "d73a4a", Description: "Updated desc"},
		}
		c := baseState()
		c.Labels["bug"] = &CurrentLabel{Name: "bug", Color: "d73a4a", Description: "Old desc"}

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncAdditive)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if len(changes[0].Details) != 1 {
			t.Fatalf("expected 1 child, got %d", len(changes[0].Details))
		}
		if changes[0].Details[0].Field != "description" {
			t.Errorf("expected child field description, got %q", changes[0].Details[0].Field)
		}
	})

	t.Run("label same values no change", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "bug", Color: "d73a4a", Description: "A bug"},
		}
		c := baseState()
		c.Labels["bug"] = &CurrentLabel{Name: "bug", Color: "d73a4a", Description: "A bug"}

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncAdditive)
		if len(changes) != 0 {
			t.Errorf("expected no changes, got %d", len(changes))
		}
	})
}

func TestDiff_Labels_Mirror(t *testing.T) {
	t.Run("deletes unmanaged labels", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "keep", Color: "00ff00"},
		}
		c := baseState()
		c.Labels["keep"] = &CurrentLabel{Name: "keep", Color: "00ff00"}
		c.Labels["remove-me"] = &CurrentLabel{Name: "remove-me", Color: "ff0000", Description: "Old label"}

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncMirror)

		var deletes []Change
		for _, ch := range changes {
			if ch.Type == ChangeDelete {
				deletes = append(deletes, ch)
			}
		}
		if len(deletes) != 1 {
			t.Fatalf("expected 1 delete, got %d: %v", len(deletes), changes)
		}
		if deletes[0].Field != "remove-me" {
			t.Errorf("expected field remove-me, got %q", deletes[0].Field)
		}
	})

	t.Run("no deletes in additive mode", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Labels = []manifest.Label{
			{Name: "keep", Color: "00ff00"},
		}
		c := baseState()
		c.Labels["keep"] = &CurrentLabel{Name: "keep", Color: "00ff00"}
		c.Labels["extra"] = &CurrentLabel{Name: "extra", Color: "aaaaaa"}

		changes := diffLabels("org/repo", d, c, manifest.LabelSyncAdditive)
		for _, ch := range changes {
			if ch.Type == ChangeDelete {
				t.Errorf("unexpected delete in additive mode: %v", ch)
			}
		}
	})
}

func TestLabelSummary(t *testing.T) {
	tests := []struct {
		name        string
		color       string
		description string
		want        string
	}{
		{"color only", "d73a4a", "", "#d73a4a"},
		{"color and description", "425df5", "A feature", `#425df5 "A feature"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelSummary(tt.color, tt.description)
			if got != tt.want {
				t.Errorf("labelSummary(%q, %q) = %q, want %q", tt.color, tt.description, got, tt.want)
			}
		})
	}
}

func TestMilestoneSummary(t *testing.T) {
	tests := []struct {
		name        string
		state       string
		dueOn       string
		description string
		want        string
	}{
		{"state only", "open", "", "", "open"},
		{"with due date", "open", "2026-06-01", "", "open due:2026-06-01"},
		{"with description", "open", "", "First release", `open "First release"`},
		{"all fields", "closed", "2026-12-31", "Final release", `closed due:2026-12-31 "Final release"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := milestoneSummary(tt.state, tt.dueOn, tt.description)
			if got != tt.want {
				t.Errorf("milestoneSummary(%q, %q, %q) = %q, want %q", tt.state, tt.dueOn, tt.description, got, tt.want)
			}
		})
	}
}

func TestDiff_Milestones(t *testing.T) {
	t.Run("new milestone", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Milestones = []manifest.Milestone{
			{Title: "v1.0", Description: "First release", State: manifest.Ptr("open")},
		}
		c := baseState()

		changes := diffMilestones("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeCreate {
			t.Errorf("expected create, got %s", changes[0].Type)
		}
		if changes[0].Field != "v1.0" {
			t.Errorf("expected field v1.0, got %s", changes[0].Field)
		}
	})

	t.Run("update milestone state", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Milestones = []manifest.Milestone{
			{Title: "v1.0", State: manifest.Ptr("closed")},
		}
		c := baseState()
		c.Milestones["v1.0"] = &CurrentMilestone{Number: 1, Title: "v1.0", State: "open"}

		changes := diffMilestones("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if changes[0].Type != ChangeUpdate {
			t.Errorf("expected update, got %s", changes[0].Type)
		}
		if len(changes[0].Details) != 1 || changes[0].Details[0].Field != "state" {
			t.Errorf("expected state child change, got %v", changes[0].Details)
		}
	})

	t.Run("update milestone due_on", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Milestones = []manifest.Milestone{
			{Title: "v1.0", State: manifest.Ptr("open"), DueOn: manifest.Ptr("2026-07-01")},
		}
		c := baseState()
		c.Milestones["v1.0"] = &CurrentMilestone{Number: 1, Title: "v1.0", State: "open", DueOn: "2026-06-01"}

		changes := diffMilestones("org/repo", d, c)
		if len(changes) != 1 {
			t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
		}
		if len(changes[0].Details) != 1 || changes[0].Details[0].Field != "due_on" {
			t.Errorf("expected due_on child change, got %v", changes[0].Details)
		}
	})

	t.Run("milestone same values no change", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Milestones = []manifest.Milestone{
			{Title: "v1.0", Description: "Desc", State: manifest.Ptr("open"), DueOn: manifest.Ptr("2026-06-01")},
		}
		c := baseState()
		c.Milestones["v1.0"] = &CurrentMilestone{Number: 1, Title: "v1.0", Description: "Desc", State: "open", DueOn: "2026-06-01"}

		changes := diffMilestones("org/repo", d, c)
		if len(changes) != 0 {
			t.Errorf("expected no changes, got %d", len(changes))
		}
	})

	t.Run("nil state defaults to open", func(t *testing.T) {
		d := baseDesired()
		d.Spec.Milestones = []manifest.Milestone{
			{Title: "v1.0"},
		}
		c := baseState()
		c.Milestones["v1.0"] = &CurrentMilestone{Number: 1, Title: "v1.0", State: "open"}

		changes := diffMilestones("org/repo", d, c)
		if len(changes) != 0 {
			t.Errorf("expected no changes (nil state = open), got %d", len(changes))
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

		changes := Diff(context.Background(), d, c)

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

// collectChildFields returns a set of field names from a Change's Children.
func collectChildFields(changes []Change) map[string]bool {
	fields := make(map[string]bool)
	for _, c := range changes {
		for _, child := range c.Details {
			fields[child.Field] = true
		}
	}
	return fields
}

func TestDiff_Rulesets_Create(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Rules: manifest.RulesetRules{
				NonFastForward: manifest.Ptr(true),
			},
		},
	}
	current := baseState()

	changes := Diff(context.Background(), desired, current)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != ChangeCreate {
		t.Errorf("expected ChangeCreate, got %v", changes[0].Type)
	}
	if changes[0].Resource != "Ruleset[protect-main]" {
		t.Errorf("resource: got %q, want %q", changes[0].Resource, "Ruleset[protect-main]")
	}
	fields := collectChildFields(changes)
	if !fields["name"] {
		t.Error("expected child field 'name'")
	}
	if !fields["enforcement"] {
		t.Error("expected child field 'enforcement'")
	}
	if !fields["rules.non_fast_forward"] {
		t.Error("expected child field 'rules.non_fast_forward'")
	}
}

func TestDiff_Rulesets_Noop(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			Rules: manifest.RulesetRules{
				NonFastForward: manifest.Ptr(true),
				Deletion:       manifest.Ptr(false),
			},
		},
	}
	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		ID:          1,
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		Rules: CurrentRulesetRules{
			NonFastForward: true,
			Deletion:       false,
		},
	}

	changes := Diff(context.Background(), desired, current)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestDiff_Rulesets_ReconcileAuthoritativeDeletesUndeclared(t *testing.T) {
	desired := baseDesired()
	mode := manifest.CollectionReconcileAuthoritative
	desired.Reconcile = &manifest.RepositoryReconcile{Rulesets: &mode}

	current := baseState()
	current.Rulesets["old-ruleset"] = &CurrentRuleset{
		ID:          42,
		Name:        "old-ruleset",
		Target:      "branch",
		Enforcement: "active",
	}

	changes := Diff(context.Background(), desired, current)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != ChangeDelete {
		t.Fatalf("change type = %q, want %q", changes[0].Type, ChangeDelete)
	}
	if changes[0].Resource != "Ruleset[old-ruleset]" {
		t.Fatalf("resource = %q, want Ruleset[old-ruleset]", changes[0].Resource)
	}
	if changes[0].OldValue != 42 {
		t.Fatalf("old value = %v, want ruleset ID 42", changes[0].OldValue)
	}
	if changes[0].NewValue != "not declared; reconcile.rulesets=authoritative" {
		t.Fatalf("delete reason = %v", changes[0].NewValue)
	}
	if len(changes[0].Details) == 0 {
		t.Fatal("expected display-only delete children")
	}
}

func TestDiff_Rulesets_AdditiveLeavesUndeclaredUntouched(t *testing.T) {
	desired := baseDesired()
	current := baseState()
	current.Rulesets["old-ruleset"] = &CurrentRuleset{
		ID:          42,
		Name:        "old-ruleset",
		Target:      "branch",
		Enforcement: "active",
	}

	changes := Diff(context.Background(), desired, current)
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d: %v", len(changes), changes)
	}
}

func TestDiff_Rulesets_UpdateEnforcement(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("evaluate"),
			Rules:       manifest.RulesetRules{},
		},
	}
	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		ID:          1,
		Name:        "protect-main",
		Enforcement: "active",
	}

	changes := Diff(context.Background(), desired, current)
	fields := collectChildFields(changes)
	if !fields["enforcement"] {
		t.Error("expected enforcement change, not found")
	}
	// Verify values
	for _, c := range changes {
		for _, child := range c.Details {
			if child.Field == "enforcement" {
				if child.OldValue != "active" || child.NewValue != "evaluate" {
					t.Errorf("enforcement: old=%v new=%v", child.OldValue, child.NewValue)
				}
			}
		}
	}
}

func TestDiff_Rulesets_UpdateToggleRules(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name: "protect-main",
			Rules: manifest.RulesetRules{
				NonFastForward:        manifest.Ptr(true),
				Deletion:              manifest.Ptr(true),
				RequiredLinearHistory: manifest.Ptr(true),
			},
		},
	}
	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		ID:   1,
		Name: "protect-main",
		Rules: CurrentRulesetRules{
			NonFastForward: false,
			Deletion:       false,
		},
	}

	changes := Diff(context.Background(), desired, current)
	fields := collectChildFields(changes)
	if !fields["rules.non_fast_forward"] {
		t.Error("expected rules.non_fast_forward change")
	}
	if !fields["rules.deletion"] {
		t.Error("expected rules.deletion change")
	}
	if !fields["rules.required_linear_history"] {
		t.Error("expected rules.required_linear_history change")
	}
}

func TestDiff_Rulesets_UpdatePullRequest(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name: "protect-main",
			Rules: manifest.RulesetRules{
				PullRequest: &manifest.RulesetPullRequest{
					RequiredApprovingReviewCount: manifest.Ptr(2),
					DismissStaleReviewsOnPush:    manifest.Ptr(true),
				},
			},
		},
	}
	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		ID:   1,
		Name: "protect-main",
		Rules: CurrentRulesetRules{
			PullRequest: &CurrentRulesetPullRequest{
				RequiredApprovingReviewCount: 1,
				DismissStaleReviewsOnPush:    false,
			},
		},
	}

	changes := Diff(context.Background(), desired, current)
	fields := collectChildFields(changes)
	if !fields["rules.pull_request.required_approving_review_count"] {
		t.Error("expected review count change")
	}
	if !fields["rules.pull_request.dismiss_stale_reviews_on_push"] {
		t.Error("expected dismiss stale reviews change")
	}
}

// ─── Rulesets diff WITH resolver ───

func TestDiff_Rulesets_BypassActorsEqual(t *testing.T) {
	mock := &gh.MockRunner{}
	resolver := manifest.NewResolver(mock, "org")

	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			BypassActors: []manifest.RulesetBypassActor{
				{Role: "admin", BypassMode: "always"},
			},
			Rules: manifest.RulesetRules{},
		},
	}

	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		BypassActors: []CurrentRulesetBypassActor{
			{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"},
		},
	}

	changes := Diff(context.Background(), desired, current, DiffOptions{Resolver: resolver})

	fields := collectChildFields(changes)
	if fields["bypass_actors"] {
		t.Errorf("expected no bypass_actors change (admin=5), but got one")
	}
}

func TestDiff_Rulesets_BypassActorsChanged(t *testing.T) {
	mock := &gh.MockRunner{}
	resolver := manifest.NewResolver(mock, "org")

	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			BypassActors: []manifest.RulesetBypassActor{
				{Role: "admin", BypassMode: "always"},
			},
			Rules: manifest.RulesetRules{},
		},
	}

	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		BypassActors: []CurrentRulesetBypassActor{
			// write=4, not admin=5 → should detect a change
			{ActorID: 4, ActorType: "RepositoryRole", BypassMode: "always"},
		},
	}

	changes := Diff(context.Background(), desired, current, DiffOptions{Resolver: resolver})

	fields := collectChildFields(changes)
	if !fields["bypass_actors"] {
		t.Error("expected bypass_actors change (admin!=write), but got none")
	}
}

func TestDiff_Rulesets_StatusChecksEqual(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	resolver := manifest.NewResolver(mock, "org")

	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			Rules: manifest.RulesetRules{
				RequiredStatusChecks: &manifest.RulesetStatusChecks{
					Contexts: []manifest.RulesetStatusCheck{
						{Context: "ci/test", App: "github-actions"},
					},
				},
			},
		},
	}

	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		Rules: CurrentRulesetRules{
			RequiredStatusChecks: &CurrentRulesetStatusChecks{
				Contexts: []CurrentRulesetStatusCheck{
					{Context: "ci/test", IntegrationID: 15368},
				},
			},
		},
	}

	changes := Diff(context.Background(), desired, current, DiffOptions{Resolver: resolver})

	fields := collectChildFields(changes)
	if fields["rules.required_status_checks.contexts"] {
		t.Error("expected no status checks change, but got one")
	}
}

func TestDiff_Rulesets_StatusChecksNoApp(t *testing.T) {
	mock := &gh.MockRunner{}
	resolver := manifest.NewResolver(mock, "org")

	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			Rules: manifest.RulesetRules{
				RequiredStatusChecks: &manifest.RulesetStatusChecks{
					Contexts: []manifest.RulesetStatusCheck{
						{Context: "ci/test"},
					},
				},
			},
		},
	}

	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		Rules: CurrentRulesetRules{
			RequiredStatusChecks: &CurrentRulesetStatusChecks{
				Contexts: []CurrentRulesetStatusCheck{
					{Context: "ci/test", IntegrationID: 0},
				},
			},
		},
	}

	changes := Diff(context.Background(), desired, current, DiffOptions{Resolver: resolver})

	fields := collectChildFields(changes)
	if fields["rules.required_status_checks.contexts"] {
		t.Error("expected no status checks change (both no app), but got one")
	}
}

func TestDiff_Rulesets_StatusChecksChanged(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	resolver := manifest.NewResolver(mock, "org")

	desired := baseDesired()
	desired.Spec.Rulesets = []manifest.Ruleset{
		{
			Name:        "protect-main",
			Enforcement: manifest.Ptr("active"),
			Target:      manifest.Ptr("branch"),
			Rules: manifest.RulesetRules{
				RequiredStatusChecks: &manifest.RulesetStatusChecks{
					Contexts: []manifest.RulesetStatusCheck{
						{Context: "ci/test", App: "github-actions"},
					},
				},
			},
		},
	}

	current := baseState()
	current.Rulesets["protect-main"] = &CurrentRuleset{
		Name:        "protect-main",
		Enforcement: "active",
		Target:      "branch",
		Rules: CurrentRulesetRules{
			RequiredStatusChecks: &CurrentRulesetStatusChecks{
				Contexts: []CurrentRulesetStatusCheck{
					// Different integration ID → change detected
					{Context: "ci/test", IntegrationID: 99999},
				},
			},
		},
	}

	changes := Diff(context.Background(), desired, current, DiffOptions{Resolver: resolver})

	fields := collectChildFields(changes)
	if !fields["rules.required_status_checks.contexts"] {
		t.Error("expected status checks change (different integration ID), but got none")
	}
}

// --- Actions diff tests ---

func TestDiffActions_NoSpec(t *testing.T) {
	desired := baseDesired()
	current := baseState()
	changes := Diff(context.Background(), desired, current)
	for _, c := range changes {
		if c.Resource == manifest.ResourceActions {
			t.Fatal("expected no actions changes when spec.actions is nil")
		}
	}
}

func TestDiffActions_NoChange(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Actions = &manifest.Actions{
		Enabled:             manifest.Ptr(true),
		AllowedActions:      manifest.Ptr("all"),
		SHAPinningRequired:  manifest.Ptr(true),
		WorkflowPermissions: manifest.Ptr("read"),
	}
	current := baseState()
	current.Actions = CurrentActions{
		Enabled:             true,
		AllowedActions:      "all",
		SHAPinningRequired:  true,
		WorkflowPermissions: "read",
	}
	changes := Diff(context.Background(), desired, current)
	for _, c := range changes {
		if c.Resource == manifest.ResourceActions {
			t.Fatal("expected no actions changes when state matches desired")
		}
	}
}

func TestDiffActions_DetectsChanges(t *testing.T) {
	desired := baseDesired()
	desired.Spec.Actions = &manifest.Actions{
		Enabled:                manifest.Ptr(true),
		AllowedActions:         manifest.Ptr("selected"),
		SHAPinningRequired:     manifest.Ptr(true),
		WorkflowPermissions:    manifest.Ptr("read"),
		CanApprovePullRequests: manifest.Ptr(false),
		SelectedActions: &manifest.SelectedActions{
			GithubOwnedAllowed: manifest.Ptr(true),
			VerifiedAllowed:    manifest.Ptr(true),
			PatternsAllowed:    []string{"actions/*"},
		},
		ForkPRApproval: manifest.Ptr("all_external_contributors"),
	}
	current := baseState()
	current.Actions = CurrentActions{
		Enabled:                true,
		AllowedActions:         "all",
		SHAPinningRequired:     false,
		WorkflowPermissions:    "write",
		CanApprovePullRequests: true,
		ForkPRApproval:         "first_time_contributors",
	}

	changes := Diff(context.Background(), desired, current)

	var actionsChange *Change
	for i := range changes {
		if changes[i].Resource == manifest.ResourceActions {
			actionsChange = &changes[i]
			break
		}
	}
	if actionsChange == nil {
		t.Fatal("expected actions change, got none")
		return
	}

	fields := make(map[string]bool)
	for _, c := range actionsChange.Details {
		fields[c.Field] = true
	}

	want := []string{
		"allowed_actions",
		"sha_pinning_required",
		"workflow_permissions",
		"can_approve_pull_requests",
		"selected_actions.github_owned_allowed",
		"selected_actions.verified_allowed",
		"selected_actions.patterns_allowed",
		"fork_pr_approval",
	}
	for _, f := range want {
		if !fields[f] {
			t.Errorf("missing expected field change: %s", f)
		}
	}
}

func TestDiffActions_ChildOrder(t *testing.T) {
	// Verify enabled/allowed_actions come before selected_actions
	desired := baseDesired()
	desired.Spec.Actions = &manifest.Actions{
		Enabled:        manifest.Ptr(true),
		AllowedActions: manifest.Ptr("selected"),
		SelectedActions: &manifest.SelectedActions{
			GithubOwnedAllowed: manifest.Ptr(true),
		},
	}
	current := baseState()
	current.Actions = CurrentActions{
		Enabled:        false,
		AllowedActions: "all",
	}

	changes := Diff(context.Background(), desired, current)

	var actionsChange *Change
	for i := range changes {
		if changes[i].Resource == manifest.ResourceActions {
			actionsChange = &changes[i]
			break
		}
	}
	if actionsChange == nil {
		t.Fatal("expected actions change")
		return
	}

	// enabled and allowed_actions should appear before selected_actions.*
	sawSelected := false
	for _, c := range actionsChange.Details {
		if c.Field == "enabled" || c.Field == "allowed_actions" {
			if sawSelected {
				t.Errorf("field %q appeared after selected_actions.*", c.Field)
			}
		}
		if strings.HasPrefix(c.Field, "selected_actions.") {
			sawSelected = true
		}
	}
}
