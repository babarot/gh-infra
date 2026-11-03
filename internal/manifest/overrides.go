package manifest

import (
	"fmt"
	"slices"
)

// FilesFor returns the files that target receives: spec.files with target's
// overrides applied. Both are as parsed, with sources resolved: a directory
// source is a file per entry, each with DirScope set to the directory.
//
// An override applies to an entry of spec.files as the manifest lists it:
//
//   - An override with a directory source replaces the files of the
//     directory entry with the same path. Fields it leaves out (vars,
//     patches, reconcile, executable, description) come from that entry.
//   - An override without content or source whose path is a directory
//     entry sets the fields it has on every file of the directory.
//   - An override whose path is a file, either a file entry or a file of a
//     directory entry, replaces that file. Fields it leaves out, including
//     content and source, come from that file.
//
// Directory overrides are applied before file overrides. An override that
// matches nothing, one that gives a directory entry a single content, and
// one that gives a file entry a directory are errors, as are two files at
// the same path after the overrides.
func (fs *FileSet) FilesFor(target FileSetRepository) ([]FileEntry, error) {
	if len(target.Overrides) == 0 {
		return fs.Spec.Files, nil
	}
	isDir := make(map[string]bool) // directory entries of spec.files
	isFile := make(map[string]bool)
	for _, f := range fs.Spec.Files {
		if f.DirScope != "" {
			isDir[f.DirScope] = true
		}
		isFile[f.Path] = true
	}

	replace := make(map[string][]FileEntry) // directory -> its files from the override
	fields := make(map[string]FileEntry)    // directory -> fields-only override
	var perFile []FileEntry
	for _, o := range target.Overrides {
		switch {
		case o.DirScope != "":
			if !isDir[o.DirScope] {
				if isFile[o.DirScope] {
					return nil, fmt.Errorf("override %s for %s has a directory source, but %s is a file in files", o.DirScope, target.Name, o.DirScope)
				}
				return nil, fmt.Errorf("override %s for %s matches no entry in files", o.DirScope, target.Name)
			}
			replace[o.DirScope] = append(replace[o.DirScope], o)
		case isDir[o.Path]:
			if o.Content != "" || o.Source != "" {
				return nil, fmt.Errorf("override %s for %s gives a directory entry a single file; use a directory source ending in / or leave out content and source", o.Path, target.Name)
			}
			fields[o.Path] = o
		case isFile[o.Path]:
			perFile = append(perFile, o)
		default:
			return nil, fmt.Errorf("override %s for %s matches no entry in files", o.Path, target.Name)
		}
	}

	var result []FileEntry
	replaced := make(map[string]bool)
	for _, f := range fs.Spec.Files {
		if files, ok := replace[f.DirScope]; ok && f.DirScope != "" {
			if !replaced[f.DirScope] {
				replaced[f.DirScope] = true
				for _, o := range files {
					result = append(result, inheritFields(o, f))
				}
			}
			continue
		}
		if o, ok := fields[f.DirScope]; ok && f.DirScope != "" {
			merged := inheritFields(o, f)
			// The override has no content or source; each file keeps its own.
			merged.Path, merged.Content = f.Path, f.Content
			merged.OriginalSource, merged.DirScope = f.OriginalSource, f.DirScope
			f = merged
		}
		result = append(result, f)
	}

	for _, o := range perFile {
		i := slices.IndexFunc(result, func(f FileEntry) bool { return f.Path == o.Path })
		if i < 0 {
			// The file was in a directory that another override replaced.
			return nil, fmt.Errorf("override %s for %s matches no file: its directory is replaced by another override", o.Path, target.Name)
		}
		result[i] = inheritFile(o, result[i])
	}

	if err := checkDuplicatePaths(result); err != nil {
		return nil, fmt.Errorf("files for %s after overrides: %w", target.Name, err)
	}
	return result, nil
}

// inheritFields returns o with the fields it leaves out taken from base:
// those that describe how a file is delivered, not its content.
func inheritFields(o, base FileEntry) FileEntry {
	if o.Vars == nil && base.Vars != nil {
		o.Vars = base.Vars
	}
	if o.Reconcile == "" {
		o.Reconcile = base.Reconcile
	}
	if o.Patches == nil && base.Patches != nil {
		o.Patches = base.Patches
	}
	if o.Executable == nil && base.Executable != nil {
		o.Executable = base.Executable
	}
	if o.Description == "" {
		o.Description = base.Description
	}
	return o
}

// inheritFile returns the file override o with everything it leaves out,
// its content and source too, taken from the file base.
func inheritFile(o, base FileEntry) FileEntry {
	o = inheritFields(o, base)
	if o.DirScope == "" {
		o.DirScope = base.DirScope
	}
	if o.Content == "" {
		o.Content = base.Content
	}
	if o.Source == "" {
		o.Source = base.Source
	}
	if o.OriginalSource == "" {
		o.OriginalSource = base.OriginalSource
	}
	return o
}

// checkOverrides checks the overrides of each repository as written, before
// their sources are resolved: two overrides with the same path would both
// apply, in no defined order.
func checkOverrides(repos []FileSetRepository) error {
	for _, r := range repos {
		seen := make(map[string]bool, len(r.Overrides))
		for _, o := range r.Overrides {
			if seen[o.Path] {
				return fmt.Errorf("override %s for %s is listed twice", o.Path, r.Name)
			}
			seen[o.Path] = true
		}
	}
	return nil
}
