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
// For backward compatibility, this only returns repositories.
func ParsePath(path string) ([]*Repository, error) {
	result, err := ParseAll(path)
	if err != nil {
		return nil, err
	}
	return result.Repositories, nil
}

// ParseAll parses a file or directory and returns all resources (Repository + FileSet).
func ParseAll(path string) (*ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if !info.IsDir() {
		return parseFileAll(path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}

	result := &ParseResult{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		parsed, err := parseFileAll(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		result.Repositories = append(result.Repositories, parsed.Repositories...)
		result.FileSets = append(result.FileSets, parsed.FileSets...)
	}
	return result, nil
}

func parseFileAll(path string) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	result := &ParseResult{}

	switch doc.Kind {
	case KindRepository:
		repos, err := parseRepository(data, path)
		if err != nil {
			return nil, err
		}
		result.Repositories = repos
	case KindRepositorySet:
		repos, err := parseRepositorySet(data, path)
		if err != nil {
			return nil, err
		}
		result.Repositories = repos
	case KindFileSet:
		fs, err := parseFileSet(data, path)
		if err != nil {
			return nil, err
		}
		result.FileSets = []*FileSet{fs}
	default:
		return nil, fmt.Errorf("%s: unknown kind %q", path, doc.Kind)
	}

	return result, nil
}

func parseRepository(data []byte, path string) ([]*Repository, error) {
	var repo Repository
	if err := yaml.Unmarshal(data, &repo); err != nil {
		return nil, fmt.Errorf("parse Repository in %s: %w", path, err)
	}
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
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
			Kind:       KindRepository,
			Metadata: RepositoryMetadata{
				Name:  entry.Name,
				Owner: set.Metadata.Owner,
			},
			Spec: mergeSpecs(set.Defaults, entry.Spec),
		}
		if err := repo.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func parseFileSet(data []byte, path string) (*FileSet, error) {
	var fs FileSet
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("parse FileSet in %s: %w", path, err)
	}

	if err := fs.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	// Resolve source files to content
	dir := filepath.Dir(path)
	for i := range fs.Spec.Files {
		entry := &fs.Spec.Files[i]
		if entry.Source != "" && entry.Content == "" {
			srcPath := entry.Source
			if !filepath.IsAbs(srcPath) {
				srcPath = filepath.Join(dir, srcPath)
			}
			content, err := os.ReadFile(srcPath)
			if err != nil {
				return nil, fmt.Errorf("%s: read source %s: %w", path, entry.Source, err)
			}
			entry.Content = string(content)
			entry.Source = "" // resolved
		}
	}

	return &fs, nil
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
	if override.Archived != nil {
		result.Archived = override.Archived
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
