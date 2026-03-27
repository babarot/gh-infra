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

// FileImportChange extends FileChange with import-specific (GitHub → local) metadata.
type FileImportChange struct {
	FileChange
	WriteMode    ImportWriteMode
	LocalTarget  string   // write-back destination path
	ManifestPath string   // path to the manifest YAML file (for inline edits)
	DocIndex     int      // document index within the manifest file
	FileIndex    int      // index in spec.files[]
	Reason       string   // reason for skip
	Warnings     []string // e.g. patches, templates
}

// PlanPull computes import changes for all FileSets.
// filterRepo must be "owner/repo" format; required if a FileSet targets multiple repos.
func PlanPull(proc *Processor, fileSets []*manifest.FileSet, filterRepo string) ([]FileImportChange, error) {
	var changes []FileImportChange

	for _, fs := range fileSets {
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
		fullName := fs.RepoFullName(target.Name)
		files := ResolveFiles(fs, target)

		for i, file := range files {
			change := planImportEntry(proc, file, i, fullName, fs)
			changes = append(changes, change)
		}
	}

	return changes, nil
}

func planImportEntry(proc *Processor, file manifest.FileEntry, fileIndex int, repo string, fs *manifest.FileSet) FileImportChange {
	change := FileImportChange{
		FileChange:   FileChange{Target: repo, Path: file.Path},
		ManifestPath: fs.SourcePath(),
		DocIndex:     fs.DocIndex(),
		FileIndex:    fileIndex,
	}

	// Determine write target
	if file.OriginalSource != "" {
		change.WriteMode = ImportWriteSource
		change.LocalTarget = file.OriginalSource
	} else if file.Source == "" {
		// Inline content (no source field was set)
		change.WriteMode = ImportWriteInline
		change.LocalTarget = fs.SourcePath() + " (inline)"
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

	// Set FileChange fields: Current = local, Desired = GitHub (what will be written)
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

	// Warn about patches
	if len(file.Patches) > 0 {
		change.Warnings = append(change.Warnings, "has patches — pulled content includes applied patches")
	}

	// Warn about templates
	if HasTemplate(currentLocal, file.Vars) {
		change.Warnings = append(change.Warnings, "uses templates — pulled content is repo-specific")
	}

	return change
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

		// Process in reverse file index order to avoid position shifts
		sortImportReverse(inlineChanges)

		var err error
		for _, c := range inlineChanges {
			data, err = ReplaceInlineContent(data, c.DocIndex, c.FileIndex, c.Desired)
			if err != nil {
				return fmt.Errorf("replace inline content for %s in %s: %w", c.Path, path, err)
			}
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write manifest %s: %w", path, err)
		}
	}

	return nil
}

// sortImportReverse sorts import changes by FileIndex in descending order.
func sortImportReverse(changes []FileImportChange) {
	for i := 1; i < len(changes); i++ {
		for j := i; j > 0 && changes[j].FileIndex > changes[j-1].FileIndex; j-- {
			changes[j], changes[j-1] = changes[j-1], changes[j]
		}
	}
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

// ToFileChanges extracts the base FileChange from each import change for unified plan display.
func ToFileChanges(changes []FileImportChange) []FileChange {
	out := make([]FileChange, len(changes))
	for i := range changes {
		out[i] = changes[i].FileChange
	}
	return out
}
