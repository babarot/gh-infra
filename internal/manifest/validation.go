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
	// Branch protection: element tag validation
	for i, bp := range r.Spec.BranchProtection {
		if err := ValidateStruct(fmt.Sprintf("%s: spec.branch_protection[%d]", name, i), &bp); err != nil {
			return err
		}
	}
	// Rulesets: element tag validation + cross-field checks
	for i, rs := range r.Spec.Rulesets {
		if err := ValidateStruct(fmt.Sprintf("%s: spec.rulesets[%d]", name, i), &rs); err != nil {
			return err
		}
		for j, ba := range rs.BypassActors {
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
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify one of: role, team, app, org-admin, or custom-role", name, rs.Name, j)
			}
			if count > 1 {
				return fmt.Errorf("%s: rulesets[%s].bypass_actors[%d] must specify exactly one of: role, team, app, org-admin, or custom-role", name, rs.Name, j)
			}
			if err := ValidateStruct(fmt.Sprintf("%s: spec.rulesets[%d].bypass_actors[%d]", name, i, j), &ba); err != nil {
				return err
			}
		}
		if rs.Conditions != nil && rs.Conditions.RefName != nil {
			if len(rs.Conditions.RefName.Include) == 0 {
				return fmt.Errorf("%s: rulesets[%s].conditions.ref_name.include must not be empty", name, rs.Name)
			}
		}
	}
	// Secrets/Variables: element tag validation (unique handled by tags)
	for i, s := range r.Spec.Secrets {
		if err := ValidateStruct(fmt.Sprintf("%s: spec.secrets[%d]", name, i), &s); err != nil {
			return err
		}
	}
	for i, v := range r.Spec.Variables {
		if err := ValidateStruct(fmt.Sprintf("%s: spec.variables[%d]", name, i), &v); err != nil {
			return err
		}
	}
	// Actions: cross-field checks
	if a := r.Spec.Actions; a != nil {
		// GitHub API requires "enabled" in every PUT to /actions/permissions.
		// To avoid silently changing the enabled state, require it whenever
		// any other actions field is specified.
		if a.Enabled == nil {
			hasOtherField := a.AllowedActions != nil || a.WorkflowPermissions != nil ||
				a.CanApprovePullRequests != nil || a.SelectedActions != nil || a.ForkPRApproval != nil
			if hasOtherField {
				return fmt.Errorf("%s: actions.enabled is required when other actions fields are specified", name)
			}
		}
		if a.SelectedActions != nil {
			if a.AllowedActions == nil || *a.AllowedActions != "selected" {
				return fmt.Errorf("%s: actions.selected_actions can only be used when allowed_actions is \"selected\"", name)
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
