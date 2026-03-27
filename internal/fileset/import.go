package fileset

import (
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// ImportWriteMode describes how the imported content will be written locally.
type ImportWriteMode string

const (
	ImportWriteSource ImportWriteMode = "source" // overwrite a source file on disk
	ImportWriteInline ImportWriteMode = "inline" // update inline content: block in manifest YAML
	ImportSkip        ImportWriteMode = "skip"   // skipped (create_only, not on GitHub, etc.)
)

// FileImportChange represents a planned file change for the import direction (GitHub → local).
type FileImportChange struct {
	Target       string // owner/repo
	Path         string
	Type         ChangeType
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

// PlanImport computes import changes for all FileSets.
// filterRepo must be "owner/repo" format; required if a FileSet targets multiple repos.
func PlanImport(proc *Processor, fileSets []*manifest.FileSetDocument, filterRepo string) ([]FileImportChange, error) {
	var changes []FileImportChange

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
		files := ResolveFilesForTarget(fsDoc, target, repoIndex)

		for _, file := range files {
			change := planImportEntry(proc, file, fullName, fsDoc)
			changes = append(changes, change)
		}
	}

	return changes, nil
}

func planImportEntry(proc *Processor, file manifest.ResolvedFile, repo string, fs *manifest.FileSetDocument) FileImportChange {
	change := FileImportChange{
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
			change.Type = FileNoOp
			change.Reason = "inline source mapping unavailable"
			return change
		}
		change.WriteMode = ImportWriteInline
		change.LocalTarget = fs.SourcePath + " (inline)"
		change.YAMLPath = yamlPath
	} else {
		// github:// or other remote source — can't write back locally
		change.WriteMode = ImportSkip
		change.Type = FileNoOp
		change.Reason = "remote source"
		return change
	}

	// Skip create_only files
	if file.Reconcile == "create_only" {
		change.WriteMode = ImportSkip
		change.Type = FileNoOp
		change.Reason = "create_only"
		return change
	}

	// Fetch current content from GitHub
	state, err := proc.fetchFileContent(repo, file.Path)
	if err != nil || !state.Exists {
		change.WriteMode = ImportSkip
		change.Type = FileNoOp
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

	// Current = local, Desired = GitHub (what will be written)
	change.Current = currentLocal
	change.Desired = state.Content

	// Check if content is identical
	if strings.TrimRight(state.Content, "\n") == strings.TrimRight(currentLocal, "\n") {
		change.Type = FileNoOp
		return change
	}

	// Content differs
	if currentLocal == "" {
		change.Type = FileCreate
	} else {
		change.Type = FileUpdate
	}

	return change
}

func importYAMLPath(origin manifest.FileOrigin) (string, bool) {
	switch origin.Kind {
	case manifest.FileOriginSpecFiles:
		return fmt.Sprintf("$.spec.files[%d].content", origin.FileIndex), true
	case manifest.FileOriginRepositoryOverride:
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
func ApplyImport(changes []FileImportChange, manifestBytes map[string][]byte) error {
	// Group inline changes by manifest file and process in reverse index order
	// to avoid position shifts when modifying the same file.
	inlineByFile := make(map[string][]FileImportChange)

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

	// Apply inline changes per manifest file
	for path, inlineChanges := range inlineByFile {
		data, ok := manifestBytes[path]
		if !ok {
			return fmt.Errorf("no raw bytes for manifest %s", path)
		}

		var err error
		for _, c := range inlineChanges {
			data, err = manifest.ReplaceLiteralContent(data, c.DocIndex, c.YAMLPath, c.Desired)
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
func ImportSummary(changes []FileImportChange) (written, unchanged, skipped int) {
	for _, c := range changes {
		switch c.WriteMode {
		case ImportWriteSource, ImportWriteInline:
			if c.Type == FileNoOp {
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
func HasImportChanges(changes []FileImportChange) bool {
	for _, c := range changes {
		if c.Type != FileNoOp && c.WriteMode != ImportSkip {
			return true
		}
	}
	return false
}
