package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepository(t *testing.T) {
	tests := []struct {
		name    string
		repo    *Repository
		wantErr string // empty means no error expected
	}{
		{
			name: "valid repository passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Visibility: Ptr("public"),
					BranchProtection: []BranchProtection{
						{Pattern: "main"},
					},
				},
			},
		},
		{
			name: "missing name fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "", Owner: "my-org"},
			},
			wantErr: "metadata.name is required",
		},
		{
			name: "missing owner fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: ""},
			},
			wantErr: "metadata.owner is required",
		},
		{
			name: "invalid visibility fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Visibility: Ptr("secret"),
				},
			},
			wantErr: "invalid spec.visibility",
		},
		{
			name: "valid visibility public passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec:     RepositorySpec{Visibility: Ptr("public")},
			},
		},
		{
			name: "valid visibility private passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec:     RepositorySpec{Visibility: Ptr("private")},
			},
		},
		{
			name: "valid visibility internal passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec:     RepositorySpec{Visibility: Ptr("internal")},
			},
		},
		{
			name: "nil visibility passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
			},
		},
		{
			name: "empty branch protection pattern fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					BranchProtection: []BranchProtection{{Pattern: ""}},
				},
			},
			wantErr: "pattern is required",
		},
		{
			name: "valid branch protection passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					BranchProtection: []BranchProtection{
						{Pattern: "main"},
						{Pattern: "release/*"},
					},
				},
			},
		},
		// Commit message settings validation
		{
			name: "valid squash_merge_commit_title",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{SquashMergeCommitTitle: Ptr("PR_TITLE")},
				},
			},
		},
		{
			name: "invalid squash_merge_commit_title",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{SquashMergeCommitTitle: Ptr("INVALID")},
				},
			},
			wantErr: "invalid spec.merge_strategy.squash_merge_commit_title",
		},
		{
			name: "valid squash_merge_commit_message",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{SquashMergeCommitMessage: Ptr("PR_BODY")},
				},
			},
		},
		{
			name: "invalid squash_merge_commit_message",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{SquashMergeCommitMessage: Ptr("NOPE")},
				},
			},
			wantErr: "invalid spec.merge_strategy.squash_merge_commit_message",
		},
		{
			name: "valid merge_commit_title",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{MergeCommitTitle: Ptr("MERGE_MESSAGE")},
				},
			},
		},
		{
			name: "invalid merge_commit_title",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{MergeCommitTitle: Ptr("BAD")},
				},
			},
			wantErr: "invalid spec.merge_strategy.merge_commit_title",
		},
		{
			name: "valid merge_commit_message",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{MergeCommitMessage: Ptr("PR_BODY")},
				},
			},
		},
		{
			name: "invalid merge_commit_message",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					MergeStrategy: &MergeStrategy{MergeCommitMessage: Ptr("WRONG")},
				},
			},
			wantErr: "invalid spec.merge_strategy.merge_commit_message",
		},
		// Secrets/variables validation
		{
			name: "empty secret name fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Secrets: []Secret{{Name: "", Value: "v"}},
				},
			},
			wantErr: "name is required",
		},
		{
			name: "empty variable name fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Variables: []Variable{{Name: "", Value: "v"}},
				},
			},
			wantErr: "name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.repo.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateFile(t *testing.T) {
	tests := []struct {
		name    string
		f       *File
		wantErr string
	}{
		{
			name: "valid file",
			f: &File{
				Metadata: FileMetadata{Name: "repo", Owner: "org"},
				Spec: FileSpec{
					Files: []FileEntry{{Path: "LICENSE", Content: "MIT"}},
				},
			},
		},
		{
			name: "missing name",
			f: &File{
				Metadata: FileMetadata{Owner: "org"},
				Spec: FileSpec{
					Files: []FileEntry{{Path: "LICENSE"}},
				},
			},
			wantErr: "metadata.name is required",
		},
		{
			name: "missing owner",
			f: &File{
				Metadata: FileMetadata{Name: "repo"},
				Spec: FileSpec{
					Files: []FileEntry{{Path: "LICENSE"}},
				},
			},
			wantErr: "metadata.owner is required",
		},
		{
			name: "missing files",
			f: &File{
				Metadata: FileMetadata{Name: "repo", Owner: "org"},
				Spec:     FileSpec{},
			},
			wantErr: "spec.files is required",
		},
		{
			name: "empty file path fails",
			f: &File{
				Metadata: FileMetadata{Name: "repo", Owner: "org"},
				Spec: FileSpec{
					Files: []FileEntry{{Path: ""}},
				},
			},
			wantErr: "path is required",
		},
		{
			name: "content and source both set",
			f: &File{
				Metadata: FileMetadata{Name: "repo", Owner: "org"},
				Spec: FileSpec{
					Files: []FileEntry{{Path: "f.txt", Content: "x", Source: "y"}},
				},
			},
			wantErr: "cannot have both content and source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.f.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateFileSet(t *testing.T) {
	tests := []struct {
		name    string
		fs      *FileSet
		wantErr string
	}{
		{
			name: "valid fileset",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: "LICENSE", Content: "MIT"}},
				},
			},
		},
		{
			name: "missing owner",
			fs: &FileSet{
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: "LICENSE"}},
				},
			},
			wantErr: "metadata.owner is required",
		},
		{
			name: "missing targets",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Files: []FileEntry{{Path: "LICENSE"}},
				},
			},
			wantErr: "spec.repositories is required",
		},
		{
			name: "missing files",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
				},
			},
			wantErr: "spec.files is required",
		},
		{
			name: "default fileset without on_drift",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: "LICENSE"}},
				},
			},
		},
		{
			name: "empty file path fails",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: ""}},
				},
			},
			wantErr: "path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fs.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateRulesets(t *testing.T) {
	base := func() *Repository {
		return &Repository{
			Metadata: RepositoryMetadata{Name: "test", Owner: "org"},
		}
	}

	tests := []struct {
		name    string
		setup   func(r *Repository)
		wantErr string
	}{
		{
			name: "valid ruleset",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{
						Name:        "protect-main",
						Enforcement: Ptr("active"),
						Target:      Ptr("branch"),
						Rules:       RulesetRules{NonFastForward: Ptr(true)},
					},
				}
			},
		},
		{
			name: "empty name",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{{Name: ""}}
			},
			wantErr: "name is required",
		},
		{
			name: "duplicate name",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "dup", Rules: RulesetRules{}},
					{Name: "dup", Rules: RulesetRules{}},
				}
			},
			wantErr: "duplicate name",
		},
		{
			name: "invalid enforcement",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", Enforcement: Ptr("invalid")},
				}
			},
			wantErr: "must be one of",
		},
		{
			name: "invalid target",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", Target: Ptr("push")},
				}
			},
			wantErr: "must be one of",
		},
		{
			name: "no actor type specified",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{BypassMode: "always"},
					}},
				}
			},
			wantErr: "must specify one of",
		},
		{
			name: "multiple actor types specified",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", Team: "maintainers", BypassMode: "always"},
					}},
				}
			},
			wantErr: "exactly one",
		},
		{
			name: "invalid role",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "invalid", BypassMode: "always"},
					}},
				}
			},
			wantErr: "must be one of",
		},
		{
			name: "valid bypass_mode always",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "always"},
					}},
				}
			},
		},
		{
			name: "valid bypass_mode pull_request",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "pull_request"},
					}},
				}
			},
		},
		{
			name: "valid bypass_mode exempt",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "exempt"},
					}},
				}
			},
		},
		{
			name: "invalid bypass_mode",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "invalid"},
					}},
				}
			},
			wantErr: "bypass_mode",
		},
		{
			name: "empty ref_name include",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{
						Name: "rs",
						Conditions: &RulesetConditions{
							RefName: &RulesetRefCondition{Include: []string{}},
						},
					},
				}
			},
			wantErr: "include must not be empty",
		},
		{
			name: "update allows fetch and merge on tag ruleset",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{
						Name:   "tags",
						Target: Ptr(RulesetTargetTag),
						Rules: RulesetRules{
							Update: &RulesetUpdate{
								Enabled:             Ptr(true),
								AllowsFetchAndMerge: Ptr(true),
							},
						},
					},
				}
			},
			wantErr: "only supported for branch rulesets",
		},
		{
			name: "update allows fetch and merge false on tag ruleset",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{
						Name:   "tags",
						Target: Ptr(RulesetTargetTag),
						Rules: RulesetRules{
							Update: &RulesetUpdate{
								Enabled:             Ptr(true),
								AllowsFetchAndMerge: Ptr(false),
							},
						},
					},
				}
			},
			wantErr: "only supported for branch rulesets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			tt.setup(r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateFileSet_InvalidReconcile(t *testing.T) {
	tests := []struct {
		name     string
		syncMode string
		wantErr  string
	}{
		{
			name:     "invalid reconcile on FileSet",
			syncMode: "full",
			wantErr:  "invalid reconcile",
		},
		{
			name:     "valid reconcile additive",
			syncMode: ReconcileAdditive,
		},
		{
			name:     "valid reconcile authoritative",
			syncMode: ReconcileAuthoritative,
		},
		{
			name:     "empty reconcile is valid (defaults to additive)",
			syncMode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: "LICENSE", Content: "MIT", Reconcile: tt.syncMode}},
				},
			}
			err := fs.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateActions_EnabledRequiredWithOtherFields(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				AllowedActions: Ptr("all"),
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error when enabled is missing but other actions fields are set")
	}
	if !strings.Contains(err.Error(), "enabled is required") || !strings.Contains(err.Error(), "add") {
		t.Errorf("error = %q, expected mention of enabled with fix suggestion", err)
	}
}

func TestValidateActions_EnabledRequiredWithSHAPinning(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				SHAPinningRequired: Ptr(true),
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error when enabled is missing but sha_pinning_required is set")
	}
	if !strings.Contains(err.Error(), "enabled is required") {
		t.Errorf("error = %q, expected mention of enabled", err)
	}
}

func TestValidateActions_SelectedActionsWithoutSelected(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				Enabled:        Ptr(true),
				AllowedActions: Ptr("all"),
				SelectedActions: &SelectedActions{
					GithubOwnedAllowed: Ptr(true),
				},
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error for selected_actions with allowed_actions != selected")
	}
	if !strings.Contains(err.Error(), "selected_actions") {
		t.Errorf("error = %q, expected mention of selected_actions", err)
	}
}

func TestValidateActions_SelectedActionsWithSelected(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				Enabled:        Ptr(true),
				AllowedActions: Ptr("selected"),
				SelectedActions: &SelectedActions{
					GithubOwnedAllowed: Ptr(true),
				},
			},
		},
	}
	if err := repo.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateActions_ForkPRApprovalUnsupportedForPrivateRepo(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Visibility: Ptr(VisibilityPrivate),
			Actions: &Actions{
				Enabled:        Ptr(true),
				ForkPRApproval: Ptr("first_time_contributors"),
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error for fork_pr_approval on private repo")
	}
	if !strings.Contains(err.Error(), "fork_pr_approval") || !strings.Contains(err.Error(), "private") {
		t.Errorf("error = %q, expected mention of fork_pr_approval and private", err)
	}
}

func TestValidateActions_InvalidAllowedActions(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				Enabled:        Ptr(true),
				AllowedActions: Ptr("none"),
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error for invalid allowed_actions value")
	}
}

func TestValidateActions_InvalidWorkflowPermissions(t *testing.T) {
	repo := &Repository{
		APIVersion: APIVersion,
		Kind:       KindRepository,
		Metadata:   RepositoryMetadata{Owner: "org", Name: "repo"},
		Spec: RepositorySpec{
			Actions: &Actions{
				Enabled:             Ptr(true),
				WorkflowPermissions: Ptr("admin"),
			},
		},
	}
	err := repo.Validate()
	if err == nil {
		t.Fatal("expected error for invalid workflow_permissions value")
	}
}

// fileSetDoc returns a kind: FileSet document owned by "org". name may be
// empty; spec holds extra spec lines such as "via: pull_request".
func fileSetDoc(name string, repos []string, spec ...string) string {
	var b strings.Builder
	b.WriteString("apiVersion: gh-infra/v1\nkind: FileSet\nmetadata:\n  owner: org\n")
	if name != "" {
		fmt.Fprintf(&b, "  name: %s\n", name)
	}
	b.WriteString("spec:\n  repositories:\n")
	for _, r := range repos {
		fmt.Fprintf(&b, "    - %s\n", r)
	}
	b.WriteString("  files:\n    - path: a.txt\n      content: a\n")
	for _, line := range spec {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String()
}

// fileDoc returns a kind: File document for org/<repo>.
func fileDoc(repo string, spec ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: gh-infra/v1\nkind: File\nmetadata:\n  owner: org\n  name: %s\n", repo)
	b.WriteString("spec:\n  files:\n    - path: a.txt\n      content: a\n")
	for _, line := range spec {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	return b.String()
}

func TestParseResultValidate_PRBranches(t *testing.T) {
	const pr = "via: pull_request"
	ab := []string{"a", "b"}
	tests := []struct {
		name    string
		docs    []string
		wantErr string // empty means no error expected
	}{
		{
			name:    "unnamed pull_request FileSets with the same repositories",
			docs:    []string{fileSetDoc("", ab, pr), fileSetDoc("", ab, pr)},
			wantErr: `on org/a from branch "gh-infra/sync-org-a+b"`,
		},
		{
			name: "distinct metadata.name",
			docs: []string{fileSetDoc("ci", ab, pr), fileSetDoc("hooks", ab, pr)},
		},
		{
			name:    "same explicit branch on a shared repository",
			docs:    []string{fileSetDoc("ci", []string{"a"}, pr, "branch: sync"), fileSetDoc("hooks", []string{"b", "a"}, pr, "branch: sync")},
			wantErr: `on org/a from branch "sync"`,
		},
		{
			name: "distinct explicit branches",
			docs: []string{fileSetDoc("", ab, pr, "branch: sync-ci"), fileSetDoc("", ab, pr, "branch: sync-hooks")},
		},
		{
			name:    "explicit branch equal to another FileSet's default branch",
			docs:    []string{fileSetDoc("", ab, pr), fileSetDoc("hooks", ab, pr, "branch: gh-infra/sync-org-a+b")},
			wantErr: `on org/a from branch "gh-infra/sync-org-a+b"`,
		},
		{
			name:    "repository names differing only in case",
			docs:    []string{fileSetDoc("ci", []string{"Repo"}, pr, "branch: sync"), fileSetDoc("hooks", []string{"repo"}, pr, "branch: sync")},
			wantErr: `from branch "sync"`,
		},
		{
			name: "push FileSets with the same repositories",
			docs: []string{fileSetDoc("", ab), fileSetDoc("", ab, "via: push")},
		},
		{
			name: "push and pull_request FileSets with the same repositories",
			docs: []string{fileSetDoc("", ab), fileSetDoc("", ab, pr)},
		},
		{
			name: "same identity without a shared repository",
			docs: []string{fileSetDoc("shared", []string{"a"}, pr), fileSetDoc("shared", []string{"b"}, pr)},
		},
		{
			name:    "kind: File manifests for one repository",
			docs:    []string{fileDoc("a", pr), fileDoc("a", pr)},
			wantErr: `on org/a from branch "gh-infra/sync-org-a"`,
		},
		{
			name: "kind: File manifests for one repository with distinct branches",
			docs: []string{fileDoc("a", pr, "branch: sync-ci"), fileDoc("a", pr, "branch: sync-hooks")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "files.yaml")
			if err := os.WriteFile(path, []byte(strings.Join(tt.docs, "---\n")), 0o644); err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseAll(path, fileOpts())
			if err != nil {
				t.Fatalf("ParseAll: %v", err)
			}

			err = parsed.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			for _, want := range []string{tt.wantErr, path + ", document 1)", path + ", document 2)"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
