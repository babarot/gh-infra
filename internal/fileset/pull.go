package fileset

import (
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// PullType describes what will happen for a file during pull.
type PullType string

const (
	PullWriteSource PullType = "write_source"
	PullWriteInline PullType = "write_inline"
	PullSkip        PullType = "skip"
	PullNoOp        PullType = "noop"
)

// PullChange represents a planned pull-back operation for a single file.
type PullChange struct {
	Path           string   // file path in the GitHub repository
	LocalTarget    string   // write-back destination (source file path or manifest path for inline)
	Type           PullType
	FetchedContent string
	CurrentLocal   string
	Reason         string   // reason for skip
	Warnings       []string // e.g. patches, templates

	// For inline content replacement
	ManifestPath string // path to the manifest YAML file
	DocIndex     int    // document index within the manifest file
	FileIndex    int    // index in spec.files[]
}

// PlanPull computes pull changes for all FileSets.
// filterRepo must be "owner/repo" format; required if a FileSet targets multiple repos.
func PlanPull(proc *Processor, fileSets []*manifest.FileSet, filterRepo string) ([]PullChange, error) {
	var changes []PullChange

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
			change := planPullEntry(proc, file, i, fullName, fs)
			changes = append(changes, change)
		}
	}

	return changes, nil
}

func planPullEntry(proc *Processor, file manifest.FileEntry, fileIndex int, repo string, fs *manifest.FileSet) PullChange {
	change := PullChange{
		Path:         file.Path,
		ManifestPath: fs.SourcePath(),
		DocIndex:     fs.DocIndex(),
		FileIndex:    fileIndex,
	}

	// Determine write target
	if file.OriginalSource != "" {
		change.Type = PullWriteSource
		change.LocalTarget = file.OriginalSource
	} else if file.Source == "" {
		// Inline content (no source field was set)
		change.Type = PullWriteInline
		change.LocalTarget = fs.SourcePath() + " (inline)"
	} else {
		// github:// or other remote source — can't write back locally
		change.Type = PullSkip
		change.Reason = "remote source"
		return change
	}

	// Skip create_only files
	if file.Reconcile == "create_only" {
		change.Type = PullSkip
		change.Reason = "create_only"
		return change
	}

	// Fetch current content from GitHub
	state, err := proc.fetchFileContent(repo, file.Path)
	if err != nil || !state.Exists {
		change.Type = PullSkip
		change.Reason = "not on GitHub"
		return change
	}

	change.FetchedContent = state.Content

	// Read current local content for comparison
	if file.OriginalSource != "" {
		if data, err := os.ReadFile(file.OriginalSource); err == nil {
			change.CurrentLocal = string(data)
		}
	} else {
		change.CurrentLocal = file.Content
	}

	// Check if content is identical
	if strings.TrimRight(change.FetchedContent, "\n") == strings.TrimRight(change.CurrentLocal, "\n") {
		change.Type = PullNoOp
		return change
	}

	// Warn about patches
	if len(file.Patches) > 0 {
		change.Warnings = append(change.Warnings, "has patches — pulled content includes applied patches")
	}

	// Warn about templates
	if HasTemplate(change.CurrentLocal, file.Vars) {
		change.Warnings = append(change.Warnings, "uses templates — pulled content is repo-specific")
	}

	return change
}

// ApplyPull executes the planned pull changes.
// manifestBytes maps manifest file paths to their raw content (for inline edits).
func ApplyPull(changes []PullChange, manifestBytes map[string][]byte) error {
	// Group inline changes by manifest file and process in reverse index order
	// to avoid position shifts when modifying the same file.
	inlineByFile := make(map[string][]PullChange)

	for _, c := range changes {
		switch c.Type {
		case PullWriteSource:
			if err := os.WriteFile(c.LocalTarget, []byte(c.FetchedContent), 0644); err != nil {
				return fmt.Errorf("write %s: %w", c.LocalTarget, err)
			}
		case PullWriteInline:
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
		sortInlineReverse(inlineChanges)

		var err error
		for _, c := range inlineChanges {
			data, err = ReplaceInlineContent(data, c.DocIndex, c.FileIndex, c.FetchedContent)
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

// sortInlineReverse sorts inline changes by FileIndex in descending order.
func sortInlineReverse(changes []PullChange) {
	for i := 1; i < len(changes); i++ {
		for j := i; j > 0 && changes[j].FileIndex > changes[j-1].FileIndex; j-- {
			changes[j], changes[j-1] = changes[j-1], changes[j]
		}
	}
}

// PullSummary returns counts of each pull type.
func PullSummary(changes []PullChange) (written, unchanged, skipped int) {
	for _, c := range changes {
		switch c.Type {
		case PullWriteSource, PullWriteInline:
			written++
		case PullNoOp:
			unchanged++
		case PullSkip:
			skipped++
		}
	}
	return
}
