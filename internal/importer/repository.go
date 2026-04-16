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
		// Preserve local label_sync — this is a local policy, not GitHub state.
		imported.LabelSync = local.LabelSync

		diffs := compareSpecs(local, imported)
		if len(diffs) == 0 {
			continue
		}

		fullName := doc.Resource.Metadata.FullName()
		for i := range diffs {
			diffs[i].Target = fullName
			diffs[i].SourcePath = doc.SourcePath
		}
		plan.Diffs = append(plan.Diffs, diffs...)

		data, ok := input.ManifestBytes[doc.SourcePath]
		if !ok {
			return plan, fmt.Errorf("no manifest bytes for %s", doc.SourcePath)
		}

		updated, err := patchRepositorySpec(repositoryPatchInput{
			Data:     data,
			DocIndex: doc.DocIndex,
			BasePath: "$.spec",
			Diffs:    diffs,
			Desired:  imported,
		})
		if err != nil {
			return plan, fmt.Errorf("yamledit patch for %s doc %d: %w", doc.SourcePath, doc.DocIndex, err)
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
		imported.LabelSync = doc.Resource.Spec.LabelSync

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
			diffs[i].SourcePath = doc.SourcePath
		}
		plan.Diffs = append(plan.Diffs, diffs...)

		yamlPath := fmt.Sprintf("$.repositories[%d].spec", doc.SetEntryIndex)

		data, ok := input.ManifestBytes[doc.SourcePath]
		if !ok {
			return plan, fmt.Errorf("no manifest bytes for %s", doc.SourcePath)
		}

		updated, err := patchRepositorySpec(repositoryPatchInput{
			Data:     data,
			DocIndex: doc.DocIndex,
			BasePath: yamlPath,
			Diffs:    diffs,
			Desired:  newOverride,
		})
		if err != nil {
			return plan, fmt.Errorf("yamledit patch for %s doc %d path %s: %w",
				doc.SourcePath, doc.DocIndex, yamlPath, err)
		}

		input.ManifestBytes[doc.SourcePath] = updated
		plan.ManifestEdits[doc.SourcePath] = updated
		plan.UpdatedDocs++
	}

	return plan, nil
}

type repositoryPatchInput struct {
	Data     []byte
	DocIndex int
	BasePath string
	Diffs    []FieldDiff
	Desired  manifest.RepositorySpec
}

func patchRepositorySpec(input repositoryPatchInput) ([]byte, error) {
	plan := newRepositoryPatchPlan(input.BasePath)
	for _, diff := range input.Diffs {
		applyRepositoryDescriptor(plan, diff.Field, input.Desired)
	}

	data := input.Data
	var err error
	if len(plan.rootMerge) > 0 {
		data, err = yamledit.Merge(data, input.DocIndex, input.BasePath, plan.rootMerge)
		if err != nil {
			return nil, err
		}
	}
	if len(plan.selectedActionsMerge) > 0 {
		if plan.nestedMerges["actions"] == nil {
			plan.nestedMerges["actions"] = map[string]any{}
		}
		plan.nestedMerges["actions"]["selected_actions"] = plan.selectedActionsMerge
	}
	for _, nestedKey := range repositoryNestedMergeOrder {
		fields := plan.nestedMerges[nestedKey]
		if len(fields) == 0 {
			continue
		}
		data, err = mergeNestedObject(data, input.DocIndex, input.BasePath, nestedKey, fields)
		if err != nil {
			return nil, err
		}
	}
	for _, path := range plan.deletes {
		data, err = yamledit.Delete(data, input.DocIndex, path)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

type fieldKind int

const (
	fieldString fieldKind = iota
	fieldBool
	fieldStringSlice
	fieldCollection
)

type repositoryFieldDescriptor struct {
	diffField string
	key       string
	parentKey string
	kind      fieldKind
	stringVal func(spec manifest.RepositorySpec) *string
	boolVal   func(spec manifest.RepositorySpec) *bool
	sliceVal  func(spec manifest.RepositorySpec) []string
	valueVal  func(spec manifest.RepositorySpec) any
	prefix    string
}

type repositoryPatchPlan struct {
	basePath             string
	rootMerge            map[string]any
	nestedMerges         map[string]map[string]any
	selectedActionsMerge map[string]any
	deletes              []string
}

var repositoryNestedMergeOrder = []string{"features", "merge_strategy", "actions", "security"}

var repositoryFieldDescriptors = []repositoryFieldDescriptor{
	{
		diffField: "description",
		key:       "description",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return spec.Description
		},
	},
	{
		diffField: "homepage",
		key:       "homepage",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return spec.Homepage
		},
	},
	{
		diffField: "visibility",
		key:       "visibility",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return spec.Visibility
		},
	},
	{
		diffField: "archived",
		key:       "archived",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return spec.Archived
		},
	},
	{
		diffField: "release_immutability",
		key:       "release_immutability",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return spec.ReleaseImmutability
		},
	},
	{
		diffField: "security.vulnerability_alerts",
		parentKey: "security",
		key:       "vulnerability_alerts",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromSecurity(spec.Security, "vulnerability_alerts")
		},
	},
	{
		diffField: "security.automated_security_fixes",
		parentKey: "security",
		key:       "automated_security_fixes",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromSecurity(spec.Security, "automated_security_fixes")
		},
	},
	{
		diffField: "security.private_vulnerability_reporting",
		parentKey: "security",
		key:       "private_vulnerability_reporting",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromSecurity(spec.Security, "private_vulnerability_reporting")
		},
	},
	{
		diffField: "topics",
		key:       "topics",
		kind:      fieldStringSlice,
		sliceVal: func(spec manifest.RepositorySpec) []string {
			return spec.Topics
		},
	},
	{
		diffField: "features.issues",
		parentKey: "features",
		key:       "issues",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromFeatures(spec.Features, "issues")
		},
	},
	{
		diffField: "features.projects",
		parentKey: "features",
		key:       "projects",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromFeatures(spec.Features, "projects")
		},
	},
	{
		diffField: "features.wiki",
		parentKey: "features",
		key:       "wiki",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromFeatures(spec.Features, "wiki")
		},
	},
	{
		diffField: "features.discussions",
		parentKey: "features",
		key:       "discussions",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromFeatures(spec.Features, "discussions")
		},
	},
	{
		diffField: "merge_strategy.allow_merge_commit",
		parentKey: "merge_strategy",
		key:       "allow_merge_commit",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromMergeStrategy(spec.MergeStrategy, "allow_merge_commit")
		},
	},
	{
		diffField: "merge_strategy.allow_squash_merge",
		parentKey: "merge_strategy",
		key:       "allow_squash_merge",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromMergeStrategy(spec.MergeStrategy, "allow_squash_merge")
		},
	},
	{
		diffField: "merge_strategy.allow_rebase_merge",
		parentKey: "merge_strategy",
		key:       "allow_rebase_merge",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromMergeStrategy(spec.MergeStrategy, "allow_rebase_merge")
		},
	},
	{
		diffField: "merge_strategy.allow_auto_merge",
		parentKey: "merge_strategy",
		key:       "allow_auto_merge",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromMergeStrategy(spec.MergeStrategy, "allow_auto_merge")
		},
	},
	{
		diffField: "merge_strategy.auto_delete_head_branches",
		parentKey: "merge_strategy",
		key:       "auto_delete_head_branches",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromMergeStrategy(spec.MergeStrategy, "auto_delete_head_branches")
		},
	},
	{
		diffField: "merge_strategy.squash_merge_commit_title",
		parentKey: "merge_strategy",
		key:       "squash_merge_commit_title",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromMergeStrategy(spec.MergeStrategy, "squash_merge_commit_title")
		},
	},
	{
		diffField: "merge_strategy.squash_merge_commit_message",
		parentKey: "merge_strategy",
		key:       "squash_merge_commit_message",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromMergeStrategy(spec.MergeStrategy, "squash_merge_commit_message")
		},
	},
	{
		diffField: "merge_strategy.merge_commit_title",
		parentKey: "merge_strategy",
		key:       "merge_commit_title",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromMergeStrategy(spec.MergeStrategy, "merge_commit_title")
		},
	},
	{
		diffField: "merge_strategy.merge_commit_message",
		parentKey: "merge_strategy",
		key:       "merge_commit_message",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromMergeStrategy(spec.MergeStrategy, "merge_commit_message")
		},
	},
	{
		diffField: "actions.enabled",
		parentKey: "actions",
		key:       "enabled",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromActions(spec.Actions, "enabled")
		},
	},
	{
		diffField: "actions.allowed_actions",
		parentKey: "actions",
		key:       "allowed_actions",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromActions(spec.Actions, "allowed_actions")
		},
	},
	{
		diffField: "actions.sha_pinning_required",
		parentKey: "actions",
		key:       "sha_pinning_required",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromActions(spec.Actions, "sha_pinning_required")
		},
	},
	{
		diffField: "actions.workflow_permissions",
		parentKey: "actions",
		key:       "workflow_permissions",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromActions(spec.Actions, "workflow_permissions")
		},
	},
	{
		diffField: "actions.can_approve_pull_requests",
		parentKey: "actions",
		key:       "can_approve_pull_requests",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromActions(spec.Actions, "can_approve_pull_requests")
		},
	},
	{
		diffField: "actions.fork_pr_approval",
		parentKey: "actions",
		key:       "fork_pr_approval",
		kind:      fieldString,
		stringVal: func(spec manifest.RepositorySpec) *string {
			return stringPtrFromActions(spec.Actions, "fork_pr_approval")
		},
	},
	{
		diffField: "actions.selected_actions.github_owned_allowed",
		parentKey: "actions.selected_actions",
		key:       "github_owned_allowed",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromSelectedActions(spec.Actions, "github_owned_allowed")
		},
	},
	{
		diffField: "actions.selected_actions.verified_allowed",
		parentKey: "actions.selected_actions",
		key:       "verified_allowed",
		kind:      fieldBool,
		boolVal: func(spec manifest.RepositorySpec) *bool {
			return boolPtrFromSelectedActions(spec.Actions, "verified_allowed")
		},
	},
	{
		diffField: "actions.selected_actions.patterns_allowed",
		parentKey: "actions.selected_actions",
		key:       "patterns_allowed",
		kind:      fieldStringSlice,
		sliceVal: func(spec manifest.RepositorySpec) []string {
			return patternsFromSelectedActions(spec.Actions)
		},
	},
	{
		prefix: "branch_protection.",
		key:    "branch_protection",
		kind:   fieldCollection,
		valueVal: func(spec manifest.RepositorySpec) any {
			return spec.BranchProtection
		},
	},
	{
		prefix: "rulesets.",
		key:    "rulesets",
		kind:   fieldCollection,
		valueVal: func(spec manifest.RepositorySpec) any {
			return spec.Rulesets
		},
	},
	{
		prefix: "variables.",
		key:    "variables",
		kind:   fieldCollection,
		valueVal: func(spec manifest.RepositorySpec) any {
			return spec.Variables
		},
	},
	{
		prefix: "labels.",
		key:    "labels",
		kind:   fieldCollection,
		valueVal: func(spec manifest.RepositorySpec) any {
			return spec.Labels
		},
	},
}

func newRepositoryPatchPlan(basePath string) *repositoryPatchPlan {
	return &repositoryPatchPlan{
		basePath:             basePath,
		rootMerge:            map[string]any{},
		nestedMerges:         map[string]map[string]any{},
		selectedActionsMerge: map[string]any{},
	}
}

func applyRepositoryDescriptor(plan *repositoryPatchPlan, field string, desired manifest.RepositorySpec) {
	for _, desc := range repositoryFieldDescriptors {
		if desc.matches(field) {
			desc.apply(plan, desired)
			return
		}
	}
}

func (d repositoryFieldDescriptor) matches(field string) bool {
	if d.diffField != "" {
		return d.diffField == field
	}
	return isPrefixedField(field, d.prefix)
}

func (d repositoryFieldDescriptor) apply(plan *repositoryPatchPlan, spec manifest.RepositorySpec) {
	switch d.kind {
	case fieldString:
		applyStringDescriptor(plan, d, d.stringVal(spec))
	case fieldBool:
		applyBoolDescriptor(plan, d, d.boolVal(spec))
	case fieldStringSlice:
		applyStringSliceDescriptor(plan, d, d.sliceVal(spec))
	case fieldCollection:
		applyCollectionDescriptor(plan, d, d.valueVal(spec))
	}
}

func applyStringDescriptor(plan *repositoryPatchPlan, desc repositoryFieldDescriptor, value *string) {
	if value == nil {
		plan.deletes = append(plan.deletes, plan.pathFor(desc))
		return
	}
	plan.mergeTarget(desc)[desc.key] = *value
}

func applyBoolDescriptor(plan *repositoryPatchPlan, desc repositoryFieldDescriptor, value *bool) {
	if value == nil {
		plan.deletes = append(plan.deletes, plan.pathFor(desc))
		return
	}
	plan.mergeTarget(desc)[desc.key] = *value
}

func applyStringSliceDescriptor(plan *repositoryPatchPlan, desc repositoryFieldDescriptor, value []string) {
	if len(value) == 0 {
		plan.deletes = append(plan.deletes, plan.pathFor(desc))
		return
	}
	plan.mergeTarget(desc)[desc.key] = value
}

func applyCollectionDescriptor(plan *repositoryPatchPlan, desc repositoryFieldDescriptor, value any) {
	if reflect.ValueOf(value).Len() == 0 {
		plan.deletes = append(plan.deletes, plan.pathFor(desc))
		return
	}
	plan.rootMerge[desc.key] = value
}

func (p *repositoryPatchPlan) mergeTarget(desc repositoryFieldDescriptor) map[string]any {
	switch desc.parentKey {
	case "":
		return p.rootMerge
	case "actions.selected_actions":
		return p.selectedActionsMerge
	default:
		if p.nestedMerges[desc.parentKey] == nil {
			p.nestedMerges[desc.parentKey] = map[string]any{}
		}
		return p.nestedMerges[desc.parentKey]
	}
}

func (p *repositoryPatchPlan) pathFor(desc repositoryFieldDescriptor) string {
	if desc.parentKey == "" {
		return p.basePath + "." + desc.key
	}
	return p.basePath + "." + desc.parentKey + "." + desc.key
}

func mergeNestedObject(data []byte, docIndex int, parentPath, key string, fields map[string]any) ([]byte, error) {
	childPath := parentPath + "." + key
	exists, err := yamledit.Exists(data, docIndex, childPath)
	if err != nil {
		return nil, err
	}
	if exists {
		return yamledit.Merge(data, docIndex, childPath, fields)
	}
	return yamledit.Merge(data, docIndex, parentPath, map[string]any{key: fields})
}

func isPrefixedField(field, prefix string) bool {
	return len(field) > len(prefix) && field[:len(prefix)] == prefix
}

func boolPtrFromSecurity(s *manifest.Security, field string) *bool {
	if s == nil {
		return nil
	}
	switch field {
	case "vulnerability_alerts":
		return s.VulnerabilityAlerts
	case "automated_security_fixes":
		return s.AutomatedSecurityFixes
	case "private_vulnerability_reporting":
		return s.PrivateVulnerabilityReporting
	default:
		return nil
	}
}

func boolPtrFromFeatures(f *manifest.Features, field string) *bool {
	if f == nil {
		return nil
	}
	switch field {
	case "issues":
		return f.Issues
	case "projects":
		return f.Projects
	case "wiki":
		return f.Wiki
	case "discussions":
		return f.Discussions
	default:
		return nil
	}
}

func boolPtrFromMergeStrategy(m *manifest.MergeStrategy, field string) *bool {
	if m == nil {
		return nil
	}
	switch field {
	case "allow_merge_commit":
		return m.AllowMergeCommit
	case "allow_squash_merge":
		return m.AllowSquashMerge
	case "allow_rebase_merge":
		return m.AllowRebaseMerge
	case "allow_auto_merge":
		return m.AllowAutoMerge
	case "auto_delete_head_branches":
		return m.AutoDeleteHeadBranches
	default:
		return nil
	}
}

func stringPtrFromMergeStrategy(m *manifest.MergeStrategy, field string) *string {
	if m == nil {
		return nil
	}
	switch field {
	case "squash_merge_commit_title":
		return m.SquashMergeCommitTitle
	case "squash_merge_commit_message":
		return m.SquashMergeCommitMessage
	case "merge_commit_title":
		return m.MergeCommitTitle
	case "merge_commit_message":
		return m.MergeCommitMessage
	default:
		return nil
	}
}

func boolPtrFromActions(a *manifest.Actions, field string) *bool {
	if a == nil {
		return nil
	}
	switch field {
	case "enabled":
		return a.Enabled
	case "sha_pinning_required":
		return a.SHAPinningRequired
	case "can_approve_pull_requests":
		return a.CanApprovePullRequests
	default:
		return nil
	}
}

func stringPtrFromActions(a *manifest.Actions, field string) *string {
	if a == nil {
		return nil
	}
	switch field {
	case "allowed_actions":
		return a.AllowedActions
	case "workflow_permissions":
		return a.WorkflowPermissions
	case "fork_pr_approval":
		return a.ForkPRApproval
	default:
		return nil
	}
}

func boolPtrFromSelectedActions(a *manifest.Actions, field string) *bool {
	if a == nil || a.SelectedActions == nil {
		return nil
	}
	switch field {
	case "github_owned_allowed":
		return a.SelectedActions.GithubOwnedAllowed
	case "verified_allowed":
		return a.SelectedActions.VerifiedAllowed
	default:
		return nil
	}
}

func patternsFromSelectedActions(a *manifest.Actions) []string {
	if a == nil || a.SelectedActions == nil {
		return nil
	}
	return a.SelectedActions.PatternsAllowed
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
	diffs = appendBoolPtrDiff(diffs, "release_immutability", local.ReleaseImmutability, imported.ReleaseImmutability)

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

	// Nested map: security
	diffs = append(diffs, compareSecurity(local.Security, imported.Security)...)

	// Branch protection (Phase 2c)
	diffs = append(diffs, compareBranchProtection(local.BranchProtection, imported.BranchProtection)...)

	// Rulesets (Phase 2c)
	diffs = append(diffs, compareRulesets(local.Rulesets, imported.Rulesets)...)

	// Variables (Phase 2d)
	diffs = append(diffs, compareVariables(local.Variables, imported.Variables)...)

	// Labels
	diffs = append(diffs, compareLabels(local.Labels, imported.Labels)...)

	return diffs
}

// compareSecurity compares Security fields.
func compareSecurity(local, imported *manifest.Security) []FieldDiff {
	if local == nil && imported == nil {
		return nil
	}
	var diffs []FieldDiff
	l := derefSecurity(local)
	i := derefSecurity(imported)
	diffs = appendBoolPtrDiff(diffs, "security.vulnerability_alerts", l.VulnerabilityAlerts, i.VulnerabilityAlerts)
	diffs = appendBoolPtrDiff(diffs, "security.automated_security_fixes", l.AutomatedSecurityFixes, i.AutomatedSecurityFixes)
	diffs = appendBoolPtrDiff(diffs, "security.private_vulnerability_reporting", l.PrivateVulnerabilityReporting, i.PrivateVulnerabilityReporting)
	return diffs
}

func derefSecurity(s *manifest.Security) manifest.Security {
	if s == nil {
		return manifest.Security{}
	}
	return *s
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
	diffs = appendBoolPtrDiff(diffs, "merge_strategy.allow_auto_merge", l.AllowAutoMerge, i.AllowAutoMerge)
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

// compareLabels compares label lists.
func compareLabels(local, imported []manifest.Label) []FieldDiff {
	if len(local) == 0 && len(imported) == 0 {
		return nil
	}

	type labelVal struct {
		Color       string
		Description string
	}

	localMap := make(map[string]labelVal)
	for _, l := range local {
		localMap[l.Name] = labelVal{Color: l.Color, Description: l.Description}
	}
	importedMap := make(map[string]labelVal)
	for _, l := range imported {
		importedMap[l.Name] = labelVal{Color: l.Color, Description: l.Description}
	}

	var diffs []FieldDiff

	for name, iv := range importedMap {
		lv, exists := localMap[name]
		if !exists || lv != iv {
			var old any
			if exists {
				old = formatLabelSummary(lv.Color, lv.Description)
			}
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("labels.%s", name),
				Old:   old,
				New:   formatLabelSummary(iv.Color, iv.Description),
			})
		}
	}

	for name, lv := range localMap {
		if _, exists := importedMap[name]; !exists {
			diffs = append(diffs, FieldDiff{
				Field: fmt.Sprintf("labels.%s", name),
				Old:   formatLabelSummary(lv.Color, lv.Description),
				New:   nil,
			})
		}
	}

	return diffs
}

func formatLabelSummary(color, description string) string {
	if description != "" {
		return fmt.Sprintf("#%s %q", color, description)
	}
	return "#" + color
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

	// Labels: override if different
	if !reflect.DeepEqual(defaults.Labels, imported.Labels) {
		override.Labels = imported.Labels
	}

	// Secrets: always preserve local override (not from import)
	// The caller already sets imported.Secrets = local.Secrets,
	// but for minimalOverride we keep whatever was in the imported spec.
	if !reflect.DeepEqual(defaults.Secrets, imported.Secrets) {
		override.Secrets = imported.Secrets
	}

	// LabelSync: preserve local policy (not from import)
	if !ptrEqual(defaults.LabelSync, imported.LabelSync) {
		override.LabelSync = imported.LabelSync
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
	if !boolPtrEqual(d.AllowAutoMerge, i.AllowAutoMerge) {
		m.AllowAutoMerge = i.AllowAutoMerge
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
	return derefStr(a) == derefStr(b)
}

func boolPtrEqual(a, b *bool) bool {
	return derefBool(a) == derefBool(b)
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
