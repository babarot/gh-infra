package fileset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/parallel"
	"github.com/babarot/gh-infra/internal/ui"
)

// FileState represents the current state of a file in a repository.
type FileState struct {
	Path    string
	Content string
	SHA     string // needed for updates via Contents API
	Exists  bool
}

// FileChange represents a planned change for a file.
type FileChange struct {
	FileSet string // FileSet owner
	Target  string // owner/repo
	Path    string
	Type    ChangeType
	Current string // current content (if exists)
	Desired string // desired content
	SHA     string // current SHA (for updates)
	OnDrift string // warn, overwrite, skip
	Drifted bool   // file exists but content differs
}

type ChangeType string

const (
	FileCreate ChangeType = "create"
	FileUpdate ChangeType = "update"
	FileDelete ChangeType = "delete"
	FileNoOp   ChangeType = "noop"
	FileDrift  ChangeType = "drift"
	FileSkip   ChangeType = "skip"
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
	onDrift     string
	owner       string
}

// fullName returns the full "owner/repo" name for this unit's target.
func (u planUnit) fullName() string {
	return u.owner + "/" + u.target.Name
}

// PlanTargetNames returns display tasks for all FileSet targets.
// If filterRepo is non-empty, only targets matching that repo are included.
func PlanTargetNames(fileSets []*manifest.FileSet, filterRepo string) []ui.RefreshTask {
	var tasks []ui.RefreshTask
	for _, fs := range fileSets {
		for _, target := range fs.Spec.Repositories {
			fullName := fs.Metadata.Owner + "/" + target.Name
			if filterRepo != "" && fullName != filterRepo {
				continue
			}
			tasks = append(tasks, ui.RefreshTask{
				Name:      "Fetching " + fullName + " files",
				DoneLabel: "Fetched " + fullName + " files",
			})
		}
	}
	return tasks
}

// planTaskKey returns the tracker key for a given fileset target.
// This must match the Name used in PlanTargetNames.
func planTaskKey(fullName string) string {
	return "Fetching " + fullName + " files"
}

// applyTaskKey returns the tracker key for a fileset apply target.
func applyTaskKey(repo string) string {
	return "Applying " + repo + " files"
}

// Plan computes changes for all FileSets concurrently.
// If filterRepo is non-empty, only targets matching that repo are processed.
func (p *Processor) Plan(fileSets []*manifest.FileSet, filterRepo string, tracker *ui.RefreshTracker) ([]FileChange, error) {
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
				onDrift:     fs.Spec.OnDrift,
				owner:       fs.Metadata.Owner,
			})
		}
	}

	// Process each unit concurrently; collect results in order.
	type unitResult struct {
		changes []FileChange
		err     error
	}
	results := parallel.Map(units, 0, func(i int, u planUnit) unitResult {
		fullName := u.fullName()
		displayName := planTaskKey(fullName)
		var out []FileChange
		for _, file := range u.files {
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
			// Mirror mode forces overwrite (mirror = complete sync)
			drift := u.onDrift
			if file.SyncMode == manifest.SyncModeMirror {
				drift = manifest.OnDriftOverwrite
			}
			change := p.planFile(u.fileSetName, fullName, file, drift)
			out = append(out, change)
		}
		// Mirror mode: detect orphaned files in target repo
		allPlannedPaths := make(map[string]bool)
		for _, change := range out {
			allPlannedPaths[change.Path] = true
		}
		mirrorDirs := make(map[string]bool)
		for _, file := range u.files {
			if file.SyncMode == manifest.SyncModeMirror && file.DirScope != "" {
				mirrorDirs[file.DirScope] = true
			}
		}
		for dirScope := range mirrorDirs {
			repoFiles, err := p.fetchDirectoryContents(fullName, dirScope)
			if err != nil {
				// Directory doesn't exist in repo yet — nothing to delete
				continue
			}
			for _, repoFile := range repoFiles {
				if !allPlannedPaths[repoFile] {
					out = append(out, FileChange{
						Type:    FileDelete,
						Target:  fullName,
						Path:    repoFile,
						FileSet: u.fileSetName,
					})
				}
			}
		}

		tracker.Done(displayName)
		return unitResult{changes: out}
	})

	// Flatten in original order; return first error.
	var changes []FileChange
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		changes = append(changes, r.changes...)
	}
	return changes, nil
}

func (p *Processor) planFile(fileSetName, repo string, file manifest.FileEntry, onDrift string) FileChange {
	current, err := p.fetchFileContent(repo, file.Path)
	if err != nil || !current.Exists {
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileCreate,
			Desired: file.Content,
			OnDrift: onDrift,
		}
	}

	// Normalize for comparison (trim trailing newlines)
	currentContent := strings.TrimRight(current.Content, "\n")
	desiredContent := strings.TrimRight(file.Content, "\n")

	if currentContent == desiredContent {
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileNoOp,
			OnDrift: onDrift,
		}
	}

	// Content differs — drift detected
	switch onDrift {
	case manifest.OnDriftSkip:
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileSkip,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	case manifest.OnDriftWarn:
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileDrift,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	default: // "overwrite"
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileUpdate,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	}
}

func (p *Processor) fetchFileContent(repo, path string) (*FileState, error) {
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/contents/%s", repo, path))
	if err != nil {
		return &FileState{Path: path, Exists: false}, err
	}

	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return &FileState{Path: path, Exists: false}, err
	}

	content := raw.Content
	if raw.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode base64 for %s: %w", path, err)
		}
		content = string(decoded)
	}

	return &FileState{
		Path:    path,
		Content: content,
		SHA:     raw.SHA,
		Exists:  true,
	}, nil
}

// fetchDirectoryContents returns all file paths under a directory in a repo (recursively).
func (p *Processor) fetchDirectoryContents(repo, dirPath string) ([]string, error) {
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/contents/%s", repo, dirPath))
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
			subFiles, err := p.fetchDirectoryContents(repo, item.Path)
			if err != nil {
				continue
			}
			files = append(files, subFiles...)
		}
	}
	return files, nil
}

// ApplyOptions configures apply behavior from FileSet spec.
type ApplyOptions struct {
	CommitMessage  string
	CommitStrategy string // "push" or "pull_request"
	Branch         string
	FileSetName    string
	PRTitle        string // custom PR title (pull_request only)
	PRBody         string // custom PR body (pull_request only)
}

const defaultApplyParallel = 5

// Apply executes the planned file changes using Git Data API.
// Changes are grouped by target repo and applied in parallel across repos.
func (p *Processor) Apply(changes []FileChange, opts ApplyOptions, reporter ui.ProgressReporter) []FileApplyResult {
	grouped := groupChangesByTarget(changes)

	// Build ordered repo list for deterministic output
	type repoEntry struct {
		name    string
		changes []FileChange
	}
	var repoList []repoEntry
	for repo, repoChanges := range grouped {
		repoList = append(repoList, repoEntry{name: repo, changes: repoChanges})
	}

	if len(repoList) == 0 {
		return nil
	}

	// Progress reporting is handled by the caller-provided reporter

	// Apply repos in parallel
	allResults := parallel.Map(repoList, defaultApplyParallel, func(_ int, entry repoEntry) []FileApplyResult {
		var results []FileApplyResult
		var filesToApply []FileChange
		for _, c := range entry.changes {
			switch c.Type {
			case FileCreate, FileUpdate, FileDelete:
				filesToApply = append(filesToApply, c)
			case FileDrift:
				results = append(results, FileApplyResult{Change: c, Skipped: true})
			case FileSkip, FileNoOp:
				// do nothing
			}
		}

		if len(filesToApply) == 0 {
			reporter.Done(entry.name, 0, 0)
			return results
		}

		paths := make([]string, len(filesToApply))
		for j, c := range filesToApply {
			paths[j] = c.Path
		}
		reporter.Start(entry.name, paths)

		start := time.Now()
		err := p.applyToRepo(entry.name, filesToApply, opts)
		elapsed := time.Since(start)

		for _, c := range filesToApply {
			results = append(results, FileApplyResult{Change: c, Err: err})
		}

		if err != nil {
			reporter.Error(entry.name, elapsed, err)
		} else {
			reporter.Done(entry.name, elapsed, len(filesToApply))
		}
		return results
	})
	reporter.Wait()

	// Flatten in order
	var results []FileApplyResult
	for _, r := range allResults {
		results = append(results, r...)
	}
	return results
}

type FileApplyResult struct {
	Change  FileChange
	Err     error
	Skipped bool
}

func groupChangesByTarget(changes []FileChange) map[string][]FileChange {
	grouped := make(map[string][]FileChange)
	for _, c := range changes {
		grouped[c.Target] = append(grouped[c.Target], c)
	}
	return grouped
}

// treeEntry represents a file entry in a Git tree.
// SHA is a pointer: non-nil for create/update, nil for delete (GitHub removes the file).
type treeEntry struct {
	Path string  `json:"path"`
	Mode string  `json:"mode"`
	Type string  `json:"type"`
	SHA  *string `json:"sha"` // nil = delete file from tree
}

// applyToRepo creates a single commit with all file changes using Git Data API.
// Falls back to Contents API for empty repositories (no commits yet).
func (p *Processor) applyToRepo(repo string, changes []FileChange, opts ApplyOptions) error {
	headSHA, defaultBranch, err := p.getHeadSHA(repo)
	if err != nil {
		if strings.Contains(err.Error(), "repository is empty") {
			return p.applyToEmptyRepo(repo, changes, opts)
		}
		return fmt.Errorf("get HEAD: %w", err)
	}

	message := resolveCommitMessage(opts)
	return p.applyViaGitDataAPI(repo, defaultBranch, headSHA, changes, message, opts)
}

// resolveCommitMessage returns the commit message from opts or a default.
func resolveCommitMessage(opts ApplyOptions) string {
	if opts.CommitMessage != "" {
		return opts.CommitMessage
	}
	return fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetName)
}

// applyViaGitDataAPI creates blobs, a tree, a commit, and updates the ref
// (or creates a PR) in a single atomic operation. All files are included in
// one commit regardless of count.
func (p *Processor) applyViaGitDataAPI(repo, branch, headSHA string, changes []FileChange, message string, opts ApplyOptions) error {
	// 1. Create blobs
	entries, err := p.createBlobs(repo, changes)
	if err != nil {
		return err
	}

	// 2. Create tree
	treeSHA, err := p.createTree(repo, headSHA, entries)
	if err != nil {
		return fmt.Errorf("create tree: %w", err)
	}

	// 3. Create commit
	commitSHA, err := p.createCommit(repo, message, treeSHA, headSHA)
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	// 4. Update ref or create PR
	if opts.CommitStrategy == manifest.CommitStrategyPullRequest {
		return p.createPR(repo, branch, commitSHA, message, opts)
	}
	return p.updateRef(repo, branch, commitSHA)
}

// createBlobs creates a Git blob for each file change and returns tree entries.
// FileDelete entries get a nil SHA which tells the Git Data API to remove the file.
func (p *Processor) createBlobs(repo string, changes []FileChange) ([]treeEntry, error) {
	var entries []treeEntry
	for _, c := range changes {
		if c.Type == FileDelete {
			entries = append(entries, treeEntry{
				Path: c.Path,
				Mode: "100644",
				Type: "blob",
				SHA:  nil, // nil SHA = delete from tree
			})
			continue
		}
		blobSHA, err := p.createBlob(repo, c.Desired)
		if err != nil {
			return nil, fmt.Errorf("create blob for %s: %w", c.Path, err)
		}
		sha := blobSHA
		entries = append(entries, treeEntry{
			Path: c.Path,
			Mode: "100644",
			Type: "blob",
			SHA:  &sha,
		})
	}
	return entries, nil
}

// applyToEmptyRepo uses Contents API as fallback for repos with no commits.
func (p *Processor) applyToEmptyRepo(repo string, changes []FileChange, opts ApplyOptions) error {
	p.printer.Progress(fmt.Sprintf("Updating %s (empty repo, using fallback)...", repo))
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetName)
	}
	for _, c := range changes {
		commitMsg := fmt.Sprintf("%s: %s", message, c.Path)
		if err := p.putFileViaContentsAPI(repo, c.Path, c.Desired, "", commitMsg); err != nil {
			return err
		}
	}
	return nil
}

// putFileViaContentsAPI creates or updates a single file using the Contents API.
// This results in one commit per call. Use for empty repos or when Git Data API
// is not available. Pass sha="" for new files, or the current blob SHA for updates.
func (p *Processor) putFileViaContentsAPI(repo, path, content, sha, message string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	endpoint := fmt.Sprintf("repos/%s/contents/%s", repo, path)

	args := []string{
		"api", endpoint,
		"--method", "PUT",
		"-f", fmt.Sprintf("message=%s", message),
		"-f", fmt.Sprintf("content=%s", encoded),
	}
	if sha != "" {
		args = append(args, "-f", fmt.Sprintf("sha=%s", sha))
	}

	_, err := p.runner.Run(args...)
	if err != nil {
		return fmt.Errorf("put %s: %w", path, err)
	}
	return nil
}

func (p *Processor) getHeadSHA(repo string) (sha, branch string, err error) {
	// Get default branch name
	out, err := p.runner.Run("repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if err != nil {
		return "", "", err
	}
	branch = strings.TrimSpace(string(out))
	if branch == "" {
		return "", "", fmt.Errorf("repository is empty (no default branch)")
	}

	// Get HEAD SHA
	out, err = p.runner.Run("api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch), "--jq", ".object.sha")
	if err != nil {
		return "", "", err
	}
	sha = strings.TrimSpace(string(out))
	return sha, branch, nil
}

func (p *Processor) createBlob(repo, content string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/blobs", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("content=%s", encoded),
		"-f", "encoding=base64",
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func (p *Processor) createTree(repo, baseTree string, entries any) (string, error) {
	body := map[string]any{
		"base_tree": baseTree,
		"tree":      entries,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	// Write JSON body to a temp file for --input
	tmpFile, err := os.CreateTemp("", "gh-infra-tree-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(bodyJSON); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/trees", repo),
		"--method", "POST",
		"--input", tmpFile.Name(),
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func (p *Processor) createCommit(repo, message, treeSHA, parentSHA string) (string, error) {
	body := map[string]any{
		"message": message,
		"tree":    treeSHA,
		"parents": []string{parentSHA},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp("", "gh-infra-commit-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(bodyJSON); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/commits", repo),
		"--method", "POST",
		"--input", tmpFile.Name(),
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func (p *Processor) updateRef(repo, branch, commitSHA string) error {
	_, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, branch),
		"--method", "PATCH",
		"-f", fmt.Sprintf("sha=%s", commitSHA),
	)
	return err
}

func (p *Processor) createPR(repo, defaultBranch, commitSHA, title string, opts ApplyOptions) error {
	branchName := opts.Branch
	if branchName == "" {
		branchName = fmt.Sprintf("gh-infra/sync-%s", opts.FileSetName)
	}

	// Create branch pointing to the new commit; if it already exists, force-update it.
	_, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/refs", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("ref=refs/heads/%s", branchName),
		"-f", fmt.Sprintf("sha=%s", commitSHA),
	)
	if err != nil {
		if strings.Contains(err.Error(), "Reference already exists") {
			_, err = p.runner.Run("api", fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, branchName),
				"--method", "PATCH",
				"-f", fmt.Sprintf("sha=%s", commitSHA),
				"-F", "force=true",
			)
		}
		if err != nil {
			return fmt.Errorf("create branch %s: %w", branchName, err)
		}
	}

	// Create PR (skip if one already exists for this head branch)
	prTitle := opts.PRTitle
	if prTitle == "" {
		prTitle = title
	}
	prBody := opts.PRBody
	if prBody == "" {
		prBody = fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetName)
	}
	_, err = p.runner.Run("pr", "create",
		"--repo", repo,
		"--base", defaultBranch,
		"--head", branchName,
		"--title", prTitle,
		"--body", prBody,
	)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return nil // PR already open for this branch
	}
	return err
}

// PrintPlan prints FileSet changes.
func PrintPlan(p ui.Printer, changes []FileChange) {
	if len(changes) == 0 {
		return
	}

	hasChanges := false
	for _, c := range changes {
		if c.Type != FileNoOp {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return
	}

	// Group by fileset+target
	type groupKey struct{ fileSet, target string }
	type group struct {
		key     groupKey
		changes []FileChange
	}
	seen := make(map[groupKey]int)
	var groups []group

	for _, c := range changes {
		if c.Type == FileNoOp {
			continue
		}
		k := groupKey{c.FileSet, c.Target}
		idx, ok := seen[k]
		if !ok {
			idx = len(groups)
			seen[k] = idx
			groups = append(groups, group{key: k})
		}
		groups[idx].changes = append(groups[idx].changes, c)
	}

	for _, g := range groups {
		maxPathLen := 0
		for _, c := range g.changes {
			if len(c.Path) > maxPathLen {
				maxPathLen = len(c.Path)
			}
		}
		p.SetColumnWidth(maxPathLen)
		label := fmt.Sprintf("%d file", len(g.changes))
		if len(g.changes) != 1 {
			label += "s"
		}
		p.GroupHeader(ui.IconChange, fmt.Sprintf("FileSet: %s → %s", ui.Bold.Render(label), ui.Bold.Render(g.key.target)))
		for _, c := range g.changes {
			switch c.Type {
			case FileCreate:
				p.FileCreate(c.Path)
			case FileUpdate:
				p.FileUpdate(c.Path)
			case FileDelete:
				p.FileDelete(c.Path)
			case FileDrift:
				p.FileDrift(c.Path, c.OnDrift)
			case FileSkip:
				p.FileSkip(c.Path)
			}
		}
		p.GroupEnd()
	}
	p.SetColumnWidth(0)
}

// PrintApplyResults prints the results of FileSet apply.
func PrintApplyResults(p ui.Printer, results []FileApplyResult) {
	for _, r := range results {
		if r.Skipped {
			p.Warning(fmt.Sprintf("%s %s", r.Change.Target, r.Change.Path),
				fmt.Sprintf("drift detected, skipped (on_drift: %s)", r.Change.OnDrift))
		} else if r.Err != nil {
			p.Error(r.Change.Target, fmt.Sprintf("%s: %s", r.Change.Path, r.Err.Error()))
		} else {
			p.Success(r.Change.Target, fmt.Sprintf("%s %sd", r.Change.Path, r.Change.Type))
		}
	}
}

// HasChanges returns true if any file changes are non-noop.
func HasChanges(changes []FileChange) bool {
	for _, c := range changes {
		if c.Type != FileNoOp && c.Type != FileSkip {
			return true
		}
	}
	return false
}

// CountChanges returns create, update, delete, drift counts.
func CountChanges(changes []FileChange) (creates, updates, deletes, drifts int) {
	for _, c := range changes {
		switch c.Type {
		case FileCreate:
			creates++
		case FileUpdate:
			updates++
		case FileDelete:
			deletes++
		case FileDrift:
			drifts++
		}
	}
	return
}

func PrintSummary(p ui.Printer, results []FileApplyResult) {
	succeeded := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		if r.Skipped {
			skipped++
		} else if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	msg := fmt.Sprintf("FileSet apply: %d changes applied", succeeded)
	if failed > 0 {
		msg += fmt.Sprintf(", %d failed", failed)
	}
	if skipped > 0 {
		msg += fmt.Sprintf(", %d skipped (drift)", skipped)
	}
	msg += "."
	p.Message("\n" + msg)
}
