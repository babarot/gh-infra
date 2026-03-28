package fileset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

// Processor handles FileSet plan and apply operations.
type Processor struct {
	runner  gh.Runner
	printer ui.Printer
}

func NewProcessor(runner gh.Runner, printer ui.Printer) *Processor {
	return &Processor{runner: runner, printer: printer}
}

// planUnit represents one (fileSet, repository) pair to process.
type planUnit struct {
	fileSetName string
	target      manifest.FileSetRepository
	files       []manifest.FileEntry
	owner       string
	via         string
}

// fullName returns the full "owner/repo" name for this unit's target.
func (u planUnit) fullName() string {
	return u.owner + "/" + u.target.Name
}

// PlanTargetRepoNames returns the list of repo full names for all FileSet targets.
// If filterRepo is non-empty, only targets matching that repo are included.
func PlanTargetRepoNames(fileSets []*manifest.FileSet, filterRepo string) []string {
	var names []string
	for _, fs := range fileSets {
		for _, target := range fs.Spec.Repositories {
			fullName := fs.Metadata.Owner + "/" + target.Name
			if filterRepo != "" && fullName != filterRepo {
				continue
			}
			names = append(names, fullName)
		}
	}
	return names
}

// planTaskKey returns the tracker key for a given fileset target.
func planTaskKey(fullName string) string {
	return "Fetching " + fullName
}

// Plan computes changes for all FileSets concurrently.
// If filterRepo is non-empty, only targets matching that repo are processed.
func (p *Processor) Plan(ctx context.Context, fileSets []*manifest.FileSet, filterRepo string, tracker *ui.RefreshTracker) ([]Change, error) {
	// Build work units (order-preserving index).
	var units []planUnit
	for _, fs := range fileSets {
		for _, target := range fs.Spec.Repositories {
			fullName := fs.Metadata.Owner + "/" + target.Name
			if filterRepo != "" && fullName != filterRepo {
				continue
			}
			files := ResolveFiles(fs, target)
			units = append(units, planUnit{
				fileSetName: fs.Metadata.Owner,
				target:      target,
				files:       files,
				owner:       fs.Metadata.Owner,
				via:         fs.Spec.Via,
			})
		}
	}

	// Process each unit concurrently; collect results in order.
	type unitResult struct {
		changes []Change
		err     error
	}
	results := parallel.Map(ctx, units, 0, func(ctx context.Context, i int, u planUnit) unitResult {
		fullName := u.fullName()
		displayName := planTaskKey(fullName)
		updateStatus := func(s string) {
			tracker.UpdateStatus(displayName, s)
		}
		var out []Change
		for _, file := range u.files {
			updateStatus(file.Path + "...")
			// Template rendering (deep copy vars to avoid data races)
			needsTemplate := HasTemplate(file.Content, file.Vars) || HasTemplate(file.Path, nil)
			if needsTemplate {
				varsCopy := copyVars(file.Vars)
				// Render path
				if HasTemplate(file.Path, nil) {
					renderedPath, err := RenderTemplate(file.Path, fullName, varsCopy)
					if err != nil {
						tracker.Error(displayName, err)
						return unitResult{err: fmt.Errorf("template path %s for %s: %w", file.Path, fullName, err)}
					}
					file.Path = renderedPath
				}
				// Render content
				rendered, err := RenderTemplate(file.Content, fullName, varsCopy)
				if err != nil {
					tracker.Error(displayName, err)
					return unitResult{err: fmt.Errorf("template %s for %s: %w", file.Path, fullName, err)}
				}
				file.Content = rendered
			}
			// Apply unified diff patches (after template rendering)
			if len(file.Patches) > 0 {
				patched, err := ApplyPatches(file.Content, file.Patches)
				if err != nil {
					tracker.Error(displayName, err)
					return unitResult{err: fmt.Errorf("patch %s for %s: %w", file.Path, fullName, err)}
				}
				file.Content = patched
			}
			// create_only: create if missing, skip entirely if exists
			if file.Reconcile == manifest.ReconcileCreateOnly {
				change := p.planCreateOnly(ctx, u.fileSetName, fullName, file)
				out = append(out, change)
				continue
			}
			change := p.planFile(ctx, u.fileSetName, fullName, file)
			out = append(out, change)
		}
		// Mirror mode: detect orphaned files in target repo
		allPlannedPaths := make(map[string]bool)
		for _, change := range out {
			allPlannedPaths[change.Path] = true
		}
		mirrorDirs := make(map[string]bool)
		for _, file := range u.files {
			if file.Reconcile == manifest.ReconcileMirror && file.DirScope != "" {
				mirrorDirs[file.DirScope] = true
			}
		}
		for dirScope := range mirrorDirs {
			updateStatus("mirror: " + dirScope + "...")
			repoFiles, err := p.fetchDirectoryContents(ctx, fullName, dirScope)
			if err != nil {
				// Directory doesn't exist in repo yet — nothing to delete
				continue
			}
			for _, repoFile := range repoFiles {
				if !allPlannedPaths[repoFile] {
					out = append(out, Change{
						Type:         ChangeDelete,
						Target:       fullName,
						Path:         repoFile,
						FileSetOwner: u.fileSetName,
					})
				}
			}
		}

		// Tag all changes with the commit strategy for display
		for i := range out {
			out[i].Via = u.via
		}

		tracker.Done(displayName)
		return unitResult{changes: out}
	})

	// If canceled, return immediately without surfacing errors.
	if ctx.Err() != nil {
		return nil, nil
	}

	// Flatten in original order; return first error.
	var changes []Change
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		changes = append(changes, r.changes...)
	}
	return changes, nil
}

// planCreateOnly handles sync_mode: create_only — create if missing, NoOp if exists.
func (p *Processor) planCreateOnly(ctx context.Context, fileSetName, repo string, file manifest.FileEntry) Change {
	current, err := p.fetchFileContent(ctx, repo, file.Path)
	if err != nil || !current.Exists {
		return Change{
			FileSetOwner: fileSetName,
			Target:       repo,
			Path:         file.Path,
			Type:         ChangeCreate,
			Desired:      file.Content,
		}
	}
	return Change{
		FileSetOwner: fileSetName,
		Target:       repo,
		Path:         file.Path,
		Type:         ChangeNoOp,
	}
}

func (p *Processor) planFile(ctx context.Context, fileSetName, repo string, file manifest.FileEntry) Change {
	current, err := p.fetchFileContent(ctx, repo, file.Path)
	if err != nil || !current.Exists {
		return Change{
			FileSetOwner: fileSetName,
			Target:       repo,
			Path:         file.Path,
			Type:         ChangeCreate,
			Desired:      file.Content,
		}
	}

	// Normalize for comparison (trim trailing newlines)
	currentContent := strings.TrimRight(current.Content, "\n")
	desiredContent := strings.TrimRight(file.Content, "\n")

	if currentContent == desiredContent {
		return Change{
			FileSetOwner: fileSetName,
			Target:       repo,
			Path:         file.Path,
			Type:         ChangeNoOp,
		}
	}

	// Content differs — update
	return Change{
		FileSetOwner: fileSetName,
		Target:       repo,
		Path:         file.Path,
		Type:         ChangeUpdate,
		Current:      current.Content,
		Desired:      file.Content,
		SHA:          current.SHA,
	}
}

func (p *Processor) fetchFileContent(ctx context.Context, repo, path string) (*State, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/contents/%s", repo, path))
	if err != nil {
		return &State{Path: path, Exists: false}, err
	}

	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return &State{Path: path, Exists: false}, err
	}

	content := raw.Content
	if raw.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode base64 for %s: %w", path, err)
		}
		content = string(decoded)
	}

	return &State{
		Path:    path,
		Content: content,
		SHA:     raw.SHA,
		Exists:  true,
	}, nil
}

// fetchDirectoryContents returns all file paths under a directory in a repo (recursively).
func (p *Processor) fetchDirectoryContents(ctx context.Context, repo, dirPath string) ([]string, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/contents/%s", repo, dirPath))
	if err != nil {
		return nil, err
	}

	var items []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}

	var files []string
	for _, item := range items {
		if item.Type == "file" {
			files = append(files, item.Path)
		} else if item.Type == "dir" {
			subFiles, err := p.fetchDirectoryContents(ctx, repo, item.Path)
			if err != nil {
				continue
			}
			files = append(files, subFiles...)
		}
	}
	return files, nil
}
