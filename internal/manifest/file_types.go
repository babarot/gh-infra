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
	ReconcileAdditive      = "additive"      // default: add/update only
	ReconcileAuthoritative = "authoritative" // add/update + delete orphans in scope
	ReconcileCreateOnly    = "create_only"   // create if missing, never update

	// Deprecated: use ReconcileAdditive.
	ReconcilePatch = ReconcileAdditive
	// Deprecated: use ReconcileAuthoritative.
	ReconcileMirror = ReconcileAuthoritative

	// Deprecated: use Reconcile* constants instead.
	SyncModePatch      = ReconcileAdditive
	SyncModeMirror     = ReconcileAuthoritative
	SyncModeCreateOnly = ReconcileCreateOnly
)

const (
	legacyReconcilePatch  = "patch"
	legacyReconcileMirror = "mirror"
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
	Path           string            `yaml:"path"                validate:"required"`
	Content        string            `yaml:"content,omitempty" validate:"exclusive=source"`
	Source         string            `yaml:"source,omitempty"`
	Patches        []string          `yaml:"patches,omitempty"`
	Vars           map[string]string `yaml:"vars,omitempty"`
	Reconcile      string            `yaml:"reconcile,omitempty" validate:"omitempty,oneof=additive authoritative create_only"`
	DirScope       string            `yaml:"-"`
	OriginalSource string            `yaml:"-"` // local file path set during source resolution (import --into)

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
	reconcileWarnings, err := normalizeFileReconcile(fe)
	if err != nil {
		if fe.Path != "" {
			return fmt.Errorf("%s: %w", fe.Path, err)
		}
		return err
	}
	warnings = append(warnings, reconcileWarnings...)
	// Prefix warnings with path for context
	for i, w := range warnings {
		if fe.Path != "" {
			warnings[i] = fe.Path + ": " + w
		}
	}
	fe.DeprecationWarnings = warnings
	return nil
}

func normalizeFileReconcile(fe *FileEntry) ([]string, error) {
	switch fe.Reconcile {
	case "", ReconcileAdditive, ReconcileAuthoritative, ReconcileCreateOnly:
		return nil, nil
	case legacyReconcilePatch:
		fe.Reconcile = ReconcileAdditive
		return []string{`"reconcile" value "patch" is deprecated, use "additive" instead`}, nil
	case legacyReconcileMirror:
		fe.Reconcile = ReconcileAuthoritative
		return []string{`"reconcile" value "mirror" is deprecated, use "authoritative" instead`}, nil
	default:
		return nil, nil
	}
}

// validateAndMigrateVia validates that commit_strategy and on_apply are not both set,
// then calls MigrateDeprecated. Shared by FileSpec and FileSetSpec.
func validateAndMigrateVia(commitStrategy, onApply string, target any) ([]string, error) {
	if commitStrategy != "" && onApply != "" {
		return nil, fmt.Errorf("cannot specify both \"commit_strategy\" and \"on_apply\"")
	}
	return MigrateDeprecated(target)
}
