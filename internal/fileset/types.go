package fileset

import "github.com/babarot/gh-infra/internal/manifest"

// FileOriginKind describes where a resolved file entry came from.
type FileOriginKind string

const (
	FileOriginSpecFiles          FileOriginKind = "spec.files"
	FileOriginRepositoryOverride FileOriginKind = "spec.repositories.overrides"
)

// FileOrigin tracks which manifest node produced a resolved file entry.
// RepoIndex is only used for repository override entries.
type FileOrigin struct {
	Kind      FileOriginKind
	RepoIndex int
	FileIndex int
}

// ResolvedFile is the execution-time representation of a FileEntry after source
// expansion and override resolution. It carries provenance and local-source data
// needed by the import flow.
type ResolvedFile struct {
	manifest.FileEntry
	Origin FileOrigin
}
