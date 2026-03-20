package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// ParsePath parses a file or directory and returns all Repository resources.
func ParsePath(path string) ([]*Repository, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if !info.IsDir() {
		return parseFile(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}

	var repos []*Repository
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		parsed, err := parseFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		repos = append(repos, parsed...)
	}
	return repos, nil
}

func parseFile(path string) ([]*Repository, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Determine the kind
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	switch doc.Kind {
	case "Repository":
		return parseRepository(data, path)
	case "RepositorySet":
		return parseRepositorySet(data, path)
	default:
		return nil, fmt.Errorf("%s: unknown kind %q", path, doc.Kind)
	}
}

func parseRepository(data []byte, path string) ([]*Repository, error) {
	var repo Repository
	if err := yaml.Unmarshal(data, &repo); err != nil {
		return nil, fmt.Errorf("parse Repository in %s: %w", path, err)
	}
	if err := validateRepository(&repo, path); err != nil {
		return nil, err
	}
	return []*Repository{&repo}, nil
}

func parseRepositorySet(data []byte, path string) ([]*Repository, error) {
	var set RepositorySet
	if err := yaml.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("parse RepositorySet in %s: %w", path, err)
	}

	var repos []*Repository
	for _, entry := range set.Repositories {
		repo := &Repository{
			APIVersion: set.APIVersion,
			Kind:       "Repository",
			Metadata: RepositoryMetadata{
				Name:  entry.Name,
				Owner: set.Metadata.Owner,
			},
			Spec: mergeSpecs(set.Defaults, entry.Spec),
		}
		if err := validateRepository(repo, path); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// mergeSpecs merges defaults with per-repo overrides. Per-repo values take precedence.
func mergeSpecs(defaults *RepositorySetDefaults, override RepositorySpec) RepositorySpec {
	if defaults == nil {
		return override
	}

	result := defaults.Spec

	if override.Description != nil {
		result.Description = override.Description
	}
	if override.Homepage != nil {
		result.Homepage = override.Homepage
	}
	if override.Visibility != nil {
		result.Visibility = override.Visibility
	}
	if len(override.Topics) > 0 {
		result.Topics = override.Topics
	}
	if override.Features != nil {
		result.Features = mergeFeatures(result.Features, override.Features)
	}
	if len(override.BranchProtection) > 0 {
		result.BranchProtection = override.BranchProtection
	}
	if len(override.Secrets) > 0 {
		result.Secrets = override.Secrets
	}
	if len(override.Variables) > 0 {
		result.Variables = override.Variables
	}

	return result
}

func mergeFeatures(base, override *Features) *Features {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := *base
	if override.Issues != nil {
		result.Issues = override.Issues
	}
	if override.Projects != nil {
		result.Projects = override.Projects
	}
	if override.Wiki != nil {
		result.Wiki = override.Wiki
	}
	if override.Discussions != nil {
		result.Discussions = override.Discussions
	}
	if override.MergeCommit != nil {
		result.MergeCommit = override.MergeCommit
	}
	if override.SquashMerge != nil {
		result.SquashMerge = override.SquashMerge
	}
	if override.RebaseMerge != nil {
		result.RebaseMerge = override.RebaseMerge
	}
	if override.AutoDeleteHeadBranches != nil {
		result.AutoDeleteHeadBranches = override.AutoDeleteHeadBranches
	}
	if override.MergeCommitTitle != nil {
		result.MergeCommitTitle = override.MergeCommitTitle
	}
	if override.MergeCommitMessage != nil {
		result.MergeCommitMessage = override.MergeCommitMessage
	}
	if override.SquashMergeCommitTitle != nil {
		result.SquashMergeCommitTitle = override.SquashMergeCommitTitle
	}
	if override.SquashMergeCommitMessage != nil {
		result.SquashMergeCommitMessage = override.SquashMergeCommitMessage
	}
	return &result
}

// expandEnvVars replaces ${ENV_*} references with actual environment variables.
func expandEnvVars(s string) string {
	return os.Expand(s, func(key string) string {
		if strings.HasPrefix(key, "ENV_") {
			return os.Getenv(key)
		}
		return "${" + key + "}"
	})
}

// ResolveSecrets expands environment variable references in secret values.
func ResolveSecrets(repos []*Repository) {
	for _, repo := range repos {
		for i := range repo.Spec.Secrets {
			repo.Spec.Secrets[i].Value = expandEnvVars(repo.Spec.Secrets[i].Value)
		}
	}
}
