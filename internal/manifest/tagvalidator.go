package manifest

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// ValidateStruct recursively inspects validate tags on struct fields and
// returns the first validation error found. It automatically walks into
// nested struct and *struct fields, building up the YAML field path
// (e.g. "metadata.name", "spec.merge_strategy.squash_merge_commit_title").
// Slice fields are NOT recursed into — the caller handles element iteration.
//
// prefix is an optional contextual label prepended with ": " to error messages
// (e.g. repo name, "my-repo: rulesets[0]"). It is separate from the structural path.
//
// Supported tag syntax:
//
//	validate:"required"              — string non-empty or slice non-empty
//	validate:"oneof=a b c"           — value must be one of the listed values
//	validate:"omitempty,oneof=a b c" — skip if zero/nil, otherwise check oneof
//	validate:"unique=name"            — slice elements must have unique values for the named yaml field
//	validate:"exclusive=source"       — this field and the named yaml field cannot both be non-empty
func ValidateStruct(prefix string, v any) error {
	return validateStructRecursive(prefix, "", v)
}

// validateStructRecursive walks the struct fields, accumulating structPath
// (dot-separated YAML path) for error messages.
func validateStructRecursive(prefix, structPath string, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	rt := rv.Type()

	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)

		// Skip fields not part of YAML schema
		if field.Tag.Get("yaml") == "-" {
			continue
		}

		fieldPath := joinDot(structPath, yamlFieldName(field))

		// Process validate tag
		if tag := field.Tag.Get("validate"); tag != "" && tag != "-" {
			if err := validateField(fieldPath, fv, tag, rv); err != nil {
				return wrapPrefix(prefix, err)
			}
		}

		// Recurse into nested struct / *struct (not slices)
		switch fv.Kind() {
		case reflect.Struct:
			if err := validateStructRecursive(prefix, fieldPath, fv.Addr().Interface()); err != nil {
				return err
			}
		case reflect.Pointer:
			if !fv.IsNil() && fv.Elem().Kind() == reflect.Struct {
				if err := validateStructRecursive(prefix, fieldPath, fv.Interface()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateField checks a single field value against its validate tag.
// parentStruct is needed for exclusive checks (to look up sibling fields).
func validateField(fieldPath string, fv reflect.Value, tag string, parentStruct ...reflect.Value) error {
	parts := strings.Split(tag, ",")
	omitempty := slices.Contains(parts, "omitempty")

	for _, p := range parts {
		switch {
		case p == "required":
			if err := checkRequired(fieldPath, fv); err != nil {
				return err
			}
		case strings.HasPrefix(p, "oneof="):
			if omitempty && isZero(fv) {
				continue
			}
			allowed := strings.Fields(strings.TrimPrefix(p, "oneof="))
			if err := checkOneOf(fieldPath, fv, allowed); err != nil {
				return err
			}
		case strings.HasPrefix(p, "unique="):
			keyField := strings.TrimPrefix(p, "unique=")
			if err := checkUnique(fieldPath, fv, keyField); err != nil {
				return err
			}
		case strings.HasPrefix(p, "exclusive="):
			otherGoName := strings.TrimPrefix(p, "exclusive=")
			if len(parentStruct) > 0 {
				if err := checkExclusive(fieldPath, fv, parentStruct[0], otherGoName); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// MigrateDeprecated processes deprecated tags on the direct fields of a struct.
// It migrates values to their target fields and returns collected warnings.
//
// Tag syntax (target is a yaml field name):
//
//	deprecated:"via:message"   — migrate value to the field with yaml:"via", warn
//	deprecated:":message"      — no migration target, just warn and clear
//
// Returns an error if a deprecated field and its migration target are both set.
func MigrateDeprecated(v any) ([]string, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, nil
	}
	rt := rv.Type()

	var warnings []string

	for i := range rt.NumField() {
		field := rt.Field(i)
		tag := field.Tag.Get("deprecated")
		if tag == "" {
			continue
		}

		fv := rv.Field(i)
		// Only process non-empty string fields
		if fv.Kind() != reflect.String || fv.String() == "" {
			continue
		}

		targetYAML, msg := parseDeprecatedTag(tag)

		if targetYAML != "" {
			// Resolve yaml name to Go field
			sf, ok := goFieldByYAMLName(rt, targetYAML)
			if !ok {
				return nil, fmt.Errorf("deprecated tag references unknown yaml field %q", targetYAML)
			}
			targetField := rv.FieldByName(sf.Name)
			if !targetField.CanSet() {
				return nil, fmt.Errorf("deprecated tag references unsettable field %q", targetYAML)
			}
			// Check conflict: both deprecated and target are set
			if targetField.Kind() == reflect.String && targetField.String() != "" {
				return nil, fmt.Errorf("cannot specify both %q and %q", yamlFieldName(field), targetYAML)
			}
			// Migrate value
			targetField.Set(fv)
		}

		warnings = append(warnings, fmt.Sprintf("%q is deprecated, %s", yamlFieldName(field), msg))
		// Clear the deprecated field
		fv.SetString("")
	}

	return warnings, nil
}

// parseDeprecatedTag splits "TargetField:message" into target and message.
func parseDeprecatedTag(tag string) (target, msg string) {
	target, msg, found := strings.Cut(tag, ":")
	if !found {
		return "", tag
	}
	return target, msg
}

// joinDot joins two path segments with ".". Returns name if prefix is empty.
func joinDot(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// yamlFieldName extracts the YAML field name from struct tags, falling back
// to the Go field name.
func yamlFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("yaml")
	if tag == "" || tag == "-" {
		return f.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	return name
}

// goFieldByYAMLName looks up a struct field by its yaml tag name.
// Returns the field and true if found, or zero value and false if not.
func goFieldByYAMLName(rt reflect.Type, yamlName string) (reflect.StructField, bool) {
	for f := range rt.Fields() {
		if yamlFieldName(f) == yamlName {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// checkRequired verifies a field is non-empty.
func checkRequired(name string, fv reflect.Value) error {
	switch fv.Kind() {
	case reflect.String:
		if fv.String() == "" {
			return fmt.Errorf("%s is required", name)
		}
	case reflect.Slice:
		if fv.IsNil() || fv.Len() == 0 {
			return fmt.Errorf("%s is required", name)
		}
	case reflect.Pointer:
		if fv.IsNil() {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

// checkOneOf verifies the field value is one of the allowed values.
func checkOneOf(name string, fv reflect.Value, allowed []string) error {
	var val string
	switch fv.Kind() {
	case reflect.String:
		val = fv.String()
	case reflect.Pointer:
		if fv.IsNil() {
			return nil
		}
		val = fv.Elem().String()
	default:
		return nil
	}
	if slices.Contains(allowed, val) {
		return nil
	}
	return fmt.Errorf("invalid %s %q (must be one of: %s)", name, val, strings.Join(allowed, ", "))
}

// checkUnique verifies that elements of a slice have unique values for the given field.
// keyYAML is the yaml field name (e.g. "name", "pattern").
// e.g. validate:"unique=name" on a []Secret checks that no two secrets share the same name.
func checkUnique(fieldPath string, fv reflect.Value, keyYAML string) error {
	if fv.Kind() != reflect.Slice || fv.IsNil() || fv.Len() == 0 {
		return nil
	}
	// Resolve yaml name to Go field name from the element type
	elemType := fv.Type().Elem()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}
	sf, ok := goFieldByYAMLName(elemType, keyYAML)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for i := range fv.Len() {
		elem := fv.Index(i)
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			continue
		}
		kf := elem.FieldByName(sf.Name)
		if !kf.IsValid() || kf.Kind() != reflect.String {
			continue
		}
		val := kf.String()
		if val == "" {
			continue
		}
		if seen[val] {
			return fmt.Errorf("duplicate %s %q in %s", keyYAML, val, fieldPath)
		}
		seen[val] = true
	}
	return nil
}

// checkExclusive verifies that this field and another named field are not both non-empty.
// otherYAML is the yaml field name of the other field (e.g. "source").
// e.g. validate:"exclusive=source" on content checks that content and source aren't both set.
func checkExclusive(fieldPath string, fv reflect.Value, parent reflect.Value, otherYAML string) error {
	if isZero(fv) {
		return nil
	}
	// Resolve yaml name to Go field name
	if parent.Kind() != reflect.Struct {
		return nil
	}
	sf, ok := goFieldByYAMLName(parent.Type(), otherYAML)
	if !ok {
		return nil
	}
	other := parent.FieldByName(sf.Name)
	if !other.IsValid() || isZero(other) {
		return nil
	}
	// Extract the field name from the path (last segment)
	thisYAML := fieldPath
	if idx := strings.LastIndex(fieldPath, "."); idx >= 0 {
		thisYAML = fieldPath[idx+1:]
	}
	return fmt.Errorf("cannot have both %s and %s", thisYAML, otherYAML)
}

// isZero checks if a value is the zero value for its type.
func isZero(fv reflect.Value) bool {
	switch fv.Kind() {
	case reflect.Pointer, reflect.Interface:
		return fv.IsNil()
	case reflect.String:
		return fv.String() == ""
	case reflect.Slice:
		return fv.IsNil() || fv.Len() == 0
	default:
		return false
	}
}

// wrapPrefix prepends a context prefix to an error.
func wrapPrefix(prefix string, err error) error {
	if prefix == "" {
		return err
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
