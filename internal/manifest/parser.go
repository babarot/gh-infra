package manifest

import (
	"bytes"
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
	return result.Repositories(), nil
}

// ParseOptions controls parsing behavior.
type ParseOptions struct {
	FailOnUnknown bool // Error on files with unknown Kind (default: skip)
}

// ParseAll parses a file or directory and returns all resources (Repository + FileSet).
func ParseAll(path string, opts ...ParseOptions) (*ParseResult, error) {
	var opt ParseOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return parseFileAll(path, opt)
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
		parsed, err := parseFileAll(filepath.Join(path, entry.Name()), opt)
		if err != nil {
			return nil, err
		}
		result.RepositoryDocs = append(result.RepositoryDocs, parsed.RepositoryDocs...)
		result.FileSetDocs = append(result.FileSetDocs, parsed.FileSetDocs...)
		result.Warnings = append(result.Warnings, parsed.Warnings...)
	}
	return result, nil
}

func parseFileAll(path string, opt ParseOptions) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	docs := splitDocuments(data)
	result := &ParseResult{}

	for i, docData := range docs {
		parsed, err := parseDocument(docData, path, i+1, opt)
		if err != nil {
			return nil, err
		}
		result.RepositoryDocs = append(result.RepositoryDocs, parsed.RepositoryDocs...)
		result.FileSetDocs = append(result.FileSetDocs, parsed.FileSetDocs...)
		result.Warnings = append(result.Warnings, parsed.Warnings...)
	}

	return result, nil
}

// splitDocuments splits YAML data on "---" document separators,
// returning one []byte per document. Empty documents are skipped.
func splitDocuments(data []byte) [][]byte {
	sep := []byte("\n---")
	parts := bytes.Split(data, sep)

	var docs [][]byte
	for _, part := range parts {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) == 0 {
			continue
		}
		docs = append(docs, part)
	}
	return docs
}

// parseDocument parses a single YAML document within a file.
// docNum is the 1-based document index, used for error messages.
func parseDocument(data []byte, path string, docNum int, opt ParseOptions) (*ParseResult, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		if len(splitDocuments(data)) <= 1 {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		return nil, fmt.Errorf("parse %s (document %d): %w", path, docNum, err)
	}

	result := &ParseResult{}

	switch doc.Kind {
	case KindRepository:
		repos, err := parseRepository(data, path)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			result.RepositoryDocs = append(result.RepositoryDocs, &RepositoryDocument{
				Resource:   r,
				SourcePath: path,
				DocIndex:   docNum - 1,
			})
		}
	case KindRepositorySet:
		repos, err := parseRepositorySet(data, path)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			r.SourcePath = path
			r.DocIndex = docNum - 1
			result.RepositoryDocs = append(result.RepositoryDocs, r)
		}
	case KindFile:
		fs, resolved, warnings, err := parseFile(data, path)
		if err != nil {
			return nil, err
		}
		result.FileSetDocs = []*FileSetDocument{{
			Resource:      fs,
			SourcePath:    path,
			DocIndex:      docNum - 1,
			ResolvedFiles: resolved,
		}}
		result.Warnings = append(result.Warnings, warnings...)
	case KindFileSet:
		fs, resolved, warnings, err := parseFileSet(data, path)
		if err != nil {
			return nil, err
		}
		result.FileSetDocs = []*FileSetDocument{{
			Resource:      fs,
			SourcePath:    path,
			DocIndex:      docNum - 1,
			ResolvedFiles: resolved,
		}}
		result.Warnings = append(result.Warnings, warnings...)
	default:
		if opt.FailOnUnknown {
			return nil, fmt.Errorf("%s: unknown kind %q", path, doc.Kind)
		}
	}

	return result, nil
}

func parseRepository(data []byte, path string) ([]*Repository, error) {
	var repo Repository
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&repo); err != nil {
		return nil, fmt.Errorf("parse Repository in %s: %w", path, err)
	}
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return []*Repository{&repo}, nil
}

func parseRepositorySet(data []byte, path string) ([]*RepositoryDocument, error) {
	var set RepositorySet
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&set); err != nil {
		return nil, fmt.Errorf("parse RepositorySet in %s: %w", path, err)
	}

	var docs []*RepositoryDocument
	for i, entry := range set.Repositories {
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
		docs = append(docs, &RepositoryDocument{
			Resource:      repo,
			FromSet:       true,
			SetEntryIndex: i,
		})
	}
	return docs, nil
}

func parseFile(data []byte, path string) (*FileSet, []ResolvedFile, []string, error) {
	var f File
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&f); err != nil {
		return nil, nil, nil, fmt.Errorf("parse File in %s: %w", path, err)
	}

	if err := f.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Expand File into a FileSet with a single repository entry.
	fs := &FileSet{
		APIVersion: f.APIVersion,
		Kind:       KindFileSet,
		Metadata:   FileSetMetadata{Owner: f.Metadata.Owner},
		Spec: FileSetSpec{
			Repositories:  []FileSetRepository{{Name: f.Metadata.Name}},
			Files:         f.Spec.Files,
			CommitMessage: f.Spec.CommitMessage,
			Via:           f.Spec.Via,
			Branch:        f.Spec.Branch,
			PRTitle:       f.Spec.PRTitle,
			PRBody:        f.Spec.PRBody,
		},
	}

	if err := fs.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Resolve source references (local files, directories, GitHub URLs)
	resolver := DefaultResolver
	resolved, err := resolver.ResolveFiles(fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Collect deprecation warnings
	var warnings []string
	warnings = append(warnings, f.Spec.DeprecationWarnings...)
	warnings = append(warnings, fs.Spec.DeprecationWarnings...)
	warnings = append(warnings, collectFileEntryWarnings(fs.Spec.Files)...)

	return fs, resolved, warnings, nil
}

func parseFileSet(data []byte, path string) (*FileSet, []ResolvedFile, []string, error) {
	var fs FileSet
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&fs); err != nil {
		return nil, nil, nil, fmt.Errorf("parse FileSet in %s: %w", path, err)
	}

	if err := fs.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Resolve source references (local files, directories, GitHub URLs)
	resolver := DefaultResolver
	resolved, err := resolver.ResolveFiles(fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Collect deprecation warnings
	var warnings []string
	warnings = append(warnings, fs.Spec.DeprecationWarnings...)
	warnings = append(warnings, collectFileEntryWarnings(fs.Spec.Files)...)

	return &fs, resolved, warnings, nil
}

// collectFileEntryWarnings drains deprecation warnings from all FileEntry instances.
func collectFileEntryWarnings(files []FileEntry) []string {
	var warnings []string
	for _, f := range files {
		warnings = append(warnings, f.DeprecationWarnings...)
	}
	return warnings
}

// expandDir walks a directory and returns a ResolvedFile for each file,
// with path relative to destPrefix.
func expandDir(srcDir, destPrefix string) ([]ResolvedFile, error) {
	var entries []ResolvedFile
	err := filepath.WalkDir(srcDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		destPath := rel
		if destPrefix != "" {
			destPath = filepath.Join(destPrefix, rel)
		}
		// Normalize to forward slashes for GitHub paths
		destPath = filepath.ToSlash(destPath)
		entries = append(entries, ResolvedFile{
			FileEntry: FileEntry{
				Path:    destPath,
				Content: string(content),
			},
			OriginalSource: p,
		})
		return nil
	})
	return entries, err
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
	if override.MergeStrategy != nil {
		result.MergeStrategy = mergeMergeStrategy(result.MergeStrategy, override.MergeStrategy)
	}
	if len(override.BranchProtection) > 0 {
		result.BranchProtection = override.BranchProtection
	}
	if len(override.Rulesets) > 0 {
		result.Rulesets = override.Rulesets
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
	return &result
}

func mergeMergeStrategy(base, override *MergeStrategy) *MergeStrategy {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := *base
	if override.AllowMergeCommit != nil {
		result.AllowMergeCommit = override.AllowMergeCommit
	}
	if override.AllowSquashMerge != nil {
		result.AllowSquashMerge = override.AllowSquashMerge
	}
	if override.AllowRebaseMerge != nil {
		result.AllowRebaseMerge = override.AllowRebaseMerge
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
