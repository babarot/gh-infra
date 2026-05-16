package manifest

import (
	"fmt"
)

// Validate checks that the Repository has valid field values.
func (r *Repository) Validate() error {
	// Recursive tag-based validation for metadata, spec, nested structs,
	// and slice-level checks (unique, etc.)
	if err := ValidateStruct("", r); err != nil {
		return err
	}
	name := r.Metadata.Name
	// Condition/ConditionalSpec must be specified together.
	if r.Condition != nil && r.ConditionalSpec == nil {
		return fmt.Errorf("%s: when: requires conditional_spec: to be present", name)
	}
	if r.ConditionalSpec != nil && r.Condition == nil {
		return fmt.Errorf("%s: conditional_spec: requires when: to be present", name)
	}
	if r.Condition != nil {
		if r.Condition.Visibility == "" {
			return fmt.Errorf("%s: when: must specify at least one condition (e.g. visibility: public)", name)
		}
		allowed := map[string]bool{
			VisibilityPublic:   true,
			VisibilityPrivate:  true,
			VisibilityInternal: true,
		}
		if !allowed[r.Condition.Visibility] {
			return fmt.Errorf("%s: when[visibility] must be one of: public, private, internal", name)
		}
	}
	if r.Reconcile != nil {
		if r.Reconcile.Labels != nil {
			if r.Spec.LabelSync != nil {
				return fmt.Errorf("%s: cannot specify both reconcile.labels and spec.label_sync (use reconcile.labels)", name)
			}
		}
	}
	if err := validateSpecElements(name, "spec", &r.Spec); err != nil {
		return err
	}
	if r.ConditionalSpec != nil {
		if err := validateSpecElements(name, "conditional_spec", r.ConditionalSpec); err != nil {
			return err
		}
	}
	return nil
}

// validateSpecElements validates element-level fields within a RepositorySpec.
// prefix is "spec" or "conditional_spec" for error message context.
func validateSpecElements(name, prefix string, spec *RepositorySpec) error {
	for i, bp := range spec.BranchProtection {
		if err := ValidateStruct(fmt.Sprintf("%s: %s.branch_protection[%d]", name, prefix, i), &bp); err != nil {
			return err
		}
	}
	for i, rs := range spec.Rulesets {
		if err := ValidateStruct(fmt.Sprintf("%s: %s.rulesets[%d]", name, prefix, i), &rs); err != nil {
			return err
		}
		for j, ba := range rs.BypassActors {
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
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify one of: role, team, app, org-admin, or custom-role", name, rs.Name, j)
			}
			if count > 1 {
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify exactly one of: role, team, app, org-admin, or custom-role", name, rs.Name, j)
			}
			if err := ValidateStruct(fmt.Sprintf("%s: %s.rulesets[%d].bypass_actors[%d]", name, prefix, i, j), &ba); err != nil {
				return err
			}
		}
		if rs.Conditions != nil && rs.Conditions.RefName != nil {
			if len(rs.Conditions.RefName.Include) == 0 {
				return fmt.Errorf("%s: rulesets[%s].conditions.ref_name.include must not be empty", name, rs.Name)
			}
		}
	}
	for i, s := range spec.Secrets {
		if err := ValidateStruct(fmt.Sprintf("%s: %s.secrets[%d]", name, prefix, i), &s); err != nil {
			return err
		}
	}
	for i, v := range spec.Variables {
		if err := ValidateStruct(fmt.Sprintf("%s: %s.variables[%d]", name, prefix, i), &v); err != nil {
			return err
		}
	}
	for i, ms := range spec.Milestones {
		if err := ValidateStruct(fmt.Sprintf("%s: %s.milestones[%d]", name, prefix, i), &ms); err != nil {
			return err
		}
	}
	if a := spec.Actions; a != nil {
		if a.ForkPRApproval != nil && spec.Visibility != nil && *spec.Visibility == VisibilityPrivate {
			return fmt.Errorf("%s: %s.actions.fork_pr_approval is not supported for private repositories (remove actions.fork_pr_approval or set visibility to public/internal)", name, prefix)
		}
		if a.Enabled == nil {
			hasOtherField := a.AllowedActions != nil || a.SHAPinningRequired != nil || a.WorkflowPermissions != nil ||
				a.CanApprovePullRequests != nil || a.SelectedActions != nil || a.ForkPRApproval != nil
			if hasOtherField {
				return fmt.Errorf("%s: %s.actions.enabled is required when other actions fields are specified (add \"enabled: true\" or \"enabled: false\")", name, prefix)
			}
		}
		if a.SelectedActions != nil {
			if a.AllowedActions == nil || *a.AllowedActions != "selected" {
				return fmt.Errorf("%s: %s.actions.selected_actions can only be used when allowed_actions is \"selected\" (add \"allowed_actions: selected\")", name, prefix)
			}
		}
	}
	return nil
}

// Validate checks that the File has valid field values.
func (f *File) Validate() error {
	// Recursive tag-based validation for metadata and spec
	if err := ValidateStruct("", f); err != nil {
		return err
	}
	fullName := f.Metadata.FullName()
	// File entries: element tag validation (exclusive handled by tags)
	for i, fe := range f.Spec.Files {
		if err := ValidateStruct(fmt.Sprintf("File %q: spec.files[%d]", fullName, i), &fe); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks that the FileSet has valid field values.
func (fs *FileSet) Validate() error {
	// Recursive tag-based validation for metadata and spec
	// (unique on repositories handled by tags)
	if err := ValidateStruct("", fs); err != nil {
		return err
	}
	owner := fs.Metadata.Owner
	// Default via to push
	if fs.Spec.Via == "" {
		fs.Spec.Via = ViaPush
	}
	// File entries: element tag validation (exclusive handled by tags)
	for i, f := range fs.Spec.Files {
		if err := ValidateStruct(fmt.Sprintf("FileSet %q: spec.files[%d]", owner, i), &f); err != nil {
			return err
		}
	}
	// Repositories: element tag validation
	for i, r := range fs.Spec.Repositories {
		if err := ValidateStruct(fmt.Sprintf("FileSet %q: spec.repositories[%d]", owner, i), &r); err != nil {
			return err
		}
	}
	return nil
}
