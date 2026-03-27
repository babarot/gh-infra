package fileset

import "github.com/babarot/gh-infra/internal/manifest"

// ResolveFiles returns the effective files for a target, applying overrides.
func ResolveFiles(fs *manifest.FileSetDocument, target manifest.FileSetRepository) []manifest.ResolvedFile {
	return ResolveFilesForTarget(fs, target, -1)
}

// ResolveFilesForTarget returns the effective files for a target, applying overrides
// and preserving manifest origin metadata for import write-back.
func ResolveFilesForTarget(fs *manifest.FileSetDocument, target manifest.FileSetRepository, repoIndex int) []manifest.ResolvedFile {
	if len(target.Overrides) == 0 {
		return fs.ResolvedFiles
	}

	overrideMap := make(map[string]manifest.ResolvedFile)
	for i, o := range target.Overrides {
		overrideMap[o.Path] = manifest.ResolvedFile{
			FileEntry: o,
			Origin: manifest.FileOrigin{
				Kind:      manifest.FileOriginRepositoryOverride,
				RepoIndex: repoIndex,
				FileIndex: i,
			},
		}
	}

	result := make([]manifest.ResolvedFile, 0, len(fs.ResolvedFiles))
	for _, f := range fs.ResolvedFiles {
		if override, ok := overrideMap[f.Path]; ok {
			// Inherit metadata from original if override doesn't define its own
			if override.Vars == nil && f.Vars != nil {
				override.Vars = f.Vars
			}
			if override.DirScope == "" {
				override.DirScope = f.DirScope
			}
			if override.Reconcile == "" {
				override.Reconcile = f.Reconcile
			}
			if override.Patches == nil && f.Patches != nil {
				override.Patches = f.Patches
			}
			result = append(result, override)
		} else {
			result = append(result, f)
		}
	}
	return result
}
