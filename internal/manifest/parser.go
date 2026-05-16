package manifest

import (
	"bytes"
	"context"
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

// ParseOptions controls parsing behavior.
type ParseOptions struct {
	FailOnUnknown bool            // Error on files with unknown Kind (default: skip)
	Resolver      *SourceResolver // Source resolver for File/FileSet entries (required when parsing File/FileSet kinds)
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
		result.Repositories = append(result.Repositories, parsed.Repositories...)
		result.FileSets = append(result.FileSets, parsed.FileSets...)
		result.RepositoryDocs = append(result.RepositoryDocs, parsed.RepositoryDocs...)
		result.FileDocs = append(result.FileDocs, parsed.FileDocs...)
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
		result.Repositories = append(result.Repositories, parsed.Repositories...)
		result.FileSets = append(result.FileSets, parsed.FileSets...)
		result.RepositoryDocs = append(result.RepositoryDocs, parsed.RepositoryDocs...)
		result.FileDocs = append(result.FileDocs, parsed.FileDocs...)
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

	docIndex := docNum - 1 // convert 1-based to 0-based

	switch doc.Kind {
	case KindRepository:
		repos, warnings, err := parseRepository(data, path)
		if err != nil {
			return nil, err
		}
		result.Repositories = repos
		result.Warnings = append(result.Warnings, warnings...)
		for _, r := range repos {
			result.RepositoryDocs = append(result.RepositoryDocs, &RepositoryDocument{
				Resource:   r,
				SourcePath: path,
				DocIndex:   docIndex,
			})
		}
	case KindRepositorySet:
		repos, docs, warnings, err := parseRepositorySet(data, path, docIndex)
		if err != nil {
			return nil, err
		}
		result.Repositories = repos
		result.RepositoryDocs = docs
		result.Warnings = append(result.Warnings, warnings...)
	case KindFile:
		if opt.Resolver == nil {
			return nil, fmt.Errorf("%s: ParseOptions.Resolver is required for File kind", path)
		}
		fs, warnings, err := parseFile(data, path, opt.Resolver)
		if err != nil {
			return nil, err
		}
		result.FileSets = []*FileSet{fs}
		result.FileDocs = []*FileDocument{{Resource: fs, SourcePath: path, DocIndex: docIndex, Files: fs.Spec.Files}}
		result.Warnings = append(result.Warnings, warnings...)
	case KindFileSet:
		if opt.Resolver == nil {
			return nil, fmt.Errorf("%s: ParseOptions.Resolver is required for FileSet kind", path)
		}
		fs, warnings, err := parseFileSet(data, path, opt.Resolver)
		if err != nil {
			return nil, err
		}
		result.FileSets = []*FileSet{fs}
		result.FileDocs = []*FileDocument{{Resource: fs, SourcePath: path, DocIndex: docIndex, Files: fs.Spec.Files}}
		result.Warnings = append(result.Warnings, warnings...)
	default:
		if opt.FailOnUnknown {
			return nil, fmt.Errorf("%s: unknown kind %q", path, doc.Kind)
		}
	}

	return result, nil
}

func parseRepository(data []byte, path string) ([]*Repository, []string, error) {
	var repo Repository
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&repo); err != nil {
		return nil, nil, fmt.Errorf("parse Repository in %s: %w", path, err)
	}
	if err := repo.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return []*Repository{&repo}, repositoryWarnings(repo.Spec), nil
}

func parseRepositorySet(data []byte, path string, docIndex int) ([]*Repository, []*RepositoryDocument, []string, error) {
	var set RepositorySet
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&set); err != nil {
		return nil, nil, nil, fmt.Errorf("parse RepositorySet in %s: %w", path, err)
	}

	var warnings []string
	if set.Defaults != nil {
		warnings = append(warnings, repositoryWarnings(set.Defaults.Spec)...)
	}
	for i := range set.Repositories {
		warnings = append(warnings, repositoryWarnings(set.Repositories[i].Spec)...)
	}

	var repos []*Repository
	var docs []*RepositoryDocument
	for i := range set.Repositories {
		entry := set.Repositories[i]
		originalSpec := entry.Spec // copy before merge
		repo := &Repository{
			APIVersion: set.APIVersion,
			Kind:       KindRepository,
			Metadata: RepositoryMetadata{
				Name:  entry.Name,
				Owner: set.Metadata.Owner,
			},
			Reconcile:       mergeReconcile(set.Defaults, entry.Reconcile),
			Condition:       entry.When,
			ConditionalSpec: entry.ConditionalSpec,
			// ConditionalSpec is NOT merged with defaults — it is applied only
			// when the condition is met, evaluated in the diff layer.
			Spec: MergeSpecs(set.Defaults, entry.Spec),
		}
		if err := repo.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		repos = append(repos, repo)
		docs = append(docs, &RepositoryDocument{
			Resource:            repo,
			SourcePath:          path,
			DocIndex:            docIndex,
			FromSet:             true,
			SetEntryIndex:       i,
			DefaultsSpec:        set.Defaults,
			OriginalEntrySpec:   &originalSpec,
			OriginalCondition:   entry.When,
			OriginalConditional: entry.ConditionalSpec,
		})
	}
	return repos, docs, warnings, nil
}

func parseFile(data []byte, path string, resolver *SourceResolver) (*FileSet, []string, error) {
	var f File
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&f); err != nil {
		return nil, nil, fmt.Errorf("parse File in %s: %w", path, err)
	}

	if err := f.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Expand File into a FileSet with a single repository entry.
	fs := &FileSet{
		APIVersion: f.APIVersion,
		Kind:       KindFileSet,
		Metadata:   FileSetMetadata{Name: f.Metadata.Name, Owner: f.Metadata.Owner},
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
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Resolve source references (local files, directories, GitHub URLs)
	resolved, err := resolver.ResolveFiles(context.Background(), fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	fs.Spec.Files = resolved

	// Collect deprecation warnings
	var warnings []string
	warnings = append(warnings, f.Spec.DeprecationWarnings...)
	warnings = append(warnings, collectFileSetWarnings(fs.Spec)...)

	return fs, warnings, nil
}

func parseFileSet(data []byte, path string, resolver *SourceResolver) (*FileSet, []string, error) {
	var fs FileSet
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&fs); err != nil {
		return nil, nil, fmt.Errorf("parse FileSet in %s: %w", path, err)
	}

	if err := fs.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	// Resolve source references (local files, directories, GitHub URLs)
	resolved, err := resolver.ResolveFiles(context.Background(), fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	fs.Spec.Files = resolved

	// Collect deprecation warnings
	warnings := collectFileSetWarnings(fs.Spec)

	return &fs, warnings, nil
}

func repositoryWarnings(spec RepositorySpec) []string {
	if spec.LabelSync == nil {
		return nil
	}
	return []string{`"label_sync" is deprecated, use top-level "reconcile.labels" instead`}
}

// collectFileEntryWarnings drains deprecation warnings from all FileEntry instances.
func collectFileEntryWarnings(files []FileEntry) []string {
	var warnings []string
	for _, f := range files {
		warnings = append(warnings, f.DeprecationWarnings...)
	}
	return warnings
}

func collectFileSetWarnings(spec FileSetSpec) []string {
	var warnings []string
	warnings = append(warnings, spec.DeprecationWarnings...)
	warnings = append(warnings, collectFileEntryWarnings(spec.Files)...)
	for _, repo := range spec.Repositories {
		warnings = append(warnings, collectFileEntryWarnings(repo.Overrides)...)
	}
	return warnings
}

// expandDir walks a directory and returns a FileEntry for each file,
// with path relative to destPrefix.
func expandDir(srcDir, destPrefix string) ([]FileEntry, error) {
	var entries []FileEntry
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
		entries = append(entries, FileEntry{
			Path:           destPath,
			Content:        string(content),
			OriginalSource: p,
		})
		return nil
	})
	return entries, err
}

// MergeSpecs merges defaults with per-repo overrides. Per-repo values take precedence.
func MergeSpecs(defaults *RepositorySetDefaults, override RepositorySpec) RepositorySpec {
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
	if override.ReleaseImmutability != nil {
		result.ReleaseImmutability = override.ReleaseImmutability
	}
	if override.Security != nil {
		result.Security = mergeSecurity(result.Security, override.Security)
	}
	if override.Actions != nil {
		result.Actions = mergeActions(result.Actions, override.Actions)
	}
	if override.BranchProtectionSet {
		result.BranchProtection = mergeBranchProtection(result.BranchProtection, override.BranchProtection)
		result.BranchProtectionSet = true
	}
	if override.RulesetsSet {
		result.Rulesets = mergeRulesets(result.Rulesets, override.Rulesets)
		result.RulesetsSet = true
	}
	if len(override.Secrets) > 0 {
		result.Secrets = override.Secrets
	}
	if len(override.Variables) > 0 {
		result.Variables = override.Variables
	}
	if override.LabelsSet {
		result.Labels = mergeLabels(result.Labels, override.Labels)
		result.LabelsSet = true
	}
	if override.LabelSync != nil {
		result.LabelSync = override.LabelSync
	}
	if len(override.Milestones) > 0 {
		result.Milestones = override.Milestones
	}

	return result
}

func mergeReconcile(defaults *RepositorySetDefaults, override *RepositoryReconcile) *RepositoryReconcile {
	var base *RepositoryReconcile
	if defaults != nil {
		base = defaults.Reconcile
	}
	if base == nil {
		return override
	}
	if override == nil {
		result := *base
		return &result
	}

	result := *base
	if override.BranchProtection != nil {
		result.BranchProtection = override.BranchProtection
	}
	if override.Labels != nil {
		result.Labels = override.Labels
	}
	if override.Rulesets != nil {
		result.Rulesets = override.Rulesets
	}
	return &result
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
	if override.PullRequests != nil {
		result.PullRequests = mergePullRequests(result.PullRequests, override.PullRequests)
	}
	return &result
}

func mergePullRequests(base, override *PullRequests) *PullRequests {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	result := *base
	if override.Enabled != nil {
		result.Enabled = override.Enabled
	}
	if override.Creation != nil {
		result.Creation = override.Creation
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

func mergeSecurity(base, override *Security) *Security {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	result := *base
	if override.VulnerabilityAlerts != nil {
		result.VulnerabilityAlerts = override.VulnerabilityAlerts
	}
	if override.AutomatedSecurityFixes != nil {
		result.AutomatedSecurityFixes = override.AutomatedSecurityFixes
	}
	if override.PrivateVulnerabilityReporting != nil {
		result.PrivateVulnerabilityReporting = override.PrivateVulnerabilityReporting
	}
	return &result
}

func mergeActions(base, override *Actions) *Actions {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	result := *base
	if override.Enabled != nil {
		result.Enabled = override.Enabled
	}
	if override.AllowedActions != nil {
		result.AllowedActions = override.AllowedActions
	}
	if override.SHAPinningRequired != nil {
		result.SHAPinningRequired = override.SHAPinningRequired
	}
	if override.WorkflowPermissions != nil {
		result.WorkflowPermissions = override.WorkflowPermissions
	}
	if override.CanApprovePullRequests != nil {
		result.CanApprovePullRequests = override.CanApprovePullRequests
	}
	if override.ForkPRApproval != nil {
		result.ForkPRApproval = override.ForkPRApproval
	}
	if override.SelectedActions != nil {
		result.SelectedActions = mergeSelectedActions(result.SelectedActions, override.SelectedActions)
	}
	return &result
}

// mergeLabels merges two label slices by name. Override labels take precedence
// for entries with the same name; new labels are appended.
func mergeLabels(base, override []Label) []Label {
	if len(base) == 0 {
		return override
	}
	if len(override) == 0 {
		return base
	}

	index := make(map[string]int, len(base))
	result := make([]Label, len(base))
	copy(result, base)
	for i, l := range result {
		index[l.Name] = i
	}

	for _, l := range override {
		if i, ok := index[l.Name]; ok {
			result[i] = l
		} else {
			index[l.Name] = len(result)
			result = append(result, l)
		}
	}
	return result
}

// mergeBranchProtection merges two branch protection slices by pattern.
// Same-pattern entries are merged at the field level; new patterns are appended.
func mergeBranchProtection(base, override []BranchProtection) []BranchProtection {
	if len(base) == 0 {
		return override
	}
	if len(override) == 0 {
		return base
	}

	index := make(map[string]int, len(base))
	result := make([]BranchProtection, len(base))
	copy(result, base)
	for i, bp := range result {
		index[bp.Pattern] = i
	}

	for _, bp := range override {
		if i, ok := index[bp.Pattern]; ok {
			result[i] = mergeBranchProtectionEntry(result[i], bp)
		} else {
			index[bp.Pattern] = len(result)
			result = append(result, bp)
		}
	}
	return result
}

func mergeBranchProtectionEntry(base, override BranchProtection) BranchProtection {
	result := base
	if override.RequiredReviews != nil {
		result.RequiredReviews = override.RequiredReviews
	}
	if override.DismissStaleReviews != nil {
		result.DismissStaleReviews = override.DismissStaleReviews
	}
	if override.RequireCodeOwnerReviews != nil {
		result.RequireCodeOwnerReviews = override.RequireCodeOwnerReviews
	}
	if override.RequireStatusChecks != nil {
		result.RequireStatusChecks = override.RequireStatusChecks
	}
	if override.EnforceAdmins != nil {
		result.EnforceAdmins = override.EnforceAdmins
	}
	if override.RestrictPushes != nil {
		result.RestrictPushes = override.RestrictPushes
	}
	if override.AllowForcePushes != nil {
		result.AllowForcePushes = override.AllowForcePushes
	}
	if override.AllowDeletions != nil {
		result.AllowDeletions = override.AllowDeletions
	}
	return result
}

// mergeRulesets merges two ruleset slices by name. Override rulesets take precedence
// for entries with the same name; new rulesets are appended.
func mergeRulesets(base, override []Ruleset) []Ruleset {
	if len(base) == 0 {
		return override
	}
	if len(override) == 0 {
		return base
	}

	index := make(map[string]int, len(base))
	result := make([]Ruleset, len(base))
	copy(result, base)
	for i, rs := range result {
		index[rs.Name] = i
	}

	for _, rs := range override {
		if i, ok := index[rs.Name]; ok {
			result[i] = rs
		} else {
			index[rs.Name] = len(result)
			result = append(result, rs)
		}
	}
	return result
}

func mergeSelectedActions(base, override *SelectedActions) *SelectedActions {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	result := *base
	if override.GithubOwnedAllowed != nil {
		result.GithubOwnedAllowed = override.GithubOwnedAllowed
	}
	if override.VerifiedAllowed != nil {
		result.VerifiedAllowed = override.VerifiedAllowed
	}
	if len(override.PatternsAllowed) > 0 {
		result.PatternsAllowed = override.PatternsAllowed
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
		if repo.ConditionalSpec != nil {
			for i := range repo.ConditionalSpec.Secrets {
				repo.ConditionalSpec.Secrets[i].Value = expandEnvVars(repo.ConditionalSpec.Secrets[i].Value)
			}
		}
	}
}
