package fileset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

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
// applyToRepo returns (prURL, error). prURL is non-empty only for pull_request strategy.
func (p *Processor) applyToRepo(repo string, changes []Change, opts ApplyOptions) (string, error) {
	headSHA, defaultBranch, err := p.getHeadSHA(repo)
	if err != nil {
		if strings.Contains(err.Error(), "repository is empty") {
			return "", p.applyToEmptyRepo(repo, changes, opts)
		}
		return "", fmt.Errorf("get HEAD: %w", err)
	}

	message := resolveCommitMessage(opts)
	return p.applyViaGitDataAPI(repo, defaultBranch, headSHA, changes, message, opts)
}

// applyViaGitDataAPI creates blobs, a tree, a commit, and updates the ref
// (or creates a PR) in a single atomic operation. All files are included in
// one commit regardless of count.
func (p *Processor) applyViaGitDataAPI(repo, branch, headSHA string, changes []Change, message string, opts ApplyOptions) (string, error) {
	// 1. Create blobs
	entries, err := p.createBlobs(repo, changes)
	if err != nil {
		return "", err
	}

	// 2. Create tree
	treeSHA, err := p.createTree(repo, headSHA, entries)
	if err != nil {
		return "", fmt.Errorf("create tree: %w", err)
	}

	// 3. Create commit
	commitSHA, err := p.createCommit(repo, message, treeSHA, headSHA)
	if err != nil {
		return "", fmt.Errorf("create commit: %w", err)
	}

	// 4. Update ref or create PR
	if opts.Via == manifest.ViaPullRequest {
		return p.createPR(repo, branch, commitSHA, message, opts)
	}
	return "", p.updateRef(repo, branch, commitSHA)
}

// createBlobs creates a Git blob for each file change and returns tree entries.
// ChangeDelete entries get a nil SHA which tells the Git Data API to remove the file.
func (p *Processor) createBlobs(repo string, changes []Change) ([]treeEntry, error) {
	var entries []treeEntry
	for _, c := range changes {
		if c.Type == ChangeDelete {
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
func (p *Processor) applyToEmptyRepo(repo string, changes []Change, opts ApplyOptions) error {
	p.printer.Progress(fmt.Sprintf("Updating %s (empty repo, using fallback)...", repo))
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetOwner)
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

// createPR creates a pull request and returns its URL.
func (p *Processor) createPR(repo, defaultBranch, commitSHA, title string, opts ApplyOptions) (string, error) {
	branchName := opts.Branch
	if branchName == "" {
		branchName = fmt.Sprintf("gh-infra/sync-%s", opts.FileSetOwner)
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
			return "", fmt.Errorf("create branch %s: %w", branchName, err)
		}
	}

	// Create PR (skip if one already exists for this head branch)
	prTitle := opts.PRTitle
	if prTitle == "" {
		prTitle = title
	}
	prBody := opts.PRBody
	if prBody == "" {
		prBody = fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetOwner)
	}
	out, err := p.runner.Run("pr", "create",
		"--repo", repo,
		"--base", defaultBranch,
		"--head", branchName,
		"--title", prTitle,
		"--body", prBody,
	)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		// Retrieve existing PR URL
		existing, lookupErr := p.runner.Run("pr", "view",
			"--repo", repo,
			branchName,
			"--json", "url", "--jq", ".url",
		)
		if lookupErr == nil {
			return strings.TrimSpace(string(existing)), nil
		}
		return "", nil // PR already open but couldn't get URL
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
