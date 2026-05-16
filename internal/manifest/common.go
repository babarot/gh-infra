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

	// Document-level metadata for import --into.
	// These carry the same resources as Repositories/FileSets plus parse-origin info.
	RepositoryDocs []*RepositoryDocument
	FileDocs       []*FileDocument
}

// RepositoryDocument wraps a Repository with parse-origin metadata.
type RepositoryDocument struct {
	Resource          *Repository            // the parsed Repository
	SourcePath        string                 // file path that was parsed
	DocIndex          int                    // 0-based position in multi-doc YAML
	FromSet           bool                   // true if expanded from a RepositorySet
	SetEntryIndex     int                    // index within RepositorySet.Repositories (valid when FromSet)
	DefaultsSpec      *RepositorySetDefaults // RepositorySet defaults (valid when FromSet)
	OriginalEntrySpec *RepositorySpec        // pre-merge override spec (valid when FromSet)
}

// FileDocument wraps a FileSet with parse-origin metadata.
type FileDocument struct {
	Resource   *FileSet    // the parsed FileSet (or File expanded to FileSet)
	SourcePath string      // file path that was parsed
	DocIndex   int         // 0-based position in multi-doc YAML
	Files      []FileEntry // source-resolved files (OriginalSource set); use for import comparisons
}

// Merge appends all resources from other into r.
func (r *ParseResult) Merge(other *ParseResult) {
	r.Repositories = append(r.Repositories, other.Repositories...)
	r.FileSets = append(r.FileSets, other.FileSets...)
	r.RepositoryDocs = append(r.RepositoryDocs, other.RepositoryDocs...)
	r.FileDocs = append(r.FileDocs, other.FileDocs...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T { return new(v) }
