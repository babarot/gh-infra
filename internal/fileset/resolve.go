package fileset

import "github.com/babarot/gh-infra/internal/manifest"

// ResolveFiles returns the effective files for a target, applying overrides.
func ResolveFiles(fs *manifest.FileSet, target manifest.FileSetRepository) []manifest.FileEntry {
	if len(target.Overrides) == 0 {
		return fs.Spec.Files
	}

	overrideMap := make(map[string]manifest.FileEntry)
	for _, o := range target.Overrides {
		overrideMap[o.Path] = o
	}

	result := make([]manifest.FileEntry, 0, len(fs.Spec.Files))
	for _, f := range fs.Spec.Files {
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
