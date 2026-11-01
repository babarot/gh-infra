package manifest

import (
	"bytes"
	"context"
	"errors"
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
	// SkipRemoteSources leaves github:// sources unresolved (no API call).
	// The result is only fit for validation and for selecting documents by
	// their local dependencies, not for planning.
	SkipRemoteSources bool
	// Context cancels fetching github:// sources; nil means no cancellation.
	Context context.Context
}

func (o ParseOptions) context() context.Context {
	if o.Context == nil {
		return context.Background()
	}
	return o.Context
}

// resolver returns the source resolver to use, honoring SkipRemoteSources.
func (o ParseOptions) resolver() *SourceResolver {
	if !o.SkipRemoteSources || o.Resolver == nil {
		return o.Resolver
	}
	r := *o.Resolver
	r.skipRemote = true
	return &r
}

// localDeps returns the local files and directories that files read when
// resolved: local source: paths and patch file paths, absolute and cleaned.
// It mirrors ResolveFiles, which reads patches only for entries with a
// source: and no content:.
func localDeps(files []FileEntry, yamlDir string) []string {
	abs := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(yamlDir, p)
		}
		return filepath.Clean(p)
	}
	var deps []string
	for _, f := range files {
		if f.Source == "" || f.Content != "" {
			continue
		}
		if !strings.HasPrefix(f.Source, githubScheme) {
			deps = append(deps, abs(f.Source))
		}
		for _, p := range f.Patches {
			if !strings.ContainsRune(p, '\n') {
				deps = append(deps, abs(p))
			}
		}
	}
	return deps
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
		fs, warnings, deps, err := parseFile(opt.context(), data, path, opt.resolver())
		if err != nil {
			return nil, err
		}
		result.FileSets = []*FileSet{fs}
		result.FileDocs = []*FileDocument{{Resource: fs, SourcePath: path, DocIndex: docIndex, Files: fs.Spec.Files, LocalDeps: deps}}
		result.Warnings = append(result.Warnings, warnings...)
	case KindFileSet:
		if opt.Resolver == nil {
			return nil, fmt.Errorf("%s: ParseOptions.Resolver is required for FileSet kind", path)
		}
		fs, warnings, deps, err := parseFileSet(opt.context(), data, path, opt.resolver())
		if err != nil {
			return nil, err
		}
		result.FileSets = []*FileSet{fs}
		result.FileDocs = []*FileDocument{{Resource: fs, SourcePath: path, DocIndex: docIndex, Files: fs.Spec.Files, LocalDeps: deps}}
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

	// Check label names before merging: merging keys labels by lowercase
	// name, so names differing only in case would be folded silently.
	if set.Defaults != nil {
		if err := checkLabelNames("defaults.spec.labels", set.Defaults.Spec.Labels); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	for i := range set.Repositories {
		if err := checkLabelNames(set.Repositories[i].Name+": spec.labels", set.Repositories[i].Spec.Labels); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
		}
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
			Reconcile: mergeReconcile(set.Defaults, entry.Reconcile),
			Spec:      mergeSpecs(set.Defaults, entry.Spec),
		}
		if err := repo.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		warnings = append(warnings, emptyOverrideWarnings(set.Defaults, entry.Spec, repo)...)
		repos = append(repos, repo)
		docs = append(docs, &RepositoryDocument{
			Resource:          repo,
			SourcePath:        path,
			DocIndex:          docIndex,
			FromSet:           true,
			SetEntryIndex:     i,
			DefaultsSpec:      set.Defaults,
			OriginalEntrySpec: &originalSpec,
		})
	}
	return repos, docs, warnings, nil
}

func parseFile(ctx context.Context, data []byte, path string, resolver *SourceResolver) (*FileSet, []string, []string, error) {
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
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	deps := localDeps(fs.Spec.Files, filepath.Dir(path))

	// Resolve source references (local files, directories, GitHub URLs)
	resolved, err := resolver.ResolveFiles(ctx, fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	fs.Spec.Files = resolved

	// Collect deprecation warnings
	var warnings []string
	warnings = append(warnings, f.Spec.DeprecationWarnings...)
	warnings = append(warnings, collectFileSetWarnings(fs.Spec)...)

	return fs, warnings, deps, nil
}

func parseFileSet(ctx context.Context, data []byte, path string, resolver *SourceResolver) (*FileSet, []string, []string, error) {
	var fs FileSet
	if err := yaml.NewDecoder(bytes.NewReader(data), yaml.DisallowUnknownField()).Decode(&fs); err != nil {
		return nil, nil, nil, fmt.Errorf("parse FileSet in %s: %w", path, err)
	}

	if err := fs.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	deps := localDeps(fs.Spec.Files, filepath.Dir(path))
	for _, repo := range fs.Spec.Repositories {
		deps = append(deps, localDeps(repo.Overrides, filepath.Dir(path))...)
	}

	// Resolve source references (local files, directories, GitHub URLs)
	resolved, err := resolver.ResolveFiles(ctx, fs.Spec.Files, filepath.Dir(path))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	fs.Spec.Files = resolved

	for i, repo := range fs.Spec.Repositories {
		if len(repo.Overrides) > 0 {
			resolvedOverrides, err := resolver.ResolveFiles(ctx, repo.Overrides, filepath.Dir(path))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%s: resolve overrides for %s: %w", path, repo.Name, err)
			}
			fs.Spec.Repositories[i].Overrides = resolvedOverrides
		}
	}

	// Collect deprecation warnings
	warnings := collectFileSetWarnings(fs.Spec)

	return &fs, warnings, deps, nil
}

// emptyOverrideWarnings warns when an entry's [] drops the defaults' entries
// of a collection that is reconciled authoritatively, since every such item
// on the repository is then planned for deletion. Until v0.16.0 an entry's []
// inherited the defaults instead.
func emptyOverrideWarnings(defaults *RepositorySetDefaults, entry RepositorySpec, repo *Repository) []string {
	if defaults == nil {
		return nil
	}
	name := repo.Metadata.FullName()
	var warnings []string
	check := func(field, item string, set bool, entryLen, defaultsLen int, mode string) {
		if set && entryLen == 0 && defaultsLen > 0 && mode == CollectionReconcileAuthoritative {
			warnings = append(warnings, fmt.Sprintf(
				"%s: %s: [] does not inherit the defaults, so with authoritative reconcile every %s on the repository is planned for deletion; remove the line to inherit the defaults",
				name, field, item))
		}
	}
	check("labels", "label", entry.LabelsSet, len(entry.Labels), len(defaults.Spec.Labels), LabelsReconcileMode(repo.Reconcile, repo.Spec.LabelSync))
	check("rulesets", "ruleset", entry.RulesetsSet, len(entry.Rulesets), len(defaults.Spec.Rulesets), RulesetsReconcileMode(repo.Reconcile))
	check("branch_protection", "branch protection rule", entry.BranchProtectionSet, len(entry.BranchProtection), len(defaults.Spec.BranchProtection), BranchProtectionReconcileMode(repo.Reconcile))
	return warnings
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
	if override.TopicsSet || len(override.Topics) > 0 {
		result.Topics = override.Topics
		result.TopicsSet = true
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
	// A collection written as [] in an entry means the repository has none:
	// the defaults' entries are not inherited. A non-empty list is merged by
	// key with the defaults; leaving it out inherits them.
	if override.BranchProtectionSet {
		result.BranchProtection = mergeOrEmpty(result.BranchProtection, override.BranchProtection, mergeBranchProtection)
		result.BranchProtectionSet = true
	}
	if override.RulesetsSet {
		result.Rulesets = mergeOrEmpty(result.Rulesets, override.Rulesets, mergeRulesets)
		result.RulesetsSet = true
	}
	if override.SecretsSet {
		result.Secrets = mergeOrEmpty(result.Secrets, override.Secrets, mergeSecrets)
		result.SecretsSet = true
	}
	if override.VariablesSet {
		result.Variables = mergeOrEmpty(result.Variables, override.Variables, mergeVariables)
		result.VariablesSet = true
	}
	if override.LabelsSet {
		result.Labels = mergeOrEmpty(result.Labels, override.Labels, mergeLabels)
		result.LabelsSet = true
	}
	if override.EnvironmentsSet {
		result.Environments = mergeOrEmpty(result.Environments, override.Environments, mergeEnvironments)
		result.EnvironmentsSet = true
	}
	if override.LabelSync != nil {
		result.LabelSync = override.LabelSync
	}
	if override.MilestonesSet || len(override.Milestones) > 0 { // replaces the defaults' list
		result.Milestones = override.Milestones
		result.MilestonesSet = true
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
	if override.DeploymentBranchPolicies != nil {
		result.DeploymentBranchPolicies = override.DeploymentBranchPolicies
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
	if override.AllowAutoMerge != nil {
		result.AllowAutoMerge = override.AllowAutoMerge
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

// mergeByKey merges two slices by a string key derived from each entry. Entries
// from override with a matching key in base replace the base entry; new entries
// are appended. Order of base entries is preserved.
func mergeByKey[T any](base, override []T, key func(T) string) []T {
	return mergeByKeyWith(base, override, key, func(_, o T) T { return o })
}

// mergeByKeyWith is like mergeByKey but combines matching entries through a
// caller-supplied merge function instead of replacing them.
func mergeByKeyWith[T any](base, override []T, key func(T) string, merge func(base, override T) T) []T {
	if len(base) == 0 {
		return override
	}
	if len(override) == 0 {
		return base
	}

	index := make(map[string]int, len(base))
	result := make([]T, len(base))
	copy(result, base)
	for i, x := range result {
		index[key(x)] = i
	}

	for _, x := range override {
		k := key(x)
		if i, ok := index[k]; ok {
			result[i] = merge(result[i], x)
		} else {
			index[k] = len(result)
			result = append(result, x)
		}
	}
	return result
}

// mergeOrEmpty returns an empty collection when override is empty (the entry
// wrote []), and merges override into base otherwise.
func mergeOrEmpty[T any](base, override []T, merge func(base, override []T) []T) []T {
	if len(override) == 0 {
		return []T{}
	}
	return merge(base, override)
}

// mergeLabels merges two label slices by name, ignoring case as GitHub does.
// Override labels take precedence (including the casing of the name) for
// entries with the same name; new labels are appended.
func mergeLabels(base, override []Label) []Label {
	return mergeByKey(base, override, func(l Label) string { return strings.ToLower(l.Name) })
}

// mergeBranchProtection merges two branch protection slices by pattern.
// Same-pattern entries are merged at the field level; new patterns are appended.
func mergeBranchProtection(base, override []BranchProtection) []BranchProtection {
	return mergeByKeyWith(base, override,
		func(bp BranchProtection) string { return bp.Pattern },
		mergeBranchProtectionEntry,
	)
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
		// strict is a plain bool and comes from the entry; contexts only
		// when the entry sets them, so a set-level list is not lost.
		sc := *override.RequireStatusChecks
		if sc.Contexts == nil && base.RequireStatusChecks != nil {
			sc.Contexts = base.RequireStatusChecks.Contexts
		}
		result.RequireStatusChecks = &sc
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
	return mergeByKey(base, override, func(rs Ruleset) string { return rs.Name })
}

// mergeSecrets merges two secret slices by name. Override secrets take precedence
// for entries with the same name; new secrets are appended.
func mergeSecrets(base, override []Secret) []Secret {
	return mergeByKey(base, override, func(s Secret) string { return s.Name })
}

// mergeVariables merges two variable slices by name. Override variables take precedence
// for entries with the same name; new variables are appended.
func mergeVariables(base, override []Variable) []Variable {
	return mergeByKey(base, override, func(v Variable) string { return v.Name })
}

// mergeEnvironments merges two environment slices by name, ignoring case as
// GitHub does. Same-name entries are merged field by field; reviewers and
// deployment_branch_policy are each replaced as a whole, and variables and
// secrets are merged by name.
func mergeEnvironments(base, override []Environment) []Environment {
	return mergeByKeyWith(base, override,
		func(e Environment) string { return strings.ToLower(e.Name) },
		mergeEnvironmentEntry,
	)
}

func mergeEnvironmentEntry(base, override Environment) Environment {
	result := base
	result.Name = override.Name
	if override.WaitTimer != nil {
		result.WaitTimer = override.WaitTimer
	}
	if override.PreventSelfReview != nil {
		result.PreventSelfReview = override.PreventSelfReview
	}
	if override.CanAdminsBypass != nil {
		result.CanAdminsBypass = override.CanAdminsBypass
	}
	if override.HasReviewers() {
		result.Reviewers = override.Reviewers
		result.ReviewersSet = true
	}
	if override.DeploymentBranchPolicy != nil {
		result.DeploymentBranchPolicy = override.DeploymentBranchPolicy
	}
	if override.VariablesSet || len(override.Variables) > 0 {
		result.Variables = mergeOrEmpty(result.Variables, override.Variables, mergeVariables)
		result.VariablesSet = true
	}
	if override.SecretsSet || len(override.Secrets) > 0 {
		result.Secrets = mergeOrEmpty(result.Secrets, override.Secrets, mergeSecrets)
		result.SecretsSet = true
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
	if override.PatternsAllowed != nil { // [] clears the defaults' patterns
		result.PatternsAllowed = override.PatternsAllowed
	}
	return &result
}

// expandEnvVars replaces ${ENV_*} references with environment variable values.
// Only ENV_-prefixed names are expanded so that a manifest cannot pull
// arbitrary variables (e.g. GH_TOKEN) from the apply environment into a
// secret. It returns the expanded string along with any references that were
// not ENV_-prefixed and any ENV_* variables that are unset or empty.
func expandEnvVars(s string) (expanded string, disallowed, missing []string) {
	expanded = os.Expand(s, func(key string) string {
		if !strings.HasPrefix(key, "ENV_") {
			disallowed = append(disallowed, key)
			return "${" + key + "}"
		}
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	})
	return expanded, disallowed, missing
}

// ResolveSecrets expands ${ENV_*} references in secret values. It returns an
// error if a secret references a variable without the ENV_ prefix, or if a
// referenced ENV_* variable is unset or empty, rather than silently storing a
// corrupted value.
func ResolveSecrets(repos []*Repository) error {
	var errs []error
	resolve := func(repo *Repository, where string, secrets []Secret) {
		for i := range secrets {
			secret := &secrets[i]
			resolved, disallowed, missing := expandEnvVars(secret.Value)
			secret.Value = resolved
			for _, key := range disallowed {
				errs = append(errs, fmt.Errorf(
					"repo %s: %ssecret %q references ${%s}: only ${ENV_*} variables are allowed",
					repo.Metadata.Name, where, secret.Name, key,
				))
			}
			for _, key := range missing {
				errs = append(errs, fmt.Errorf(
					"repo %s: %ssecret %q references ${%s}, which is unset or empty",
					repo.Metadata.Name, where, secret.Name, key,
				))
			}
		}
	}
	for _, repo := range repos {
		resolve(repo, "", repo.Spec.Secrets)
		for _, env := range repo.Spec.Environments {
			resolve(repo, fmt.Sprintf("environment %q: ", env.Name), env.Secrets)
		}
	}
	return errors.Join(errs...)
}
