package manifest

// UnmarshalYAML allows FileSetTarget to be either a string or a struct.
func (t *FileSetTarget) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first
	var s string
	if err := unmarshal(&s); err == nil {
		t.Name = s
		return nil
	}

	// Try struct
	type raw FileSetTarget
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*t = FileSetTarget(r)
	return nil
}

// ResolveFiles returns the effective files for a target, applying overrides.
func ResolveFiles(fs *FileSet, target FileSetTarget) []FileEntry {
	if len(target.Overrides) == 0 {
		return fs.Spec.Files
	}

	overrideMap := make(map[string]FileEntry)
	for _, o := range target.Overrides {
		overrideMap[o.Path] = o
	}

	result := make([]FileEntry, 0, len(fs.Spec.Files))
	for _, f := range fs.Spec.Files {
		if override, ok := overrideMap[f.Path]; ok {
			result = append(result, override)
		} else {
			result = append(result, f)
		}
	}
	return result
}
