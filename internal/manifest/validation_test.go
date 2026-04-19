package manifest

import (
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
					BranchProtection: NewDeletable([]BranchProtection{
						{Pattern: "main"},
					}),
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
					BranchProtection: NewDeletable([]BranchProtection{{Pattern: ""}}),
				},
			},
			wantErr: "pattern is required",
		},
		{
			name: "valid branch protection passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					BranchProtection: NewDeletable([]BranchProtection{
						{Pattern: "main"},
						{Pattern: "release/*"},
					}),
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
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{
						Name:        "protect-main",
						Enforcement: Ptr("active"),
						Target:      Ptr("branch"),
						Rules:       RulesetRules{NonFastForward: Ptr(true)},
					},
				})
			},
		},
		{
			name: "empty name",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{{Name: ""}})
			},
			wantErr: "name is required",
		},
		{
			name: "duplicate name",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "dup", Rules: RulesetRules{}},
					{Name: "dup", Rules: RulesetRules{}},
				})
			},
			wantErr: "duplicate ruleset name",
		},
		{
			name: "invalid enforcement",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", Enforcement: Ptr("invalid")},
				})
			},
			wantErr: "must be one of",
		},
		{
			name: "invalid target",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", Target: Ptr("push")},
				})
			},
			wantErr: "must be one of",
		},
		{
			name: "no actor type specified",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{BypassMode: "always"},
					}},
				})
			},
			wantErr: "must specify one of",
		},
		{
			name: "multiple actor types specified",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", Team: "maintainers", BypassMode: "always"},
					}},
				})
			},
			wantErr: "exactly one",
		},
		{
			name: "invalid role",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "invalid", BypassMode: "always"},
					}},
				})
			},
			wantErr: "must be one of",
		},
		{
			name: "valid bypass_mode always",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "always"},
					}},
				})
			},
		},
		{
			name: "valid bypass_mode pull_request",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "pull_request"},
					}},
				})
			},
		},
		{
			name: "valid bypass_mode exempt",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "exempt"},
					}},
				})
			},
		},
		{
			name: "invalid bypass_mode",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{Name: "rs", BypassActors: []RulesetBypassActor{
						{Role: "admin", BypassMode: "invalid"},
					}},
				})
			},
			wantErr: "bypass_mode",
		},
		{
			name: "empty ref_name include",
			setup: func(r *Repository) {
				r.Spec.Rulesets = NewDeletable([]Ruleset{
					{
						Name: "rs",
						Conditions: &RulesetConditions{
							RefName: &RulesetRefCondition{Include: []string{}},
						},
					},
				})
			},
			wantErr: "include must not be empty",
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
			name:     "valid reconcile patch",
			syncMode: ReconcilePatch,
		},
		{
			name:     "valid reconcile mirror",
			syncMode: ReconcileMirror,
		},
		{
			name:     "empty reconcile is valid (defaults to patch)",
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
