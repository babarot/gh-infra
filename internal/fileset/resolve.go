package fileset

import "github.com/babarot/gh-infra/internal/manifest"

// ResolveFiles returns the effective files for a target, applying overrides.
func ResolveFiles(fs *manifest.FileSet, target manifest.FileSetTarget) []manifest.FileEntry {
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
			result = append(result, override)
		} else {
			result = append(result, f)
		}
	}
	return result
}
