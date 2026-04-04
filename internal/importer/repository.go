package importer

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/yamledit"
)

// DiffInput holds the inputs for planning a single repository's import.
type DiffInput struct {
	Repos         []*manifest.RepositoryDocument
	Imported      *manifest.Repository
	ManifestBytes map[string][]byte
}

// DiffRepository compares local Repository spec vs GitHub-imported spec,
// generates FieldDiffs for display, and patches the YAML via yamledit.
func DiffRepository(input DiffInput) (RepoResult, error) {
	if len(input.Repos) == 0 {
		return RepoResult{}, nil
	}

	plan := RepoResult{ManifestEdits: make(map[string][]byte)}

	for _, doc := range input.Repos {
		local := doc.Resource.Spec
		imported := input.Imported.Spec

		// Preserve local secrets — GitHub API cannot return secret values.
		imported.Secrets = local.Secrets

		diffs := compareSpecs(local, imported)
		if len(diffs) == 0 {
			continue
		}

		fullName := doc.Resource.Metadata.FullName()
		for i := range diffs {
			diffs[i].Target = fullName
		}
		plan.Diffs = append(plan.Diffs, diffs...)

		// Patch YAML: replace $.spec with the imported spec
		data, ok := input.ManifestBytes[doc.SourcePath]
		if !ok {
			return plan, fmt.Errorf("no manifest bytes for %s", doc.SourcePath)
		}

		updated, err := yamledit.ReplaceNode(data, doc.DocIndex, "$.spec", imported)
		if err != nil {
			return plan, fmt.Errorf("yamledit replace for %s doc %d: %w", doc.SourcePath, doc.DocIndex, err)
		}

		input.ManifestBytes[doc.SourcePath] = updated
		plan.ManifestEdits[doc.SourcePath] = updated
		plan.UpdatedDocs++
	}

	return plan, nil
}

// DiffRepositorySet compares a RepositorySet-derived repo, computing the
// minimal override relative to defaults, and patches $.repositories[N].spec.
func DiffRepositorySet(input DiffInput) (RepoResult, error) {
	if len(input.Repos) == 0 {
		return RepoResult{}, nil
	}

	plan := RepoResult{ManifestEdits: make(map[string][]byte)}

	for _, doc := range input.Repos {
		if !doc.FromSet || doc.DefaultsSpec == nil {
			continue
		}

		imported := input.Imported.Spec
		imported.Secrets = doc.Resource.Spec.Secrets

		newOverride := minimalOverride(doc.DefaultsSpec.Spec, imported)

		// Diff for display: OriginalEntrySpec → newOverride
		var diffs []FieldDiff
		if doc.OriginalEntrySpec != nil {
			diffs = compareSpecs(*doc.OriginalEntrySpec, newOverride)
		} else {
			diffs = compareSpecs(manifest.RepositorySpec{}, newOverride)
		}

		if len(diffs) == 0 {
			continue
		}

		fullName := doc.Resource.Metadata.FullName()
		for i := range diffs {
			diffs[i].Target = fullName
		}
		plan.Diffs = append(plan.Diffs, diffs...)

		// Patch YAML: replace $.repositories[N].spec
		yamlPath := fmt.Sprintf("$.repositories[%d].spec", doc.SetEntryIndex)

		data, ok := input.ManifestBytes[doc.SourcePath]
		if !ok {
			return plan, fmt.Errorf("no manifest bytes for %s", doc.SourcePath)
		}

		updated, err := yamledit.ReplaceNode(data, doc.DocIndex, yamlPath, newOverride)
		if err != nil {
			return plan, fmt.Errorf("yamledit replace for %s doc %d path %s: %w",
				doc.SourcePath, doc.DocIndex, yamlPath, err)
		}

		input.ManifestBytes[doc.SourcePath] = updated
		plan.ManifestEdits[doc.SourcePath] = updated
		plan.UpdatedDocs++
	}

	return plan, nil
}

// compareSpecs compares two RepositorySpecs field by field and returns diffs.
// Covers scalar, list, and nested map fields (Phase 2a).
// Complex fields (branch_protection, rulesets) are handled separately (Phase 2c).
func compareSpecs(local, imported manifest.RepositorySpec) []FieldDiff {
	var diffs []FieldDiff

	// Scalar fields
	diffs = appendPtrDiff(diffs, "description", local.Description, imported.Description)
	diffs = appendPtrDiff(diffs, "homepage", local.Homepage, imported.Homepage)
	diffs = appendPtrDiff(diffs, "visibility", local.Visibility, imported.Visibility)
	diffs = appendBoolPtrDiff(diffs, "archived", local.Archived, imported.Archived)

	// List fields
	if !stringSliceEqual(local.Topics, imported.Topics) {
		diffs = append(diffs, FieldDiff{Field: "topics", Old: local.Topics, New: imported.Topics})
	}

	// Nested map: features
	diffs = append(diffs, compareFeatures(local.Features, imported.Features)...)

	// Nested map: merge_strategy
	diffs = append(diffs, compareMergeStrategy(local.MergeStrategy, imported.MergeStrategy)...)

	// Nested map: actions
	diffs = append(diffs, compareActions(local.Actions, imported.Actions)...)

	// Branch protection (Phase 2c)
	diffs = append(diffs, compareBranchProtection(local.BranchProtection, imported.BranchProtection)...)

	// Rulesets (Phase 2c)
	diffs = append(diffs, compareRulesets(local.Rulesets, imported.Rulesets)...)

	// Variables (Phase 2d)
	diffs = append(diffs, compareVariables(local.Variables, imported.Variables)...)

	return diffs
}

// compareFeatures compares Features fields.
func compareFeatures(local, imported *manifest.Features) []FieldDiff {
	if local == nil && imported == nil {
		return nil
	}
	var diffs []FieldDiff
	l := derefFeatures(local)
	i := derefFeatures(imported)
	diffs = appendBoolPtrDiff(diffs, "features.issues", l.Issues, i.Issues)
	diffs = appendBoolPtrDiff(diffs, "features.projects", l.Projects, i.Projects)
	diffs = appendBoolPtrDiff(diffs, "features.wiki", l.Wiki, i.Wiki)
	diffs = appendBoolPtrDiff(diffs, "features.discussions", l.Discussions, i.Discussions)
	return diffs
}

func derefFeatures(f *manifest.Features) manifest.Features {
	if f == nil {
		return manifest.Features{}
	}
	return *f
}

// compareMergeStrategy compares MergeStrategy fields.
func compareMergeStrategy(local, imported *manifest.MergeStrategy) []FieldDiff {
	if local == nil && imported == nil {
		return nil
	}
	var diffs []FieldDiff
	l := derefMergeStrategy(local)
	i := derefMergeStrategy(imported)
	diffs = appendBoolPtrDiff(diffs, "merge_strategy.allow_merge_commit", l.AllowMergeCommit, i.AllowMergeCommit)
	diffs = appendBoolPtrDiff(diffs, "merge_strategy.allow_squash_merge", l.AllowSquashMerge, i.AllowSquashMerge)
	diffs = appendBoolPtrDiff(diffs, "merge_strategy.allow_rebase_merge", l.AllowRebaseMerge, i.AllowRebaseMerge)
	diffs = appendBoolPtrDiff(diffs, "merge_strategy.auto_delete_head_branches", l.AutoDeleteHeadBranches, i.AutoDeleteHeadBranches)
	diffs = appendPtrDiff(diffs, "merge_strategy.squash_merge_commit_title", l.SquashMergeCommitTitle, i.SquashMergeCommitTitle)
	diffs = appendPtrDiff(diffs, "merge_strategy.squash_merge_commit_message", l.SquashMergeCommitMessage, i.SquashMergeCommitMessage)
	diffs = appendPtrDiff(diffs, "merge_strategy.merge_commit_title", l.MergeCommitTitle, i.MergeCommitTitle)
	diffs = appendPtrDiff(diffs, "merge_strategy.merge_commit_message", l.MergeCommitMessage, i.MergeCommitMessage)
	return diffs
}

func derefMergeStrategy(m *manifest.MergeStrategy) manifest.MergeStrategy {
	if m == nil {
		return manifest.MergeStrategy{}
	}
	return *m
}

// compareActions compares Actions fields.
func compareActions(local, imported *manifest.Actions) []FieldDiff {
	if local == nil && imported == nil {
		return nil
	}
	var diffs []FieldDiff
	l := derefActions(local)
	i := derefActions(imported)
	diffs = appendBoolPtrDiff(diffs, "actions.enabled", l.Enabled, i.Enabled)
	diffs = appendPtrDiff(diffs, "actions.allowed_actions", l.AllowedActions, i.AllowedActions)
	diffs = appendBoolPtrDiff(diffs, "actions.sha_pinning_required", l.SHAPinningRequired, i.SHAPinningRequired)
	diffs = appendPtrDiff(diffs, "actions.workflow_permissions", l.WorkflowPermissions, i.WorkflowPermissions)
	diffs = appendBoolPtrDiff(diffs, "actions.can_approve_pull_requests", l.CanApprovePullRequests, i.CanApprovePullRequests)
	diffs = appendPtrDiff(diffs, "actions.fork_pr_approval", l.ForkPRApproval, i.ForkPRApproval)
	// selected_actions
	ls := derefSelectedActions(l.SelectedActions)
	is := derefSelectedActions(i.SelectedActions)
	diffs = appendBoolPtrDiff(diffs, "actions.selected_actions.github_owned_allowed", ls.GithubOwnedAllowed, is.GithubOwnedAllowed)
	diffs = appendBoolPtrDiff(diffs, "actions.selected_actions.verified_allowed", ls.VerifiedAllowed, is.VerifiedAllowed)
	if !stringSliceEqual(ls.PatternsAllowed, is.PatternsAllowed) {
		diffs = append(diffs, FieldDiff{Field: "actions.selected_actions.patterns_allowed", Old: ls.PatternsAllowed, New: is.PatternsAllowed})
	}
	return diffs
}

func derefActions(a *manifest.Actions) manifest.Actions {
	if a == nil {
		return manifest.Actions{}
	}
	return *a
}

func derefSelectedActions(s *manifest.SelectedActions) manifest.SelectedActions {
	if s == nil {
		return manifest.SelectedActions{}
	}
	return *s
}

// compareBranchProtection compares branch protection rules by pattern.
func compareBranchProtection(local, imported []manifest.BranchProtection) []FieldDiff {
	if len(local) == 0 && len(imported) == 0 {
		return nil
	}

	localMap := make(map[string]manifest.BranchProtection)
	for _, bp := range local {
		localMap[bp.Pattern] = bp
	}
	importedMap := make(map[string]manifest.BranchProtection)
	for _, bp := range imported {
		importedMap[bp.Pattern] = bp
	}

	var diffs []FieldDiff

	// Check for changes and additions
	for pattern, ibp := range importedMap {
		lbp, exists := localMap[pattern]
		if !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("branch_protection.%s", pattern),
				Old:   nil,
				New:   ibp,
			})
			continue
		}
		if !reflect.DeepEqual(lbp, ibp) {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("branch_protection.%s", pattern),
				Old:   lbp,
				New:   ibp,
			})
		}
	}

	// Check for deletions (local has, GitHub doesn't)
	for pattern, lbp := range localMap {
		if _, exists := importedMap[pattern]; !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("branch_protection.%s", pattern),
				Old:   lbp,
				New:   nil,
			})
		}
	}

	return diffs
}

// compareRulesets compares rulesets by name.
func compareRulesets(local, imported []manifest.Ruleset) []FieldDiff {
	if len(local) == 0 && len(imported) == 0 {
		return nil
	}

	localMap := make(map[string]manifest.Ruleset)
	for _, rs := range local {
		localMap[rs.Name] = rs
	}
	importedMap := make(map[string]manifest.Ruleset)
	for _, rs := range imported {
		importedMap[rs.Name] = rs
	}

	var diffs []FieldDiff

	for name, irs := range importedMap {
		lrs, exists := localMap[name]
		if !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("rulesets.%s", name),
				Old:   nil,
				New:   irs,
			})
			continue
		}
		if !reflect.DeepEqual(lrs, irs) {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("rulesets.%s", name),
				Old:   lrs,
				New:   irs,
			})
		}
	}

	for name, lrs := range localMap {
		if _, exists := importedMap[name]; !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("rulesets.%s", name),
				Old:   lrs,
				New:   nil,
			})
		}
	}

	return diffs
}

// compareVariables compares variable lists.
func compareVariables(local, imported []manifest.Variable) []FieldDiff {
	if len(local) == 0 && len(imported) == 0 {
		return nil
	}

	localMap := make(map[string]string)
	for _, v := range local {
		localMap[v.Name] = v.Value
	}
	importedMap := make(map[string]string)
	for _, v := range imported {
		importedMap[v.Name] = v.Value
	}

	var diffs []FieldDiff

	for name, iv := range importedMap {
		lv, exists := localMap[name]
		if !exists || lv != iv {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("variables.%s", name),
				Old:   lv,
				New:   iv,
			})
		}
	}

	for name, lv := range localMap {
		if _, exists := importedMap[name]; !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("variables.%s", name),
				Old:   lv,
				New:   nil,
			})
		}
	}

	return diffs
}

// minimalOverride returns the minimal spec override relative to defaults.
// Fields identical to defaults are zeroed so omitempty drops them.
func minimalOverride(defaults, imported manifest.RepositorySpec) manifest.RepositorySpec {
	var override manifest.RepositorySpec

	// Scalar fields
	if !ptrEqual(defaults.Description, imported.Description) {
		override.Description = imported.Description
	}
	if !ptrEqual(defaults.Homepage, imported.Homepage) {
		override.Homepage = imported.Homepage
	}
	if !ptrEqual(defaults.Visibility, imported.Visibility) {
		override.Visibility = imported.Visibility
	}
	if !boolPtrEqual(defaults.Archived, imported.Archived) {
		override.Archived = imported.Archived
	}

	// List: topics — override if different
	if !stringSliceEqual(defaults.Topics, imported.Topics) {
		override.Topics = imported.Topics
	}

	// Features: key-level comparison
	override.Features = minimalFeatures(defaults.Features, imported.Features)

	// MergeStrategy: key-level comparison
	override.MergeStrategy = minimalMergeStrategy(defaults.MergeStrategy, imported.MergeStrategy)

	// Complex fields: override if different from defaults
	if !reflect.DeepEqual(defaults.BranchProtection, imported.BranchProtection) {
		override.BranchProtection = imported.BranchProtection
	}
	if !reflect.DeepEqual(defaults.Rulesets, imported.Rulesets) {
		override.Rulesets = imported.Rulesets
	}
	if !reflect.DeepEqual(defaults.Actions, imported.Actions) {
		override.Actions = imported.Actions
	}

	// Variables: override if different
	if !reflect.DeepEqual(defaults.Variables, imported.Variables) {
		override.Variables = imported.Variables
	}

	// Secrets: always preserve local override (not from import)
	// The caller already sets imported.Secrets = local.Secrets,
	// but for minimalOverride we keep whatever was in the imported spec.
	if !reflect.DeepEqual(defaults.Secrets, imported.Secrets) {
		override.Secrets = imported.Secrets
	}

	return override
}

func minimalFeatures(defaults, imported *manifest.Features) *manifest.Features {
	if imported == nil {
		return nil
	}
	d := derefFeatures(defaults)
	i := *imported
	var f manifest.Features
	any := false
	if !boolPtrEqual(d.Issues, i.Issues) {
		f.Issues = i.Issues
		any = true
	}
	if !boolPtrEqual(d.Projects, i.Projects) {
		f.Projects = i.Projects
		any = true
	}
	if !boolPtrEqual(d.Wiki, i.Wiki) {
		f.Wiki = i.Wiki
		any = true
	}
	if !boolPtrEqual(d.Discussions, i.Discussions) {
		f.Discussions = i.Discussions
		any = true
	}
	if !any {
		return nil
	}
	return &f
}

func minimalMergeStrategy(defaults, imported *manifest.MergeStrategy) *manifest.MergeStrategy {
	if imported == nil {
		return nil
	}
	d := derefMergeStrategy(defaults)
	i := *imported
	var m manifest.MergeStrategy
	any := false
	if !boolPtrEqual(d.AllowMergeCommit, i.AllowMergeCommit) {
		m.AllowMergeCommit = i.AllowMergeCommit
		any = true
	}
	if !boolPtrEqual(d.AllowSquashMerge, i.AllowSquashMerge) {
		m.AllowSquashMerge = i.AllowSquashMerge
		any = true
	}
	if !boolPtrEqual(d.AllowRebaseMerge, i.AllowRebaseMerge) {
		m.AllowRebaseMerge = i.AllowRebaseMerge
		any = true
	}
	if !boolPtrEqual(d.AutoDeleteHeadBranches, i.AutoDeleteHeadBranches) {
		m.AutoDeleteHeadBranches = i.AutoDeleteHeadBranches
		any = true
	}
	if !ptrEqual(d.SquashMergeCommitTitle, i.SquashMergeCommitTitle) {
		m.SquashMergeCommitTitle = i.SquashMergeCommitTitle
		any = true
	}
	if !ptrEqual(d.SquashMergeCommitMessage, i.SquashMergeCommitMessage) {
		m.SquashMergeCommitMessage = i.SquashMergeCommitMessage
		any = true
	}
	if !ptrEqual(d.MergeCommitTitle, i.MergeCommitTitle) {
		m.MergeCommitTitle = i.MergeCommitTitle
		any = true
	}
	if !ptrEqual(d.MergeCommitMessage, i.MergeCommitMessage) {
		m.MergeCommitMessage = i.MergeCommitMessage
		any = true
	}
	if !any {
		return nil
	}
	return &m
}

// --- helpers ---

func appendPtrDiff(diffs []FieldDiff, field string, local, imported *string) []FieldDiff {
	if ptrEqual(local, imported) {
		return diffs
	}
	return append(diffs, FieldDiff{Field: field, Old: derefStr(local), New: derefStr(imported)})
}

func appendBoolPtrDiff(diffs []FieldDiff, field string, local, imported *bool) []FieldDiff {
	if boolPtrEqual(local, imported) {
		return diffs
	}
	return append(diffs, FieldDiff{Field: field, Old: derefBool(local), New: derefBool(imported)})
}

func ptrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	// Compare sorted copies
	sa := make([]string, len(a))
	copy(sa, a)
	sort.Strings(sa)
	sb := make([]string, len(b))
	copy(sb, b)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}
