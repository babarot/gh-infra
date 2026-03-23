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
			wantErr: "invalid visibility",
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
			wantErr: "branch_protection.pattern is required",
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
			wantErr: "invalid squash_merge_commit_title",
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
			wantErr: "invalid squash_merge_commit_message",
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
			wantErr: "invalid merge_commit_title",
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
			wantErr: "invalid merge_commit_message",
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
			wantErr: "secrets[].name is required",
		},
		{
			name: "empty variable name fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Variables: []Variable{{Name: "", Value: "v"}},
				},
			},
			wantErr: "variables[].name is required",
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
					OnDrift:      "overwrite",
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
			name: "invalid on_drift",
			fs: &FileSet{
				Metadata: FileSetMetadata{Owner: "org"},
				Spec: FileSetSpec{
					Repositories: []FileSetRepository{{Name: "repo"}},
					Files:        []FileEntry{{Path: "LICENSE"}},
					OnDrift:      "delete",
				},
			},
			wantErr: "invalid on_drift",
		},
		{
			name: "default on_drift is warn",
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
			wantErr: "files[0].path is required",
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
			wantErr: "rulesets[].name is required",
		},
		{
			name: "duplicate name",
			setup: func(r *Repository) {
				r.Spec.Rulesets = []Ruleset{
					{Name: "dup", Rules: RulesetRules{}},
					{Name: "dup", Rules: RulesetRules{}},
				}
			},
			wantErr: "duplicate ruleset name",
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

func TestValidateOneOf(t *testing.T) {
	if err := validateOneOf("field", "a", "a", "b", "c"); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	err := validateOneOf("field", "d", "a", "b", "c")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must be one of: a, b, c") {
		t.Errorf("unexpected error: %v", err)
	}
}
