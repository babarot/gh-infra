package manifest

import (
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
					Visibility: StringPtr("public"),
					BranchProtection: []BranchProtection{
						{Pattern: "main"},
					},
				},
			},
			wantErr: "",
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
					Visibility: StringPtr("secret"),
				},
			},
			wantErr: "invalid visibility",
		},
		{
			name: "valid visibility public passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Visibility: StringPtr("public"),
				},
			},
			wantErr: "",
		},
		{
			name: "valid visibility private passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Visibility: StringPtr("private"),
				},
			},
			wantErr: "",
		},
		{
			name: "valid visibility internal passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					Visibility: StringPtr("internal"),
				},
			},
			wantErr: "",
		},
		{
			name: "nil visibility passes",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec:     RepositorySpec{},
			},
			wantErr: "",
		},
		{
			name: "empty branch protection pattern fails",
			repo: &Repository{
				Metadata: RepositoryMetadata{Name: "my-repo", Owner: "my-org"},
				Spec: RepositorySpec{
					BranchProtection: []BranchProtection{
						{Pattern: ""},
					},
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
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepository(tt.repo, "test.yaml")
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
