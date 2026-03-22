package fileset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
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
	FileSet string // FileSet name
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
	FileNoOp   ChangeType = "noop"
	FileDrift  ChangeType = "drift"
	FileSkip   ChangeType = "skip"
)

// Processor handles FileSet plan and apply operations.
type Processor struct {
	runner gh.Runner
}

func NewProcessor(runner gh.Runner) *Processor {
	return &Processor{runner: runner}
}

// Plan computes changes for all FileSets.
func (p *Processor) Plan(fileSets []*manifest.FileSet) []FileChange {
	var changes []FileChange

	for _, fs := range fileSets {
		for _, target := range fs.Spec.Targets {
			fmt.Fprintf(os.Stderr, "  Refreshing %s → %s...\n", fs.Metadata.Name, target.Name)
			files := ResolveFiles(fs, target)
			for _, file := range files {
				change := p.planFile(fs.Metadata.Name, target.Name, file, fs.Spec.OnDrift)
				changes = append(changes, change)
			}
		}
	}

	return changes
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

// ApplyOptions configures apply behavior from FileSet spec.
type ApplyOptions struct {
	CommitMessage string
	Strategy      string // "direct" or "pull_request"
	Branch        string
	FileSetName   string
}

// Apply executes the planned file changes using Git Data API.
// Changes are grouped by target repo and applied as a single commit.
func (p *Processor) Apply(changes []FileChange, opts ApplyOptions) []FileApplyResult {
	// Separate actionable changes from skipped/noop
	var results []FileApplyResult
	grouped := groupChangesByTarget(changes)

	for repo, repoChanges := range grouped {
		// Collect skipped/drift changes
		var filesToApply []FileChange
		for _, c := range repoChanges {
			switch c.Type {
			case FileCreate, FileUpdate:
				filesToApply = append(filesToApply, c)
			case FileDrift:
				results = append(results, FileApplyResult{Change: c, Skipped: true})
			case FileSkip, FileNoOp:
				// do nothing
			}
		}

		if len(filesToApply) == 0 {
			continue
		}

		fmt.Fprintf(os.Stderr, "  Committing %s (%d files)...\n", repo, len(filesToApply))
		err := p.applyToRepo(repo, filesToApply, opts)
		for _, c := range filesToApply {
			results = append(results, FileApplyResult{Change: c, Err: err})
		}
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

// applyToRepo creates a single commit with all file changes using Git Data API.
func (p *Processor) applyToRepo(repo string, changes []FileChange, opts ApplyOptions) error {
	// 1. Get current HEAD SHA
	headSHA, defaultBranch, err := p.getHeadSHA(repo)
	if err != nil {
		return fmt.Errorf("get HEAD: %w", err)
	}

	// 2. Create blobs for each file
	type treeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	var entries []treeEntry

	for _, c := range changes {
		blobSHA, err := p.createBlob(repo, c.Desired)
		if err != nil {
			return fmt.Errorf("create blob for %s: %w", c.Path, err)
		}
		entries = append(entries, treeEntry{
			Path: c.Path,
			Mode: "100644",
			Type: "blob",
			SHA:  blobSHA,
		})
	}

	// 3. Create tree
	treeSHA, err := p.createTree(repo, headSHA, entries)
	if err != nil {
		return fmt.Errorf("create tree: %w", err)
	}

	// 4. Create commit
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetName)
	}
	commitSHA, err := p.createCommit(repo, message, treeSHA, headSHA)
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}

	// 5. Update ref (direct to default branch)
	if opts.Strategy == manifest.StrategyPullRequest {
		return p.createPR(repo, defaultBranch, commitSHA, message, opts)
	}
	return p.updateRef(repo, defaultBranch, commitSHA)
}

func (p *Processor) getHeadSHA(repo string) (sha, branch string, err error) {
	// Get default branch name
	out, err := p.runner.Run("repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if err != nil {
		return "", "", err
	}
	branch = strings.TrimSpace(string(out))

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
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/trees", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("base_tree=%s", baseTree),
		"--raw-field", fmt.Sprintf("tree=%s", string(entriesJSON)),
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
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/commits", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("message=%s", message),
		"-f", fmt.Sprintf("tree=%s", treeSHA),
		"-f", fmt.Sprintf("parents[]=%s", parentSHA),
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

	// Create branch pointing to the new commit
	_, err := p.runner.Run("api", fmt.Sprintf("repos/%s/git/refs", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("ref=refs/heads/%s", branchName),
		"-f", fmt.Sprintf("sha=%s", commitSHA),
	)
	if err != nil {
		return fmt.Errorf("create branch %s: %w", branchName, err)
	}

	// Create PR
	_, err = p.runner.Run("pr", "create",
		"--repo", repo,
		"--base", defaultBranch,
		"--head", branchName,
		"--title", title,
		"--body", fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetName),
	)
	return err
}

// PrintPlan prints FileSet changes to the writer.
func PrintPlan(w io.Writer, changes []FileChange) {
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
		fmt.Fprintf(w, "  %s FileSet: %s → %s\n",
			ui.Yellow.Render("~"),
			ui.Bold.Render(g.key.fileSet),
			ui.Bold.Render(g.key.target))
		for _, c := range g.changes {
			switch c.Type {
			case FileCreate:
				fmt.Fprintf(w, "      %s %s  %s\n",
					ui.Green.Render("+"), c.Path, ui.Green.Render("(new file)"))
			case FileUpdate:
				fmt.Fprintf(w, "      %s %s  %s\n",
					ui.Yellow.Render("~"), c.Path, ui.Yellow.Render("(content changed)"))
			case FileDrift:
				fmt.Fprintf(w, "      %s %s  %s on_drift: %s → skipping apply\n",
					ui.Yellow.Render("⚠"), c.Path, ui.Yellow.Render("[drift detected]"), c.OnDrift)
			case FileSkip:
				fmt.Fprintf(w, "      %s %s  %s on_drift: skip → ignored\n",
					ui.Dim.Render("-"), c.Path, ui.Dim.Render("[drift detected]"))
			}
		}
		fmt.Fprintln(w)
	}
}

// PrintApplyResults prints the results of FileSet apply.
func PrintApplyResults(w io.Writer, results []FileApplyResult) {
	for _, r := range results {
		if r.Skipped {
			fmt.Fprintf(w, "  %s %s %s  drift detected, skipped (on_drift: %s)\n",
				ui.Yellow.Render("⚠"), ui.Bold.Render(r.Change.Target), r.Change.Path, r.Change.OnDrift)
		} else if r.Err != nil {
			fmt.Fprintf(w, "  %s %s %s: %v\n",
				ui.Red.Render("✗"), ui.Bold.Render(r.Change.Target), r.Change.Path, r.Err)
		} else {
			fmt.Fprintf(w, "  %s %s %s  %sd\n",
				ui.Green.Render("✓"), ui.Bold.Render(r.Change.Target), r.Change.Path, r.Change.Type)
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

// CountChanges returns create, update, drift counts.
func CountChanges(changes []FileChange) (creates, updates, drifts int) {
	for _, c := range changes {
		switch c.Type {
		case FileCreate:
			creates++
		case FileUpdate:
			updates++
		case FileDrift:
			drifts++
		}
	}
	return
}

func PrintSummary(w io.Writer, results []FileApplyResult) {
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
	fmt.Fprintf(w, "\nFileSet apply: %d changes applied", succeeded)
	if failed > 0 {
		fmt.Fprintf(w, ", %d failed", failed)
	}
	if skipped > 0 {
		fmt.Fprintf(w, ", %d skipped (drift)", skipped)
	}
	fmt.Fprintln(w, ".")
}
