package manifest

import "fmt"

const (
	// OnDrift values for FileSet drift handling.
	OnDriftWarn      = "warn"
	OnDriftOverwrite = "overwrite"
	OnDriftSkip      = "skip"

	// Via values for FileSet apply behavior.
	ViaPush        = "push"
	ViaPullRequest = "pull_request"

	// Deprecated: use Via* constants instead.
	CommitStrategyPush        = ViaPush
	CommitStrategyPullRequest = ViaPullRequest

	// Reconcile values for FileEntry reconcile behavior.
	ReconcilePatch      = "patch"       // default: add/update only
	ReconcileMirror     = "mirror"      // add/update + delete orphans
	ReconcileCreateOnly = "create_only" // create if missing, never update

	// Deprecated: use Reconcile* constants instead.
	SyncModePatch      = ReconcilePatch
	SyncModeMirror     = ReconcileMirror
	SyncModeCreateOnly = ReconcileCreateOnly
)

// File represents files to manage in a single repository.
// At parse time, File is expanded into a FileSet with one repository entry.
type File struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   FileMetadata `yaml:"metadata"`
	Spec       FileSpec     `yaml:"spec"`
}

type FileMetadata struct {
	Name  string `yaml:"name"  validate:"required"`
	Owner string `yaml:"owner" validate:"required"`
}

func (m FileMetadata) FullName() string {
	return m.Owner + "/" + m.Name
}

type FileSpec struct {
	Files         []FileEntry `yaml:"files" validate:"required"`
	CommitMessage string      `yaml:"commit_message,omitempty"`
	Via           string      `yaml:"via,omitempty" validate:"omitempty,oneof=push pull_request"`
	Branch        string      `yaml:"branch,omitempty"`
	PRTitle       string      `yaml:"pr_title,omitempty"`
	PRBody        string      `yaml:"pr_body,omitempty"`

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedCommitStrategy string   `yaml:"commit_strategy,omitempty" deprecated:"via:use \"via\" instead"`
	DeprecatedOnApply        string   `yaml:"on_apply,omitempty"        deprecated:"via:use \"via\" instead"`
	DeprecatedOnDrift        string   `yaml:"on_drift,omitempty"        deprecated:":and will be ignored"`
	DeprecationWarnings      []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (s *FileSpec) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileSpec
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*s = FileSpec(r)
	warnings, err := validateAndMigrateVia(s.DeprecatedCommitStrategy, s.DeprecatedOnApply, s)
	if err != nil {
		return err
	}
	s.DeprecationWarnings = warnings
	return nil
}

type FileEntry struct {
	Path      string            `yaml:"path"                validate:"required"`
	Content   string            `yaml:"content,omitempty" validate:"exclusive=source"`
	Source    string            `yaml:"source,omitempty"`
	Patches   []string          `yaml:"patches,omitempty"`
	Vars      map[string]string `yaml:"vars,omitempty"`
	Reconcile string            `yaml:"reconcile,omitempty" validate:"omitempty,oneof=patch mirror create_only"`
	DirScope  string            `yaml:"-"`

	// Deprecated fields (still parsed for backward compatibility)
	DeprecatedSyncMode  string   `yaml:"sync_mode,omitempty" deprecated:"reconcile:use \"reconcile\" instead"`
	DeprecatedOnDrift   string   `yaml:"on_drift,omitempty"  deprecated:":and will be ignored"`
	DeprecationWarnings []string `yaml:"-"`
}

// UnmarshalYAML handles migration from deprecated fields.
func (fe *FileEntry) UnmarshalYAML(unmarshal func(any) error) error {
	type raw FileEntry
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*fe = FileEntry(r)
	warnings, err := MigrateDeprecated(fe)
	if err != nil {
		if fe.Path != "" {
			return fmt.Errorf("%s: %w", fe.Path, err)
		}
		return err
	}
	// Prefix warnings with path for context
	for i, w := range warnings {
		if fe.Path != "" {
			warnings[i] = fe.Path + ": " + w
		}
	}
	fe.DeprecationWarnings = warnings
	return nil
}

// validateAndMigrateVia validates that commit_strategy and on_apply are not both set,
// then calls MigrateDeprecated. Shared by FileSpec and FileSetSpec.
func validateAndMigrateVia(commitStrategy, onApply string, target any) ([]string, error) {
	if commitStrategy != "" && onApply != "" {
		return nil, fmt.Errorf("cannot specify both \"commit_strategy\" and \"on_apply\"")
	}
	return MigrateDeprecated(target)
}
