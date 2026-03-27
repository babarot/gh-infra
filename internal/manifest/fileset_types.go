package manifest

// FileSet represents a set of files to distribute to target repositories.
type FileSet struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   FileSetMetadata `yaml:"metadata"`
	Spec       FileSetSpec     `yaml:"spec"`
}

type FileSetMetadata struct {
	Owner string `yaml:"owner" validate:"required"`
}

type FileSetSpec struct {
	Repositories  []FileSetRepository `yaml:"repositories" validate:"required,unique=Name"`
	Files         []FileEntry         `yaml:"files"        validate:"required"`
	CommitMessage string              `yaml:"commit_message,omitempty"`
	Via           string              `yaml:"via,omitempty" validate:"omitempty,oneof=push pull_request"`
	Branch        string              `yaml:"branch,omitempty"`
	PRTitle       string              `yaml:"pr_title,omitempty"`
	PRBody        string              `yaml:"pr_body,omitempty"`

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedCommitStrategy string   `yaml:"commit_strategy,omitempty" deprecated:"via:use \"via\" instead"`
	DeprecatedOnApply        string   `yaml:"on_apply,omitempty"        deprecated:"via:use \"via\" instead"`
	DeprecatedOnDrift        string   `yaml:"on_drift,omitempty"        deprecated:":and will be ignored"`
	DeprecationWarnings      []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (s *FileSetSpec) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileSetSpec
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*s = FileSetSpec(r)
	warnings, err := validateAndMigrateVia(s.DeprecatedCommitStrategy, s.DeprecatedOnApply, s)
	if err != nil {
		return err
	}
	s.DeprecationWarnings = warnings
	return nil
}

// FileSetRepository can be a simple string "repo" or a struct with overrides.
type FileSetRepository struct {
	Name      string      `yaml:"name" validate:"required"`
	Overrides []FileEntry `yaml:"overrides,omitempty"`
}

// UnmarshalYAML allows FileSetRepository to be either a string or a struct.
func (t *FileSetRepository) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		t.Name = s
		return nil
	}
	type raw FileSetRepository
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*t = FileSetRepository(r)
	return nil
}

// RepoFullName returns the full "owner/repo" name for a repository entry.
func (fs *FileSet) RepoFullName(repoName string) string {
	return fs.Metadata.Owner + "/" + repoName
}
