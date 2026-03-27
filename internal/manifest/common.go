package manifest

const (
	// APIVersion is the current API version for all manifest resources.
	APIVersion = "gh-infra/v1"

	// Kind constants for YAML document routing.
	KindRepository    = "Repository"
	KindRepositorySet = "RepositorySet"
	KindFile          = "File"
	KindFileSet       = "FileSet"
)

// Document represents a single YAML document with kind routing.
type Document struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

// ParseResult holds all parsed resources from a path.
type ParseResult struct {
	Repositories []*Repository
	FileSets     []*FileSet
	Warnings     []string // deprecation warnings collected during parse
}

// Ptr returns a pointer to the given value.
//
//go:fix inline
func Ptr[T any](v T) *T { return new(v) }
