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

	// Default: unknown kind is silently skipped
	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("expected no error with default options, got: %v", err)
	}
	if len(result.Repositories) != 0 || len(result.FileSets) != 0 {
		t.Fatal("expected empty result for unknown kind")
	}

	// With FailOnUnknown: error
	_, err = ParseAll(path, ParseOptions{FailOnUnknown: true})
	if err == nil {
		t.Fatal("expected error for unknown kind with FailOnUnknown, got nil")
	}
	if got := err.Error(); !contains(got, "unknown kind") {
		t.Errorf("error = %q, want it to contain 'unknown kind'", got)
	}
}

func TestParsePath_UnknownField_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: testowner
spec:
  recocile: create_only
`
	path := filepath.Join(dir, "typo.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for unknown field 'recocile', got nil")
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
	if got := err.Error(); !contains(got, "invalid spec.visibility") {
		t.Errorf("error = %q, want it to contain 'invalid spec.visibility'", got)
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
	if got := err.Error(); !contains(got, "pattern is required") {
		t.Errorf("error = %q, want it to contain 'pattern is required'", got)
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

func TestParseFileSet_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo-a
    - name: repo-b
      overrides:
        - path: .github/ci.yml
          content: "custom ci"
  files:
    - path: .github/ci.yml
      content: "name: CI"
    - path: .github/lint.yml
      content: "name: Lint"
  on_drift: overwrite
`
	path := filepath.Join(dir, "fileset.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.FileSets) != 1 {
		t.Fatalf("expected 1 fileset, got %d", len(result.FileSets))
	}

	fs := result.FileSets[0]
	if fs.Metadata.Owner != "org" {
		t.Errorf("owner = %q, want %q", fs.Metadata.Owner, "org")
	}
	if len(fs.Spec.Repositories) != 2 {
		t.Fatalf("targets count = %d, want 2", len(fs.Spec.Repositories))
	}
	if fs.Spec.Repositories[0].Name != "repo-a" {
		t.Errorf("targets[0].name = %q, want %q", fs.Spec.Repositories[0].Name, "repo-a")
	}
	if fs.Spec.Repositories[1].Name != "repo-b" {
		t.Errorf("targets[1].name = %q, want %q", fs.Spec.Repositories[1].Name, "repo-b")
	}
	if len(fs.Spec.Repositories[1].Overrides) != 1 {
		t.Fatalf("targets[1].overrides count = %d, want 1", len(fs.Spec.Repositories[1].Overrides))
	}
	if len(fs.Spec.Files) != 2 {
		t.Fatalf("files count = %d, want 2", len(fs.Spec.Files))
	}
	if fs.Spec.Files[0].Content != "name: CI" {
		t.Errorf("files[0].content = %q, want %q", fs.Spec.Files[0].Content, "name: CI")
	}
	// on_drift is deprecated; just verify parsing succeeds without error
}

func TestParseFileSet_SourceFile(t *testing.T) {
	dir := t.TempDir()

	// Create source file
	sourceContent := "source file content here"
	if err := os.WriteFile(filepath.Join(dir, "template.txt"), []byte(sourceContent), 0644); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
  files:
    - path: .github/template.txt
      source: template.txt
`
	path := filepath.Join(dir, "fileset.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}

	fs := result.FileSets[0]
	if fs.Spec.Files[0].Content != sourceContent {
		t.Errorf("content = %q, want %q", fs.Spec.Files[0].Content, sourceContent)
	}
	if fs.Spec.Files[0].Source != "" {
		t.Errorf("source should be cleared after resolution, got %q", fs.Spec.Files[0].Source)
	}
}

func TestParseFileSet_MissingOwner(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: FileSet
metadata:
  owner: ""
spec:
  repositories:
    - repo
  files:
    - path: file.txt
      content: hello
`
	path := filepath.Join(dir, "fs.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for missing owner, got nil")
	}
	if !contains(err.Error(), "owner is required") {
		t.Errorf("error = %q, want it to contain 'owner is required'", err.Error())
	}
}

func TestParseFileSet_MissingTargets(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  files:
    - path: file.txt
      content: hello
`
	path := filepath.Join(dir, "fs.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for missing targets, got nil")
	}
	if !contains(err.Error(), "spec.repositories is required") {
		t.Errorf("error = %q, want it to contain 'spec.repositories is required'", err.Error())
	}
}

func TestParseFileSet_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
`
	path := filepath.Join(dir, "fs.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for missing files, got nil")
	}
	if !contains(err.Error(), "spec.files is required") {
		t.Errorf("error = %q, want it to contain 'spec.files is required'", err.Error())
	}
}

func TestParseFileSet_DeprecatedOnDrift(t *testing.T) {
	// on_drift is deprecated; parsing should succeed (field is accepted but ignored)
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
  files:
    - path: file.txt
      content: hello
  on_drift: overwrite
`
	path := filepath.Join(dir, "fs.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err != nil {
		t.Fatalf("on_drift is deprecated but should still parse: %v", err)
	}
}

func TestParseAll_RepoAndFileSet(t *testing.T) {
	dir := t.TempDir()

	repoYAML := `
apiVersion: v1
kind: Repository
metadata:
  name: my-repo
  owner: org
spec:
  visibility: public
`
	fileSetYAML := `
apiVersion: v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - my-repo
  files:
    - path: .editorconfig
      content: "root = true"
`
	if err := os.WriteFile(filepath.Join(dir, "repo.yaml"), []byte(repoYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fileset.yaml"), []byte(fileSetYAML), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(dir)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Errorf("expected 1 repository, got %d", len(result.Repositories))
	}
	if len(result.FileSets) != 1 {
		t.Errorf("expected 1 fileset, got %d", len(result.FileSets))
	}
	if result.Repositories[0].Metadata.Name != "my-repo" {
		t.Errorf("repo name = %q, want %q", result.Repositories[0].Metadata.Name, "my-repo")
	}
	if result.FileSets[0].Metadata.Owner != "org" {
		t.Errorf("fileset owner = %q, want %q", result.FileSets[0].Metadata.Owner, "org")
	}
}

func TestParseFile_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: File
metadata:
  owner: org
  name: my-repo
spec:
  files:
    - path: .github/CODEOWNERS
      content: "* @org/team"
    - path: LICENSE
      content: "MIT"
  on_drift: overwrite
  via: push
`
	path := filepath.Join(dir, "file.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.FileSets) != 1 {
		t.Fatalf("expected 1 fileset (expanded from File), got %d", len(result.FileSets))
	}

	fs := result.FileSets[0]
	if fs.Metadata.Owner != "org" {
		t.Errorf("owner = %q, want %q", fs.Metadata.Owner, "org")
	}
	if len(fs.Spec.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(fs.Spec.Repositories))
	}
	if fs.Spec.Repositories[0].Name != "my-repo" {
		t.Errorf("repo name = %q, want %q", fs.Spec.Repositories[0].Name, "my-repo")
	}
	if len(fs.Spec.Files) != 2 {
		t.Fatalf("files count = %d, want 2", len(fs.Spec.Files))
	}
	// on_drift is deprecated; just verify via is parsed
	if fs.Spec.Via != "push" {
		t.Errorf("via = %q, want %q", fs.Spec.Via, "push")
	}
}

func TestParseFile_MissingOwner(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: File
metadata:
  name: repo
spec:
  files:
    - path: file.txt
      content: hello
`
	path := filepath.Join(dir, "file.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for missing owner, got nil")
	}
	if !contains(err.Error(), "owner is required") {
		t.Errorf("error = %q, want it to contain 'owner is required'", err.Error())
	}
}

func TestParseFile_MissingName(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: v1
kind: File
metadata:
  owner: org
spec:
  files:
    - path: file.txt
      content: hello
`
	path := filepath.Join(dir, "file.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want it to contain 'name is required'", err.Error())
	}
}

func TestParseFile_SourceFile(t *testing.T) {
	dir := t.TempDir()

	sourceContent := "source file content"
	if err := os.WriteFile(filepath.Join(dir, "tmpl.txt"), []byte(sourceContent), 0644); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
apiVersion: v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: .github/tmpl.txt
      source: tmpl.txt
`
	path := filepath.Join(dir, "file.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}

	fs := result.FileSets[0]
	if fs.Spec.Files[0].Content != sourceContent {
		t.Errorf("content = %q, want %q", fs.Spec.Files[0].Content, sourceContent)
	}
	if fs.Spec.Files[0].Source != "" {
		t.Errorf("source should be cleared after resolution, got %q", fs.Spec.Files[0].Source)
	}
}

func TestParseFileSet_DeprecatedFileLevelOnDrift(t *testing.T) {
	// on_drift at file level is deprecated; parsing should still succeed
	dir := t.TempDir()
	yamlContent := `
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
  on_drift: warn
  files:
    - path: a.txt
      content: hello
      on_drift: overwrite
    - path: b.txt
      content: world
`
	path := filepath.Join(dir, "fileset.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err != nil {
		t.Fatalf("on_drift is deprecated but should still parse: %v", err)
	}
}

func TestParseFile_DeprecatedFileLevelOnDrift(t *testing.T) {
	// on_drift at file level is deprecated; parsing should still succeed
	dir := t.TempDir()
	yamlContent := `
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: org
  name: repo
spec:
  files:
    - path: a.txt
      content: hello
      on_drift: skip
    - path: b.txt
      content: world
      on_drift: overwrite
`
	path := filepath.Join(dir, "file.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseAll(path)
	if err != nil {
		t.Fatalf("on_drift is deprecated but should still parse: %v", err)
	}
}

func TestMergeMergeStrategy_MergeCommitTitleMessage(t *testing.T) {
	base := &MergeStrategy{
		MergeCommitTitle:   Ptr("MERGE_MESSAGE"),
		MergeCommitMessage: Ptr("PR_BODY"),
	}
	override := &MergeStrategy{
		MergeCommitTitle:         Ptr("PR_TITLE"),
		SquashMergeCommitTitle:   Ptr("PR_TITLE"),
		SquashMergeCommitMessage: Ptr("BLANK"),
	}

	result := mergeMergeStrategy(base, override)

	// overridden
	if result.MergeCommitTitle == nil || *result.MergeCommitTitle != "PR_TITLE" {
		t.Errorf("merge_commit_title = %v, want PR_TITLE", result.MergeCommitTitle)
	}
	// base preserved when not overridden
	if result.MergeCommitMessage == nil || *result.MergeCommitMessage != "PR_BODY" {
		t.Errorf("merge_commit_message = %v, want PR_BODY (from base)", result.MergeCommitMessage)
	}
	// new from override
	if result.SquashMergeCommitTitle == nil || *result.SquashMergeCommitTitle != "PR_TITLE" {
		t.Errorf("squash_merge_commit_title = %v, want PR_TITLE", result.SquashMergeCommitTitle)
	}
	if result.SquashMergeCommitMessage == nil || *result.SquashMergeCommitMessage != "BLANK" {
		t.Errorf("squash_merge_commit_message = %v, want BLANK", result.SquashMergeCommitMessage)
	}
}

func TestMergeFeatures_NilBase(t *testing.T) {
	override := &Features{
		Issues: Ptr(true),
	}
	result := mergeFeatures(nil, override)
	if result != override {
		t.Error("expected override returned when base is nil")
	}
}

func TestMergeFeatures_NilOverride(t *testing.T) {
	base := &Features{
		Issues: Ptr(true),
	}
	result := mergeFeatures(base, nil)
	if result != base {
		t.Error("expected base returned when override is nil")
	}
}

func TestMergeMergeStrategy_NilBase(t *testing.T) {
	override := &MergeStrategy{
		MergeCommitTitle: Ptr("PR_TITLE"),
	}
	result := mergeMergeStrategy(nil, override)
	if result != override {
		t.Error("expected override returned when base is nil")
	}
}

func TestMergeMergeStrategy_NilOverride(t *testing.T) {
	base := &MergeStrategy{
		MergeCommitTitle: Ptr("MERGE_MESSAGE"),
	}
	result := mergeMergeStrategy(base, nil)
	if result != base {
		t.Error("expected base returned when override is nil")
	}
}

func TestParseAll_MultiDocument_TwoRepositories(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-a
  owner: my-org
spec:
  description: "Repo A"
  visibility: public
---
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-b
  owner: my-org
spec:
  description: "Repo B"
  visibility: private
`
	path := filepath.Join(dir, "repos.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.Repositories) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(result.Repositories))
	}
	if result.Repositories[0].Metadata.Name != "repo-a" {
		t.Errorf("first repo name = %q, want %q", result.Repositories[0].Metadata.Name, "repo-a")
	}
	if result.Repositories[1].Metadata.Name != "repo-b" {
		t.Errorf("second repo name = %q, want %q", result.Repositories[1].Metadata.Name, "repo-b")
	}
}

func TestParseAll_MultiDocument_MixedKinds(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-repo
  owner: my-org
spec:
  description: "A repo"
  visibility: public
---
apiVersion: gh-infra/v1
kind: File
metadata:
  name: my-repo
  owner: my-org
spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @my-org
  via: push
`
	path := filepath.Join(dir, "mixed.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(result.Repositories))
	}
	if len(result.FileSets) != 1 {
		t.Fatalf("expected 1 fileset, got %d", len(result.FileSets))
	}
	if result.Repositories[0].Metadata.Name != "my-repo" {
		t.Errorf("repo name = %q, want %q", result.Repositories[0].Metadata.Name, "my-repo")
	}
}

func TestParseAll_MultiDocument_SingleDocStillWorks(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: solo
  owner: my-org
spec:
  visibility: public
`
	path := filepath.Join(dir, "single.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.Repositories) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(result.Repositories))
	}
	if result.Repositories[0].Metadata.Name != "solo" {
		t.Errorf("repo name = %q, want %q", result.Repositories[0].Metadata.Name, "solo")
	}
}

func TestParseAll_MultiDocument_LeadingSeparator(t *testing.T) {
	dir := t.TempDir()
	// Some YAML files start with --- as the first line
	content := `---
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-a
  owner: my-org
spec:
  visibility: public
---
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: repo-b
  owner: my-org
spec:
  visibility: private
`
	path := filepath.Join(dir, "leading.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll returned error: %v", err)
	}
	if len(result.Repositories) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(result.Repositories))
	}
}

func TestSplitDocuments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single", "kind: Repository\nname: foo", 1},
		{"two docs", "kind: Repository\nname: foo\n---\nkind: File\nname: bar", 2},
		{"leading separator", "---\nkind: Repository\nname: foo", 1},
		{"trailing separator", "kind: Repository\nname: foo\n---\n", 1},
		{"empty between", "kind: A\n---\n\n---\nkind: B", 2},
		{"only separators", "\n---\n---\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := splitDocuments([]byte(tt.input))
			if len(docs) != tt.want {
				t.Errorf("splitDocuments() returned %d docs, want %d", len(docs), tt.want)
			}
		})
	}
}

func TestParseResult_RepositoryDocs(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-repo
  owner: my-org
spec:
  visibility: public
`
	path := filepath.Join(dir, "repo.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if len(result.RepositoryDocs) != 1 {
		t.Fatalf("expected 1 RepositoryDoc, got %d", len(result.RepositoryDocs))
	}

	doc := result.RepositoryDocs[0]
	if doc.Resource.Metadata.Name != "my-repo" {
		t.Errorf("Resource.Name = %q, want %q", doc.Resource.Metadata.Name, "my-repo")
	}
	if doc.SourcePath != path {
		t.Errorf("SourcePath = %q, want %q", doc.SourcePath, path)
	}
	if doc.DocIndex != 0 {
		t.Errorf("DocIndex = %d, want 0", doc.DocIndex)
	}
	if doc.FromSet {
		t.Error("FromSet should be false for standalone Repository")
	}
}

func TestParseRepositorySet_DefaultsSpec(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org
defaults:
  spec:
    visibility: private
    features:
      issues: true
repositories:
  - name: repo-a
  - name: repo-b
    spec:
      visibility: public
`
	path := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if len(result.RepositoryDocs) != 2 {
		t.Fatalf("expected 2 RepositoryDocs, got %d", len(result.RepositoryDocs))
	}

	for i, doc := range result.RepositoryDocs {
		if !doc.FromSet {
			t.Errorf("doc[%d].FromSet should be true", i)
		}
		if doc.DefaultsSpec == nil {
			t.Errorf("doc[%d].DefaultsSpec should not be nil", i)
		}
		if doc.SetEntryIndex != i {
			t.Errorf("doc[%d].SetEntryIndex = %d, want %d", i, doc.SetEntryIndex, i)
		}
	}

	// Verify defaults spec content
	defaults := result.RepositoryDocs[0].DefaultsSpec
	if defaults.Spec.Visibility == nil || *defaults.Spec.Visibility != "private" {
		t.Errorf("DefaultsSpec.Visibility = %v, want private", defaults.Spec.Visibility)
	}
}

func TestParseRepositorySet_OriginalEntrySpec(t *testing.T) {
	dir := t.TempDir()
	content := `apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org
defaults:
  spec:
    visibility: private
repositories:
  - name: repo-a
  - name: repo-b
    spec:
      visibility: public
      description: "override desc"
`
	path := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseAll(path)
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if len(result.RepositoryDocs) != 2 {
		t.Fatalf("expected 2 RepositoryDocs, got %d", len(result.RepositoryDocs))
	}

	// repo-a: no override → OriginalEntrySpec should have zero-value fields
	docA := result.RepositoryDocs[0]
	if docA.OriginalEntrySpec == nil {
		t.Fatal("repo-a OriginalEntrySpec should not be nil")
	}
	if docA.OriginalEntrySpec.Visibility != nil {
		t.Errorf("repo-a OriginalEntrySpec.Visibility should be nil, got %v", docA.OriginalEntrySpec.Visibility)
	}

	// repo-b: has overrides
	docB := result.RepositoryDocs[1]
	if docB.OriginalEntrySpec == nil {
		t.Fatal("repo-b OriginalEntrySpec should not be nil")
	}
	if docB.OriginalEntrySpec.Visibility == nil || *docB.OriginalEntrySpec.Visibility != "public" {
		t.Errorf("repo-b OriginalEntrySpec.Visibility = %v, want public", docB.OriginalEntrySpec.Visibility)
	}
	if docB.OriginalEntrySpec.Description == nil || *docB.OriginalEntrySpec.Description != "override desc" {
		t.Errorf("repo-b OriginalEntrySpec.Description = %v, want 'override desc'", docB.OriginalEntrySpec.Description)
	}

	// But the merged result for repo-b should have visibility=public (override wins)
	if result.Repositories[1].Spec.Visibility == nil || *result.Repositories[1].Spec.Visibility != "public" {
		t.Errorf("merged repo-b Visibility = %v, want public", result.Repositories[1].Spec.Visibility)
	}
}
