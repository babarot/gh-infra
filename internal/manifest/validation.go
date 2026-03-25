package manifest

import (
	"fmt"
	"strings"
)

// Validate checks that the Repository has valid field values.
func (r *Repository) Validate() error {
	if r.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if r.Metadata.Owner == "" {
		return fmt.Errorf("metadata.owner is required for %q", r.Metadata.Name)
	}
	if r.Spec.Visibility != nil {
		if err := validateOneOf("visibility", *r.Spec.Visibility,
			VisibilityPublic, VisibilityPrivate, VisibilityInternal); err != nil {
			return fmt.Errorf("%s: %w", r.Metadata.Name, err)
		}
	}
	if ms := r.Spec.MergeStrategy; ms != nil {
		if ms.SquashMergeCommitTitle != nil {
			if err := validateOneOf("squash_merge_commit_title", *ms.SquashMergeCommitTitle,
				SquashMergeCommitTitlePRTitle, SquashMergeCommitTitleCommitOrPRTitle); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if ms.SquashMergeCommitMessage != nil {
			if err := validateOneOf("squash_merge_commit_message", *ms.SquashMergeCommitMessage,
				SquashMergeCommitMessageCommitMessages, SquashMergeCommitMessagePRBody, SquashMergeCommitMessageBlank); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if ms.MergeCommitTitle != nil {
			if err := validateOneOf("merge_commit_title", *ms.MergeCommitTitle,
				MergeCommitTitleMergeMessage, MergeCommitTitlePRTitle); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if ms.MergeCommitMessage != nil {
			if err := validateOneOf("merge_commit_message", *ms.MergeCommitMessage,
				MergeCommitMessagePRTitle, MergeCommitMessagePRBody, MergeCommitMessageBlank); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
	}
	bpPatterns := make(map[string]bool)
	for _, bp := range r.Spec.BranchProtection {
		if bp.Pattern == "" {
			return fmt.Errorf("%s: branch_protection.pattern is required", r.Metadata.Name)
		}
		if bpPatterns[bp.Pattern] {
			return fmt.Errorf("%s: duplicate branch_protection pattern %q", r.Metadata.Name, bp.Pattern)
		}
		bpPatterns[bp.Pattern] = true
	}
	rulesetNames := make(map[string]bool)
	for _, rs := range r.Spec.Rulesets {
		if rs.Name == "" {
			return fmt.Errorf("%s: rulesets[].name is required", r.Metadata.Name)
		}
		if rulesetNames[rs.Name] {
			return fmt.Errorf("%s: duplicate ruleset name %q", r.Metadata.Name, rs.Name)
		}
		rulesetNames[rs.Name] = true
		if rs.Enforcement != nil {
			if err := validateOneOf("rulesets.enforcement", *rs.Enforcement,
				RulesetEnforcementActive, RulesetEnforcementEvaluate, RulesetEnforcementDisabled); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if rs.Target != nil {
			if err := validateOneOf("rulesets.target", *rs.Target,
				RulesetTargetBranch, RulesetTargetTag); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		for i, ba := range rs.BypassActors {
			// Exactly one actor type must be specified
			count := 0
			if ba.Role != "" {
				count++
			}
			if ba.Team != "" {
				count++
			}
			if ba.App != "" {
				count++
			}
			if ba.OrgAdmin != nil {
				count++
			}
			if ba.CustomRole != "" {
				count++
			}
			if count == 0 {
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify one of: role, team, app, org-admin, or custom-role", r.Metadata.Name, rs.Name, i)
			}
			if count > 1 {
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify exactly one of: role, team, app, org-admin, or custom-role", r.Metadata.Name, rs.Name, i)
			}
			if ba.Role != "" {
				if err := validateOneOf("rulesets.bypass_actors.role", ba.Role, "admin", "write", "maintain"); err != nil {
					return fmt.Errorf("%s: %w", r.Metadata.Name, err)
				}
			}
			if err := validateOneOf("rulesets.bypass_actors.bypass_mode", ba.BypassMode,
				"always", "pull_request", "exempt"); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if rs.Conditions != nil && rs.Conditions.RefName != nil {
			if len(rs.Conditions.RefName.Include) == 0 {
				return fmt.Errorf("%s: rulesets[%s].conditions.ref_name.include must not be empty", r.Metadata.Name, rs.Name)
			}
		}
	}
	secretNames := make(map[string]bool)
	for _, s := range r.Spec.Secrets {
		if s.Name == "" {
			return fmt.Errorf("%s: secrets[].name is required", r.Metadata.Name)
		}
		if secretNames[s.Name] {
			return fmt.Errorf("%s: duplicate secret name %q", r.Metadata.Name, s.Name)
		}
		secretNames[s.Name] = true
	}
	variableNames := make(map[string]bool)
	for _, v := range r.Spec.Variables {
		if v.Name == "" {
			return fmt.Errorf("%s: variables[].name is required", r.Metadata.Name)
		}
		if variableNames[v.Name] {
			return fmt.Errorf("%s: duplicate variable name %q", r.Metadata.Name, v.Name)
		}
		variableNames[v.Name] = true
	}
	return nil
}

// Validate checks that the File has valid field values.
func (f *File) Validate() error {
	if f.Metadata.Name == "" {
		return fmt.Errorf("File metadata.name is required")
	}
	if f.Metadata.Owner == "" {
		return fmt.Errorf("File metadata.owner is required for %q", f.Metadata.Name)
	}
	if len(f.Spec.Files) == 0 {
		return fmt.Errorf("File %q: spec.files is required", f.Metadata.FullName())
	}
	if f.Spec.Via != "" {
		if err := validateOneOf("via", f.Spec.Via,
			ViaPush, ViaPullRequest); err != nil {
			return fmt.Errorf("File %q: %w", f.Metadata.FullName(), err)
		}
	}
	for i, fe := range f.Spec.Files {
		if fe.Path == "" {
			return fmt.Errorf("File %q: files[%d].path is required", f.Metadata.FullName(), i)
		}
		if fe.Content != "" && fe.Source != "" {
			return fmt.Errorf("File %q: files[%d] (%s) cannot have both content and source", f.Metadata.FullName(), i, fe.Path)
		}
		if fe.Reconcile != "" {
			if err := validateOneOf("reconcile", fe.Reconcile, ReconcilePatch, ReconcileMirror, ReconcileCreateOnly); err != nil {
				return fmt.Errorf("File %q: files[%d] (%s): %w", f.Metadata.FullName(), i, fe.Path, err)
			}
		}
	}
	return nil
}

// Validate checks that the FileSet has valid field values.
func (fs *FileSet) Validate() error {
	if fs.Metadata.Owner == "" {
		return fmt.Errorf("FileSet metadata.owner is required")
	}
	if len(fs.Spec.Repositories) == 0 {
		return fmt.Errorf("FileSet %q: spec.repositories is required", fs.Metadata.Owner)
	}
	if len(fs.Spec.Files) == 0 {
		return fmt.Errorf("FileSet %q: spec.files is required", fs.Metadata.Owner)
	}
	if fs.Spec.Via == "" {
		fs.Spec.Via = ViaPush
	}
	if err := validateOneOf("via", fs.Spec.Via,
		ViaPush, ViaPullRequest); err != nil {
		return fmt.Errorf("FileSet %q: %w", fs.Metadata.Owner, err)
	}
	for i, f := range fs.Spec.Files {
		if f.Path == "" {
			return fmt.Errorf("FileSet %q: files[%d].path is required", fs.Metadata.Owner, i)
		}
		if f.Content != "" && f.Source != "" {
			return fmt.Errorf("FileSet %q: files[%d] (%s) cannot have both content and source", fs.Metadata.Owner, i, f.Path)
		}
		if f.Reconcile != "" {
			if err := validateOneOf("reconcile", f.Reconcile, ReconcilePatch, ReconcileMirror, ReconcileCreateOnly); err != nil {
				return fmt.Errorf("FileSet %q: files[%d] (%s): %w", fs.Metadata.Owner, i, f.Path, err)
			}
		}
	}
	repoNames := make(map[string]bool)
	for _, r := range fs.Spec.Repositories {
		if r.Name == "" {
			return fmt.Errorf("FileSet %q: repositories[].name is required", fs.Metadata.Owner)
		}
		if repoNames[r.Name] {
			return fmt.Errorf("FileSet %q: duplicate repository %q", fs.Metadata.Owner, r.Name)
		}
		repoNames[r.Name] = true
	}
	return nil
}

// validateOneOf checks that value is one of the allowed values.
func validateOneOf(field, value string, allowed ...string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("invalid %s %q (must be one of: %s)", field, value, strings.Join(allowed, ", "))
}
