package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePath_SingleRepository(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
  owner: my-org
spec:
  description: "A test repo"
  visibility: public
  topics:
    - go
    - cli
  features:
    issues: true
    wiki: false
  branch_protection:
    - pattern: main
      required_reviews: 2
`
	path := filepath.Join(dir, "repo.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	repo := repos[0]
	if repo.Metadata.Name != "my-repo" {
		t.Errorf("name = %q, want %q", repo.Metadata.Name, "my-repo")
	}
	if repo.Metadata.Owner != "my-org" {
		t.Errorf("owner = %q, want %q", repo.Metadata.Owner, "my-org")
	}
	if repo.Metadata.FullName() != "my-org/my-repo" {
		t.Errorf("FullName() = %q, want %q", repo.Metadata.FullName(), "my-org/my-repo")
	}
	if repo.Spec.Description == nil || *repo.Spec.Description != "A test repo" {
		t.Errorf("description = %v, want %q", repo.Spec.Description, "A test repo")
	}
	if repo.Spec.Visibility == nil || *repo.Spec.Visibility != "public" {
		t.Errorf("visibility = %v, want %q", repo.Spec.Visibility, "public")
	}
	if len(repo.Spec.Topics) != 2 {
		t.Errorf("topics count = %d, want 2", len(repo.Spec.Topics))
	}
	if repo.Spec.Features == nil {
		t.Fatal("features is nil")
	}
	if repo.Spec.Features.Issues == nil || *repo.Spec.Features.Issues != true {
		t.Errorf("features.issues = %v, want true", repo.Spec.Features.Issues)
	}
	if repo.Spec.Features.Wiki == nil || *repo.Spec.Features.Wiki != false {
		t.Errorf("features.wiki = %v, want false", repo.Spec.Features.Wiki)
	}
	if len(repo.Spec.BranchProtection) != 1 {
		t.Fatalf("branch_protection count = %d, want 1", len(repo.Spec.BranchProtection))
	}
	if repo.Spec.BranchProtection[0].Pattern != "main" {
		t.Errorf("branch_protection[0].pattern = %q, want %q", repo.Spec.BranchProtection[0].Pattern, "main")
	}
	if repo.Spec.BranchProtection[0].RequiredReviews == nil || *repo.Spec.BranchProtection[0].RequiredReviews != 2 {
		t.Errorf("branch_protection[0].required_reviews = %v, want 2", repo.Spec.BranchProtection[0].RequiredReviews)
	}
}

func TestParsePath_RepositorySet_WithDefaultsMerging(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: RepositorySet
metadata:
  owner: my-org
defaults:
  spec:
    visibility: private
    features:
      issues: true
      wiki: false
repositories:
  - name: repo-a
    spec:
      description: "Repo A"
  - name: repo-b
    spec:
      description: "Repo B"
      visibility: public
`
	path := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	// repo-a inherits defaults
	a := repos[0]
	if a.Metadata.Name != "repo-a" {
		t.Errorf("repos[0].name = %q, want %q", a.Metadata.Name, "repo-a")
	}
	if a.Metadata.Owner != "my-org" {
		t.Errorf("repos[0].owner = %q, want %q", a.Metadata.Owner, "my-org")
	}
	if a.Spec.Visibility == nil || *a.Spec.Visibility != "private" {
		t.Errorf("repos[0].visibility = %v, want %q", a.Spec.Visibility, "private")
	}
	if a.Spec.Features == nil || a.Spec.Features.Issues == nil || *a.Spec.Features.Issues != true {
		t.Errorf("repos[0].features.issues should be true from defaults")
	}

	// repo-b overrides visibility
	b := repos[1]
	if b.Spec.Visibility == nil || *b.Spec.Visibility != "public" {
		t.Errorf("repos[1].visibility = %v, want %q", b.Spec.Visibility, "public")
	}
}

func TestParsePath_Directory_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	file1 := `
apiVersion: v1
kind: Repository
metadata:
  name: repo-one
  owner: org
spec:
  visibility: public
`
	file2 := `
apiVersion: v1
kind: Repository
metadata:
  name: repo-two
  owner: org
spec:
  visibility: private
`
	// Non-YAML file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(file1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yml"), []byte(file2), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParsePath(dir)
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	names := map[string]bool{}
	for _, r := range repos {
		names[r.Metadata.Name] = true
	}
	if !names["repo-one"] || !names["repo-two"] {
		t.Errorf("expected repo-one and repo-two, got %v", names)
	}
}

func TestParsePath_UnknownKind_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: UnknownThing
metadata:
  name: test
`
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParsePath(path)
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
	if got := err.Error(); !contains(got, "unknown kind") {
		t.Errorf("error = %q, want it to contain 'unknown kind'", got)
	}
}

func TestParsePath_MissingName_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: Repository
metadata:
  owner: org
spec:
  visibility: public
`
	path := filepath.Join(dir, "noname.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParsePath(path)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if got := err.Error(); !contains(got, "metadata.name") {
		t.Errorf("error = %q, want it to contain 'metadata.name'", got)
	}
}

func TestParsePath_MissingOwner_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
spec:
  visibility: public
`
	path := filepath.Join(dir, "noowner.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParsePath(path)
	if err == nil {
		t.Fatal("expected error for missing owner, got nil")
	}
	if got := err.Error(); !contains(got, "metadata.owner") {
		t.Errorf("error = %q, want it to contain 'metadata.owner'", got)
	}
}

func TestParsePath_InvalidVisibility_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
  owner: org
spec:
  visibility: secret
`
	path := filepath.Join(dir, "badvis.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParsePath(path)
	if err == nil {
		t.Fatal("expected error for invalid visibility, got nil")
	}
	if got := err.Error(); !contains(got, "invalid visibility") {
		t.Errorf("error = %q, want it to contain 'invalid visibility'", got)
	}
}

func TestParsePath_EmptyBranchProtectionPattern_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
  owner: org
spec:
  branch_protection:
    - pattern: ""
      required_reviews: 1
`
	path := filepath.Join(dir, "emptybp.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParsePath(path)
	if err == nil {
		t.Fatal("expected error for empty branch protection pattern, got nil")
	}
	if got := err.Error(); !contains(got, "branch_protection.pattern") {
		t.Errorf("error = %q, want it to contain 'branch_protection.pattern'", got)
	}
}

func TestRepositorySet_PerRepoOverridesTakePrecedence(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: RepositorySet
metadata:
  owner: org
defaults:
  spec:
    description: "default description"
    visibility: private
    topics:
      - default-topic
    homepage: "https://default.example.com"
repositories:
  - name: override-repo
    spec:
      description: "overridden"
      visibility: public
      topics:
        - custom-topic
      homepage: "https://custom.example.com"
`
	path := filepath.Join(dir, "overrides.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	repo := repos[0]
	if repo.Spec.Description == nil || *repo.Spec.Description != "overridden" {
		t.Errorf("description = %v, want %q", repo.Spec.Description, "overridden")
	}
	if repo.Spec.Visibility == nil || *repo.Spec.Visibility != "public" {
		t.Errorf("visibility = %v, want %q", repo.Spec.Visibility, "public")
	}
	if len(repo.Spec.Topics) != 1 || repo.Spec.Topics[0] != "custom-topic" {
		t.Errorf("topics = %v, want [custom-topic]", repo.Spec.Topics)
	}
	if repo.Spec.Homepage == nil || *repo.Spec.Homepage != "https://custom.example.com" {
		t.Errorf("homepage = %v, want %q", repo.Spec.Homepage, "https://custom.example.com")
	}
}

func TestRepositorySet_FeaturesMerge(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: RepositorySet
metadata:
  owner: org
defaults:
  spec:
    visibility: public
    features:
      issues: true
      wiki: true
      projects: false
repositories:
  - name: merged-repo
    spec:
      features:
        wiki: false
        discussions: true
`
	path := filepath.Join(dir, "features.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath returned error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	f := repos[0].Spec.Features
	if f == nil {
		t.Fatal("features is nil after merge")
	}
	// From defaults
	if f.Issues == nil || *f.Issues != true {
		t.Errorf("features.issues = %v, want true (from defaults)", f.Issues)
	}
	if f.Projects == nil || *f.Projects != false {
		t.Errorf("features.projects = %v, want false (from defaults)", f.Projects)
	}
	// Overridden
	if f.Wiki == nil || *f.Wiki != false {
		t.Errorf("features.wiki = %v, want false (overridden)", f.Wiki)
	}
	// New from override
	if f.Discussions == nil || *f.Discussions != true {
		t.Errorf("features.discussions = %v, want true (from override)", f.Discussions)
	}
}

func TestResolveSecrets_ExpandsEnvVars(t *testing.T) {
	// Set test environment variables
	t.Setenv("ENV_SECRET_TOKEN", "my-secret-value")
	t.Setenv("ENV_API_KEY", "api-key-123")

	repos := []*Repository{
		{
			Metadata: RepositoryMetadata{Name: "test", Owner: "org"},
			Spec: RepositorySpec{
				Secrets: []Secret{
					{Name: "TOKEN", Value: "${ENV_SECRET_TOKEN}"},
					{Name: "API_KEY", Value: "${ENV_API_KEY}"},
					{Name: "LITERAL", Value: "plain-value"},
					{Name: "NON_ENV", Value: "${NOT_ENV_PREFIX}"},
				},
			},
		},
	}

	ResolveSecrets(repos)

	tests := []struct {
		idx  int
		want string
	}{
		{0, "my-secret-value"},
		{1, "api-key-123"},
		{2, "plain-value"},
		{3, "${NOT_ENV_PREFIX}"},
	}

	for _, tt := range tests {
		got := repos[0].Spec.Secrets[tt.idx].Value
		if got != tt.want {
			t.Errorf("secret[%d].Value = %q, want %q", tt.idx, got, tt.want)
		}
	}
}

// contains is a small helper to check substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
