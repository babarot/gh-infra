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

// DiffFilesOptions configures file-level import diffing.
type DiffFilesOptions struct {
	FilterRepo     string
	SourceRefCount map[string]int
	OnStatus       func(string)
}

// DiffFiles computes file-level import changes for all matched FileSets.
// It fetches current content from GitHub and compares against local content.
func DiffFiles(ctx context.Context, runner gh.Runner, fileSets []*manifest.FileDocument, opts DiffFilesOptions) ([]Change, error) {
	var changes []Change

	for _, doc := range fileSets {
		fs := doc.Resource
		for repoIdx, repo := range fs.Spec.Repositories {
			fullName := fs.Metadata.Owner + "/" + repo.Name
			if opts.FilterRepo != "" && fullName != opts.FilterRepo {
				continue
			}

			repoCount := len(fs.Spec.Repositories)

			// Resolve files with overrides
			files := fileset.ResolveFiles(fs, repo)

			for _, file := range files {
				if opts.OnStatus != nil {
					opts.OnStatus("fetching file " + file.Path + "...")
				}
				meta := importEntryContext{
					Doc:       doc,
					RepoIndex: repoIdx,
					Repo:      repo,
					RepoCount: repoCount,
					Shared:    file.OriginalSource != "" && opts.SourceRefCount[file.OriginalSource] > 1,
				}
				change := planImportEntry(ctx, runner, fullName, file, meta)
				changes = append(changes, change)
			}
		}
	}

	return changes, nil
}

// buildSourceRefCount counts how many FileEntries across all FileSets reference
// each local source file (by OriginalSource path). This is used to detect shared
// templates that should not be overwritten during import.
func buildSourceRefCount(fileSets []*manifest.FileDocument) map[string]int {
	counts := make(map[string]int)
	for _, doc := range fileSets {
		for _, file := range doc.Files {
			if file.OriginalSource != "" {
				counts[file.OriginalSource]++
			}
		}
	}
	return counts
}

type importEntryContext struct {
	Doc       *manifest.FileDocument
	RepoIndex int
	Repo      manifest.FileSetRepository
	RepoCount int
	Shared    bool
}

// planImportEntry determines the suggested write mode, supported write modes, and
// diff contents for a single file entry.
func planImportEntry(ctx context.Context, runner gh.Runner, fullName string, file manifest.FileEntry, meta importEntryContext) Change {
	change := Change{
		Target:             fullName,
		Path:               file.Path,
		Type:               fileset.ChangeNoOp,
		CreateOnly:         file.Reconcile == manifest.ReconcileCreateOnly,
		HasExistingPatches: len(file.Patches) > 0,
	}
	hasTemplateSyntax := strings.Contains(file.Content, "<%")
	var templateTrace fileset.RenderedTemplate

	sourceBacked := file.OriginalSource != "" && !strings.HasPrefix(file.Source, "github://")
	inlineBacked := file.OriginalSource == "" && (file.Source == "" || !strings.HasPrefix(file.Source, "github://"))

	// Populate write target info upfront so it's available even for skipped entries.
	if sourceBacked {
		change.LocalTarget = file.OriginalSource
	} else if inlineBacked {
		change.ManifestPath = meta.Doc.SourcePath
		change.DocIndex = meta.Doc.DocIndex
		overrideIdx := findFileIndex(meta.Repo.Overrides, file.Path)
		if overrideIdx >= 0 {
			change.YAMLPath = fmt.Sprintf("$.spec.repositories[%d].overrides[%d].content", meta.RepoIndex, overrideIdx)
		} else {
			baseIdx := findFileIndex(meta.Doc.Resource.Spec.Files, file.Path)
			if baseIdx >= 0 {
				change.YAMLPath = fmt.Sprintf("$.spec.files[%d].content", baseIdx)
			}
		}
	}

	if hasTemplateSyntax {
		trace, ok := prepareTemplateReverse(file.Content, fullName, file.Vars)
		if !ok {
			setWriteMetadata(&change, WriteSkip)
			change.Reason = "cannot safely write back to template"
			return change
		}
		templateTrace = trace
	}

	if file.Source != "" && strings.HasPrefix(file.Source, "github://") {
		setWriteMetadata(&change, WriteSkip)
		change.Reason = "remote source (github://)"
		return change
	}

	patchSupported := false
	if len(file.Patches) > 0 || sourceBacked {
		patchSupported = configurePatchTarget(&change, file, meta)
	}
	if len(file.Patches) > 0 && !patchSupported {
		setWriteMetadata(&change, WriteSkip)
		change.Reason = "expanded from directory source"
		return change
	}
	patchPreferred := len(file.Patches) > 0 || meta.Shared
	availableModes := []WriteMode{WriteInline}
	suggestedMode := WriteInline
	if sourceBacked {
		availableModes = []WriteMode{WriteSource}
		suggestedMode = WriteSource
		if patchSupported {
			availableModes = []WriteMode{WriteSource, WritePatch}
			if patchPreferred {
				suggestedMode = WritePatch
			}
		}
	} else if patchSupported {
		availableModes = []WriteMode{WriteInline, WritePatch}
		suggestedMode = WritePatch
	}
	setWriteMetadata(&change, suggestedMode, availableModes...)

	// Fetch current content from GitHub
	githubContent, err := fetchFileContent(ctx, runner, fullName, file.Path)
	if err != nil {
		// File doesn't exist on GitHub — nothing to import
		change.Type = fileset.ChangeNoOp
		return change
	}

	desiredContent := githubContent
	if hasTemplateSyntax {
		reversed, ok := reverseRenderedTemplate(templateTrace, githubContent)
		if !ok {
			setWriteMetadata(&change, WriteSkip)
			change.Type = fileset.ChangeNoOp
			change.Reason = "cannot safely write back to template"
			return change
		}
		desiredContent = reversed
	}

	change.Desired = desiredContent

	change.WriteCurrent = file.Content

	if len(file.Patches) > 0 {
		patchedContent, err := fileset.ApplyPatches(fileset.EnsureTrailingNewline(file.Content), file.Patches)
		if err != nil {
			setWriteMetadata(&change, WriteSkip)
			change.Type = fileset.ChangeNoOp
			change.Reason = fmt.Sprintf("cannot apply existing patches: %v", err)
			return change
		}
		change.PatchCurrent = patchedContent
	} else if patchSupported {
		change.PatchCurrent = file.Content
	}

	change.UpdateTypeForMode(change.SuggestedWriteMode)
	if patchSupported {
		if strings.TrimRight(change.WriteCurrent, "\n") == strings.TrimRight(desiredContent, "\n") {
			change.PatchContent = ""
		} else {
			patch, err := fileset.GeneratePatch(file.Content, desiredContent, file.Path)
			if err != nil {
				setWriteMetadata(&change, WriteSkip)
				change.Type = fileset.ChangeNoOp
				change.Reason = fmt.Sprintf("patch generation failed: %v", err)
				return change
			}
			change.PatchContent = patch
		}
	}

	change.UpdateTypeForMode(change.SuggestedWriteMode)
	if change.Type == fileset.ChangeNoOp {
		return change
	}

	return change
}

func configurePatchTarget(change *Change, file manifest.FileEntry, meta importEntryContext) bool {
	overrideIdx := findFileIndex(meta.Repo.Overrides, file.Path)
	if overrideIdx >= 0 {
		change.ManifestPath = meta.Doc.SourcePath
		change.DocIndex = meta.Doc.DocIndex
		change.PatchYAMLPath = fmt.Sprintf("$.spec.repositories[%d].overrides[%d]", meta.RepoIndex, overrideIdx)
		if file.OriginalSource == "" {
			change.YAMLPath = change.PatchYAMLPath + ".content"
		}
	} else {
		baseIdx := findFileIndex(meta.Doc.Resource.Spec.Files, file.Path)
		if baseIdx < 0 {
			return false
		}
		change.ManifestPath = meta.Doc.SourcePath
		change.DocIndex = meta.Doc.DocIndex
		change.PatchYAMLPath = fmt.Sprintf("$.spec.files[%d]", baseIdx)
		if file.OriginalSource == "" {
			change.YAMLPath = change.PatchYAMLPath + ".content"
		}
		if meta.RepoCount > 1 {
			change.Warnings = append(change.Warnings,
				fmt.Sprintf("shared manifest: affects %d repositories", meta.RepoCount))
		}
	}

	entryCopy := file
	change.PatchEntry = &entryCopy
	return true
}

func setWriteMetadata(change *Change, suggested WriteMode, available ...WriteMode) {
	change.WriteMode = suggested
	change.SuggestedWriteMode = suggested
	change.AvailableModes = append([]WriteMode(nil), available...)
}

// findFileIndex returns the index of the first FileEntry with the given path, or -1 if not found.
func findFileIndex(files []manifest.FileEntry, path string) int {
	for i, f := range files {
		if f.Path == path {
			return i
		}
	}
	return -1
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
