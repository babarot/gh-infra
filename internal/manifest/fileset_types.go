package manifest

import (
	"sort"
	"strings"
)

// FileSet represents a set of files to distribute to target repositories.
type FileSet struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   FileSetMetadata `yaml:"metadata"`
	Spec       FileSetSpec     `yaml:"spec"`
}

type FileSetMetadata struct {
	Name  string `yaml:"name,omitempty"`
	Owner string `yaml:"owner" validate:"required"`
}

// Identity returns "owner/name" when metadata.name is set (kind: File sets it
// to the target repository), and otherwise "owner/" plus the sorted repository
// names joined by "+". It names the FileSet in plan output and derives the
// default commit message, PR branch, and PR body. It is not unique: unnamed
// FileSets listing the same repositories, and kind: File manifests for the same
// repository, share it.
func (fs *FileSet) Identity() string {
	if fs.Metadata.Name != "" {
		return fs.Metadata.Owner + "/" + fs.Metadata.Name
	}
	names := make([]string, len(fs.Spec.Repositories))
	for i, r := range fs.Spec.Repositories {
		names[i] = r.Name
	}
	sort.Strings(names)
	return fs.Metadata.Owner + "/" + strings.Join(names, "+")
}

// PRBranch returns the branch that via: pull_request commits to: spec.branch,
// or DefaultPRBranch of the Identity when it is unset.
func (fs *FileSet) PRBranch() string {
	if fs.Spec.Branch != "" {
		return fs.Spec.Branch
	}
	return DefaultPRBranch(fs.Identity())
}

// DefaultPRBranch returns the pull_request branch for a FileSet identity
// without spec.branch, e.g. "gh-infra/sync-my-org-ci" for "my-org/ci".
func DefaultPRBranch(identity string) string {
	return "gh-infra/sync-" + sanitizeBranchName(identity)
}

// sanitizeBranchName converts an identity string into a valid Git branch name component.
func sanitizeBranchName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "..", "")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '~', '^', ':', '?', '*', '[', '\\':
			return -1
		}
		return r
	}, s)
	s = strings.Trim(s, "-.")
	return s
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
func (fsr *FileSetRepository) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		fsr.Name = s
		return nil
	}
	type raw FileSetRepository
	var r raw
	if err := unmarshal(&r); err != nil {
		return err
	}
	*fsr = FileSetRepository(r)
	return nil
}

// RepoFullName returns the full "owner/repo" name for a repository entry.
func (fs *FileSet) RepoFullName(repoName string) string {
	return fs.Metadata.Owner + "/" + repoName
}
