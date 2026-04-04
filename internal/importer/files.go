package importer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// DiffFiles computes file-level import changes for all matched FileSets.
// It fetches current content from GitHub and compares against local content.
func DiffFiles(ctx context.Context, runner gh.Runner, fileSets []*manifest.FileDocument, filterRepo string) ([]Change, error) {
	var changes []Change

	for _, doc := range fileSets {
		fs := doc.Resource
		for _, repo := range fs.Spec.Repositories {
			fullName := fs.Metadata.Owner + "/" + repo.Name
			if filterRepo != "" && fullName != filterRepo {
				continue
			}

			repoCount := len(fs.Spec.Repositories)

			// Resolve files with overrides
			files := fileset.ResolveFiles(fs, repo)

			for fileIdx, file := range files {
				change := planImportEntry(ctx, runner, fullName, file, fileIdx, doc, repoCount)
				changes = append(changes, change)
			}
		}
	}

	return changes, nil
}

// planImportEntry determines the WriteMode and computes the diff for a single file entry.
func planImportEntry(ctx context.Context, runner gh.Runner, fullName string, file manifest.FileEntry, fileIdx int, doc *manifest.FileDocument, repoCount int) Change {
	change := Change{
		Target: fullName,
		Path:   file.Path,
		Type:   fileset.ChangeNoOp,
	}

	// Populate local path info upfront so it's available even for skipped entries.
	if file.OriginalSource != "" && !strings.HasPrefix(file.Source, "github://") {
		change.LocalTarget = file.OriginalSource
	} else if file.Source == "" || !strings.HasPrefix(file.Source, "github://") {
		change.ManifestPath = doc.SourcePath
		change.DocIndex = doc.DocIndex
		change.YAMLPath = fmt.Sprintf("$.spec.files[%d].content", fileIdx)
	}

	// Templates and patches: skip (reverse transformation is impossible)
	if len(file.Vars) > 0 && len(file.Patches) > 0 {
		change.WriteMode = WriteSkip
		change.Reason = "uses templates and patches"
		return change
	}
	if len(file.Vars) > 0 {
		change.WriteMode = WriteSkip
		change.Reason = "uses template variables"
		return change
	}
	if len(file.Patches) > 0 {
		change.WriteMode = WriteSkip
		change.Reason = "uses patches"
		return change
	}

	// Determine write mode based on source
	// (create_only is allowed — importing updates the local master template)
	if file.OriginalSource != "" {
		if strings.HasPrefix(file.Source, "github://") {
			change.WriteMode = WriteSkip
			change.Reason = "remote source (github://)"
			return change
		}
		change.WriteMode = WriteSource
	} else if file.Source != "" && strings.HasPrefix(file.Source, "github://") {
		change.WriteMode = WriteSkip
		change.Reason = "remote source (github://)"
		return change
	} else {
		change.WriteMode = WriteInline
	}

	// Shared source warning
	if repoCount > 1 && change.WriteMode == WriteSource {
		change.Warnings = append(change.Warnings,
			fmt.Sprintf("shared source: affects %d repositories", repoCount))
	}

	// Fetch current content from GitHub
	githubContent, err := fetchFileContent(ctx, runner, fullName, file.Path)
	if err != nil {
		// File doesn't exist on GitHub — nothing to import
		change.Type = fileset.ChangeNoOp
		return change
	}

	change.Desired = githubContent

	// Compare with local content
	localContent := file.Content
	change.Current = localContent

	currentNorm := strings.TrimRight(localContent, "\n")
	desiredNorm := strings.TrimRight(githubContent, "\n")

	if currentNorm == desiredNorm {
		change.Type = fileset.ChangeNoOp
		return change
	}

	change.Type = fileset.ChangeUpdate
	return change
}

// fetchFileContent fetches a file's content from GitHub via the Contents API.
func fetchFileContent(ctx context.Context, runner gh.Runner, repo, path string) (string, error) {
	if runner == nil {
		return "", fmt.Errorf("no runner available")
	}
	out, err := runner.Run(ctx, "api", fmt.Sprintf("repos/%s/contents/%s", repo, path))
	if err != nil {
		return "", err
	}

	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", err
	}

	content := raw.Content
	if raw.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("decode base64 for %s: %w", path, err)
		}
		content = string(decoded)
	}

	return content, nil
}
