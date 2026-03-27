package importer

import (
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/yamlpatch"
)

// ImportWriteMode describes how the imported content will be written locally.
type ImportWriteMode string

const (
	ImportWriteSource ImportWriteMode = "source" // overwrite a source file on disk
	ImportWriteInline ImportWriteMode = "inline" // update inline content block in manifest YAML
	ImportSkip        ImportWriteMode = "skip"   // skipped (create_only, not on GitHub, etc.)
)

// ImportChange represents a planned file change for the import direction (GitHub → local).
type ImportChange struct {
	Target       string // owner/repo
	Path         string
	Type         fileset.ChangeType
	Current      string // current local content
	Desired      string // content from GitHub (what will be written)
	WriteMode    ImportWriteMode
	LocalTarget  string   // write-back destination path
	ManifestPath string   // path to the manifest YAML file (for inline edits)
	DocIndex     int      // document index within the manifest file
	YAMLPath     string   // YAML path to the content field for inline edits
	Reason       string   // reason for skip
	Warnings     []string // e.g. patches, templates
}

// FileFetcher fetches a file's current state from a repository.
type FileFetcher func(repo, path string) (*fileset.FileState, error)

// PlanImport computes import changes for all FileSets.
// filterRepo must be "owner/repo" format; required if a FileSet targets multiple repos.
func PlanImport(fetchFile FileFetcher, fileSets []*manifest.FileSetDocument, filterRepo string) ([]ImportChange, error) {
	var changes []ImportChange

	for _, fsDoc := range fileSets {
		fs := fsDoc.Resource
		repos := fs.Spec.Repositories
		if filterRepo != "" {
			var filtered []manifest.FileSetRepository
			for _, r := range repos {
				if fs.RepoFullName(r.Name) == filterRepo {
					filtered = append(filtered, r)
				}
			}
			repos = filtered
		}

		if len(repos) == 0 {
			continue
		}

		if len(repos) > 1 {
			return nil, fmt.Errorf(
				"FileSet %q targets multiple repositories; use --repo to specify which one to pull from",
				fs.Metadata.Owner,
			)
		}

		target := repos[0]
		repoIndex := 0
		for i, r := range fs.Spec.Repositories {
			if r.Name == target.Name {
				repoIndex = i
				break
			}
		}
		fullName := fs.RepoFullName(target.Name)
		files := ResolveFilesForImport(fsDoc, target, repoIndex)

		for _, file := range files {
			change := planImportEntry(fetchFile, file, fullName, fsDoc)
			changes = append(changes, change)
		}
	}

	return changes, nil
}

func planImportEntry(fetchFile FileFetcher, file fileset.ResolvedFile, repo string, fs *manifest.FileSetDocument) ImportChange {
	change := ImportChange{
		Target:       repo,
		Path:         file.Path,
		ManifestPath: fs.SourcePath,
		DocIndex:     fs.DocIndex,
	}

	// Determine write target
	if file.OriginalSource != "" {
		change.WriteMode = ImportWriteSource
		change.LocalTarget = file.OriginalSource
	} else if file.Source == "" {
		// Inline content (no source field was set)
		yamlPath, ok := importYAMLPath(file.Origin)
		if !ok {
			change.WriteMode = ImportSkip
			change.Type = fileset.FileNoOp
			change.Reason = "inline source mapping unavailable"
			return change
		}
		change.WriteMode = ImportWriteInline
		change.LocalTarget = fs.SourcePath + " (inline)"
		change.YAMLPath = yamlPath
	} else {
		// github:// or other remote source — can't write back locally
		change.WriteMode = ImportSkip
		change.Type = fileset.FileNoOp
		change.Reason = "remote source"
		return change
	}

	// Skip create_only files
	if file.Reconcile == "create_only" {
		change.WriteMode = ImportSkip
		change.Type = fileset.FileNoOp
		change.Reason = "create_only"
		return change
	}

	// Fetch current content from GitHub
	state, err := fetchFile(repo, file.Path)
	if err != nil || !state.Exists {
		change.WriteMode = ImportSkip
		change.Type = fileset.FileNoOp
		change.Reason = "not on GitHub"
		return change
	}

	// Read current local content for comparison
	var currentLocal string
	if file.OriginalSource != "" {
		if data, err := os.ReadFile(file.OriginalSource); err == nil {
			currentLocal = string(data)
		}
	} else {
		currentLocal = file.Content
	}

	change.Current = currentLocal
	change.Desired = state.Content

	if strings.TrimRight(state.Content, "\n") == strings.TrimRight(currentLocal, "\n") {
		change.Type = fileset.FileNoOp
		return change
	}

	if currentLocal == "" {
		change.Type = fileset.FileCreate
	} else {
		change.Type = fileset.FileUpdate
	}

	return change
}

func importYAMLPath(origin fileset.FileOrigin) (string, bool) {
	switch origin.Kind {
	case fileset.FileOriginSpecFiles:
		return fmt.Sprintf("$.spec.files[%d].content", origin.FileIndex), true
	case fileset.FileOriginRepositoryOverride:
		if origin.RepoIndex < 0 {
			return "", false
		}
		return fmt.Sprintf("$.spec.repositories[%d].overrides[%d].content", origin.RepoIndex, origin.FileIndex), true
	default:
		return "", false
	}
}

// ApplyImport executes the planned import changes.
// manifestBytes maps manifest file paths to their raw content (for inline edits).
func ApplyImport(changes []ImportChange, manifestBytes map[string][]byte) error {
	inlineByFile := make(map[string][]ImportChange)

	for _, c := range changes {
		switch c.WriteMode {
		case ImportWriteSource:
			if err := os.WriteFile(c.LocalTarget, []byte(c.Desired), 0644); err != nil {
				return fmt.Errorf("write %s: %w", c.LocalTarget, err)
			}
		case ImportWriteInline:
			inlineByFile[c.ManifestPath] = append(inlineByFile[c.ManifestPath], c)
		}
	}

	for path, inlineChanges := range inlineByFile {
		data, ok := manifestBytes[path]
		if !ok {
			return fmt.Errorf("no raw bytes for manifest %s", path)
		}

		var err error
		for _, c := range inlineChanges {
			data, err = yamlpatch.ReplaceLiteralContent(data, c.DocIndex, c.YAMLPath, c.Desired)
			if err != nil {
				return fmt.Errorf("replace inline content for %s at %s in %s: %w", c.Path, c.YAMLPath, path, err)
			}
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write manifest %s: %w", path, err)
		}
	}

	return nil
}

// ImportSummary returns counts by write mode.
func ImportSummary(changes []ImportChange) (written, unchanged, skipped int) {
	for _, c := range changes {
		switch c.WriteMode {
		case ImportWriteSource, ImportWriteInline:
			if c.Type == fileset.FileNoOp {
				unchanged++
			} else {
				written++
			}
		case ImportSkip:
			skipped++
		}
	}
	return
}

// HasImportChanges returns true if any import changes are non-noop and non-skip.
func HasImportChanges(changes []ImportChange) bool {
	for _, c := range changes {
		if c.Type != fileset.FileNoOp && c.WriteMode != ImportSkip {
			return true
		}
	}
	return false
}

// ResolveFilesForImport returns files for a target with FileOrigin metadata
// needed for import write-back. This is the import-direction resolver.
func ResolveFilesForImport(fs *manifest.FileSetDocument, target manifest.FileSetRepository, repoIndex int) []fileset.ResolvedFile {
	if len(target.Overrides) == 0 {
		result := make([]fileset.ResolvedFile, len(fs.Files))
		for i, f := range fs.Files {
			result[i] = fileset.ResolvedFile{
				FileEntry: f,
				Origin: fileset.FileOrigin{
					Kind:      fileset.FileOriginSpecFiles,
					FileIndex: i,
				},
			}
		}
		return result
	}

	overrideMap := make(map[string]fileset.ResolvedFile)
	for i, o := range target.Overrides {
		overrideMap[o.Path] = fileset.ResolvedFile{
			FileEntry: o,
			Origin: fileset.FileOrigin{
				Kind:      fileset.FileOriginRepositoryOverride,
				RepoIndex: repoIndex,
				FileIndex: i,
			},
		}
	}

	result := make([]fileset.ResolvedFile, 0, len(fs.Files))
	for i, f := range fs.Files {
		if override, ok := overrideMap[f.Path]; ok {
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
			result = append(result, fileset.ResolvedFile{
				FileEntry: f,
				Origin: fileset.FileOrigin{
					Kind:      fileset.FileOriginSpecFiles,
					FileIndex: i,
				},
			})
		}
	}
	return result
}
