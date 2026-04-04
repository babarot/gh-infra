package importer

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestFindMatches_Repository(t *testing.T) {
	parsed := &manifest.ParseResult{
		RepositoryDocs: []*manifest.RepositoryDocument{
			{
				Resource: &manifest.Repository{
					Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-a"},
				},
				FromSet: false,
			},
		},
	}

	m := FindMatches(parsed, "org/repo-a")
	if len(m.Repositories) != 1 {
		t.Fatalf("expected 1 Repository match, got %d", len(m.Repositories))
	}
	if len(m.RepositorySets) != 0 {
		t.Errorf("expected 0 RepositorySet matches, got %d", len(m.RepositorySets))
	}
	if m.IsEmpty() {
		t.Error("Matches should not be empty")
	}
}

func TestFindMatches_RepositorySet(t *testing.T) {
	parsed := &manifest.ParseResult{
		RepositoryDocs: []*manifest.RepositoryDocument{
			{
				Resource: &manifest.Repository{
					Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-b"},
				},
				FromSet: true,
			},
		},
	}

	m := FindMatches(parsed, "org/repo-b")
	if len(m.RepositorySets) != 1 {
		t.Fatalf("expected 1 RepositorySet match, got %d", len(m.RepositorySets))
	}
	if len(m.Repositories) != 0 {
		t.Errorf("expected 0 Repository matches, got %d", len(m.Repositories))
	}
}

func TestFindMatches_FileSet(t *testing.T) {
	parsed := &manifest.ParseResult{
		FileDocs: []*manifest.FileDocument{
			{
				Resource: &manifest.FileSet{
					Metadata: manifest.FileSetMetadata{Owner: "org"},
					Spec: manifest.FileSetSpec{
						Repositories: []manifest.FileSetRepository{
							{Name: "repo-a"},
							{Name: "repo-b"},
						},
					},
				},
			},
		},
	}

	m := FindMatches(parsed, "org/repo-a")
	if len(m.FileSets) != 1 {
		t.Fatalf("expected 1 FileSet match, got %d", len(m.FileSets))
	}
	if !m.HasFiles() {
		t.Error("HasFiles should be true")
	}
}

func TestFindMatches_NotFound(t *testing.T) {
	parsed := &manifest.ParseResult{
		RepositoryDocs: []*manifest.RepositoryDocument{
			{
				Resource: &manifest.Repository{
					Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-a"},
				},
			},
		},
	}

	m := FindMatches(parsed, "org/nonexistent")
	if !m.IsEmpty() {
		t.Error("Matches should be empty for nonexistent repo")
	}
}

func TestFindMatches_Multiple(t *testing.T) {
	parsed := &manifest.ParseResult{
		RepositoryDocs: []*manifest.RepositoryDocument{
			{
				Resource: &manifest.Repository{
					Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo-a"},
				},
				FromSet: false,
			},
		},
		FileDocs: []*manifest.FileDocument{
			{
				Resource: &manifest.FileSet{
					Metadata: manifest.FileSetMetadata{Owner: "org"},
					Spec: manifest.FileSetSpec{
						Repositories: []manifest.FileSetRepository{
							{Name: "repo-a"},
						},
					},
				},
			},
		},
	}

	m := FindMatches(parsed, "org/repo-a")
	if len(m.Repositories) != 1 {
		t.Errorf("expected 1 Repository match, got %d", len(m.Repositories))
	}
	if len(m.FileSets) != 1 {
		t.Errorf("expected 1 FileSet match, got %d", len(m.FileSets))
	}
	if !m.HasRepo() {
		t.Error("HasRepo should be true")
	}
	if !m.HasFiles() {
		t.Error("HasFiles should be true")
	}
}
