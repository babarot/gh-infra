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
	if f := r.Spec.Features; f != nil {
		if f.SquashMergeCommitTitle != nil {
			if err := validateOneOf("squash_merge_commit_title", *f.SquashMergeCommitTitle,
				SquashMergeCommitTitlePRTitle, SquashMergeCommitTitleCommitOrPRTitle); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if f.SquashMergeCommitMessage != nil {
			if err := validateOneOf("squash_merge_commit_message", *f.SquashMergeCommitMessage,
				SquashMergeCommitMessageCommitMessages, SquashMergeCommitMessagePRBody, SquashMergeCommitMessageBlank); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if f.MergeCommitTitle != nil {
			if err := validateOneOf("merge_commit_title", *f.MergeCommitTitle,
				MergeCommitTitleMergeMessage, MergeCommitTitlePRTitle); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
		if f.MergeCommitMessage != nil {
			if err := validateOneOf("merge_commit_message", *f.MergeCommitMessage,
				MergeCommitMessagePRTitle, MergeCommitMessagePRBody, MergeCommitMessageBlank); err != nil {
				return fmt.Errorf("%s: %w", r.Metadata.Name, err)
			}
		}
	}
	for _, bp := range r.Spec.BranchProtection {
		if bp.Pattern == "" {
			return fmt.Errorf("%s: branch_protection.pattern is required", r.Metadata.Name)
		}
	}
	for _, s := range r.Spec.Secrets {
		if s.Name == "" {
			return fmt.Errorf("%s: secrets[].name is required", r.Metadata.Name)
		}
	}
	for _, v := range r.Spec.Variables {
		if v.Name == "" {
			return fmt.Errorf("%s: variables[].name is required", r.Metadata.Name)
		}
	}
	return nil
}

// Validate checks that the FileSet has valid field values.
func (fs *FileSet) Validate() error {
	if fs.Metadata.Name == "" {
		return fmt.Errorf("FileSet metadata.name is required")
	}
	if len(fs.Spec.Targets) == 0 {
		return fmt.Errorf("FileSet %q: spec.targets is required", fs.Metadata.Name)
	}
	if len(fs.Spec.Files) == 0 {
		return fmt.Errorf("FileSet %q: spec.files is required", fs.Metadata.Name)
	}
	if fs.Spec.OnDrift == "" {
		fs.Spec.OnDrift = OnDriftWarn
	}
	if err := validateOneOf("on_drift", fs.Spec.OnDrift,
		OnDriftWarn, OnDriftOverwrite, OnDriftSkip); err != nil {
		return fmt.Errorf("FileSet %q: %w", fs.Metadata.Name, err)
	}
	if fs.Spec.Strategy == "" {
		fs.Spec.Strategy = StrategyDirect
	}
	if err := validateOneOf("strategy", fs.Spec.Strategy,
		StrategyDirect, StrategyPullRequest); err != nil {
		return fmt.Errorf("FileSet %q: %w", fs.Metadata.Name, err)
	}
	for i, f := range fs.Spec.Files {
		if f.Path == "" {
			return fmt.Errorf("FileSet %q: files[%d].path is required", fs.Metadata.Name, i)
		}
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
