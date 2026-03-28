package repository

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestPlanTargetRepoNames_All(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo2"}},
	}
	names := PlanTargetRepoNames(repos, "")
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
	if names[0] != "org/repo1" {
		t.Errorf("names[0] = %q", names[0])
	}
	if names[1] != "org/repo2" {
		t.Errorf("names[1] = %q", names[1])
	}
}

func TestPlanTargetRepoNames_Filtered(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo2"}},
	}
	names := PlanTargetRepoNames(repos, "org/repo2")
	if len(names) != 1 {
		t.Fatalf("expected 1, got %d", len(names))
	}
	if names[0] != "org/repo2" {
		t.Errorf("names[0] = %q", names[0])
	}
}

func TestPlanTargetRepoNames_NoMatch(t *testing.T) {
	repos := []*manifest.Repository{
		{Metadata: manifest.RepositoryMetadata{Owner: "org", Name: "repo1"}},
	}
	names := PlanTargetRepoNames(repos, "org/other")
	if len(names) != 0 {
		t.Errorf("expected 0, got %d", len(names))
	}
}

func TestPlanTargetRepoNames_Empty(t *testing.T) {
	names := PlanTargetRepoNames(nil, "")
	if len(names) != 0 {
		t.Errorf("expected 0, got %d", len(names))
	}
}
