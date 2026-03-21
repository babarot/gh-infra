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
