package fileset

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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

// githubUser holds information about the authenticated GitHub user.
type githubUser struct {
	name  string
	email string
	isBot bool // true when authenticated as a GitHub App (login ends with "[bot]")
}

// getGitHubUser returns the authenticated user, fetching it only once per Processor.
func (p *Processor) getGitHubUser(ctx context.Context) (githubUser, error) {
	p.userOnce.Do(func() {
		p.userInfo, p.userErr = p.fetchGitHubUser(ctx)
	})
	return p.userInfo, p.userErr
}

func (p *Processor) fetchGitHubUser(ctx context.Context) (githubUser, error) {
	out, err := p.runner.Run(ctx, "api", "/user", "--jq", ".login+\"|\"+(.name // .login)")
	if err != nil {
		return githubUser{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	login := parts[0]
	name := parts[0]
	if len(parts) == 2 {
		name = parts[1]
	}

	isBot := strings.HasSuffix(login, "[bot]")

	var email string
	if !isBot {
		out, err = p.runner.Run(ctx, "api", "/user/emails", "--jq", "[.[] | select(.primary == true)] | .[0].email")
		if err != nil {
			return githubUser{}, err
		}
		email = strings.TrimSpace(string(out))
	}

	return githubUser{name: name, email: email, isBot: isBot}, nil
}

// applyToRepo creates commits for all file changes in the given repo.
// - GitHub App token (isBot): uses Contents API, which GitHub auto-signs as Verified.
// - PAT: uses Git Data API with local GPG signing.
// Falls back to Contents API for empty repositories (no commits yet).
// Returns (prURL, error); prURL is non-empty only for pull_request strategy.
func (p *Processor) applyToRepo(ctx context.Context, repo string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, error) {
	headSHA, defaultBranch, err := p.getHeadSHA(ctx, repo)
	if err != nil {
		if strings.Contains(err.Error(), "repository is empty") {
			return "", p.applyToEmptyRepo(ctx, repo, changes, opts)
		}
		return "", fmt.Errorf("get HEAD: %w", err)
	}

	user, err := p.getGitHubUser(ctx)
	if err != nil {
		return "", fmt.Errorf("get GitHub user: %w", err)
	}

	if user.isBot {
		return p.applyViaContentsAPI(ctx, repo, defaultBranch, headSHA, changes, opts, statusFn)
	}
	return p.applyViaGitDataAPI(ctx, repo, defaultBranch, headSHA, changes, opts, statusFn)
}

// applyViaGitDataAPI creates blobs, a tree, a GPG-signed commit, and updates the ref
// (or creates a PR) in a single atomic operation.
func (p *Processor) applyViaGitDataAPI(ctx context.Context, repo, branch, headSHA string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, error) {
	message := resolveCommitMessage(opts)

	// 1. Create blobs
	statusFn("creating blobs...")
	entries, err := p.createBlobs(ctx, repo, changes)
	if err != nil {
		return "", err
	}

	// 2. Create tree
	statusFn("creating tree...")
	treeSHA, err := p.createTree(ctx, repo, headSHA, entries)
	if err != nil {
		return "", fmt.Errorf("create tree: %w", err)
	}

	// 3. Create commit
	statusFn("creating commit...")
	commitSHA, err := p.createCommit(ctx, repo, message, treeSHA, headSHA)
	if err != nil {
		return "", fmt.Errorf("create commit: %w", err)
	}

	// 4. Update ref or create PR
	if opts.Via == manifest.ViaPullRequest {
		statusFn("creating pull request...")
		return p.createPR(ctx, repo, branch, commitSHA, opts)
	}
	statusFn("updating ref...")
	return "", p.updateRef(ctx, repo, branch, commitSHA)
}

// applyViaContentsAPI applies changes one file at a time using the Contents API.
// GitHub automatically marks commits made with an App installation token as Verified.
func (p *Processor) applyViaContentsAPI(ctx context.Context, repo, defaultBranch, headSHA string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, error) {
	message := resolveCommitMessage(opts)
	targetBranch := defaultBranch

	if opts.Via == manifest.ViaPullRequest {
		prBranch := opts.Branch
		if prBranch == "" {
			prBranch = fmt.Sprintf("gh-infra/sync-%s", sanitizeBranchName(opts.FileSetID))
		}
		statusFn("creating PR branch...")
		if err := p.createBranchAt(ctx, repo, prBranch, headSHA); err != nil {
			return "", fmt.Errorf("create PR branch: %w", err)
		}
		targetBranch = prBranch
	}

	for _, c := range changes {
		statusFn(fmt.Sprintf("updating %s...", c.Path))
		commitMsg := fmt.Sprintf("%s: %s", message, c.Path)
		var err error
		if c.Type == ChangeDelete {
			err = p.deleteFileViaContentsAPI(ctx, repo, c.Path, c.SHA, commitMsg, targetBranch)
		} else {
			err = p.putFileViaContentsAPI(ctx, repo, c.Path, c.Desired, c.SHA, commitMsg, targetBranch)
		}
		if err != nil {
			return "", err
		}
	}

	if opts.Via == manifest.ViaPullRequest {
		statusFn("creating pull request...")
		return p.openPR(ctx, repo, defaultBranch, targetBranch, opts)
	}
	return "", nil
}

// createBlobs creates a Git blob for each file change and returns tree entries.
// ChangeDelete entries get a nil SHA which tells the Git Data API to remove the file.
func (p *Processor) createBlobs(ctx context.Context, repo string, changes []Change) ([]treeEntry, error) {
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
		blobSHA, err := p.createBlob(ctx, repo, c.Desired)
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
func (p *Processor) applyToEmptyRepo(ctx context.Context, repo string, changes []Change, opts ApplyOptions) error {
	p.writer.Progress(fmt.Sprintf("Updating %s (empty repo, using fallback)...", repo))
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("chore: sync %s files via gh-infra", opts.FileSetID)
	}
	for _, c := range changes {
		commitMsg := fmt.Sprintf("%s: %s", message, c.Path)
		if err := p.putFileViaContentsAPI(ctx, repo, c.Path, c.Desired, "", commitMsg, ""); err != nil {
			return err
		}
	}
	return nil
}

// putFileViaContentsAPI creates or updates a single file using the Contents API.
// One commit per call. Pass sha="" for new files, or the current blob SHA for updates.
// Pass branch="" to use the repository's default branch.
func (p *Processor) putFileViaContentsAPI(ctx context.Context, repo, path, content, sha, message, branch string) error {
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
	if branch != "" {
		args = append(args, "-f", fmt.Sprintf("branch=%s", branch))
	}

	_, err := p.runner.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("put %s: %w", path, err)
	}
	return nil
}

// deleteFileViaContentsAPI deletes a single file using the Contents API.
// sha is the current blob SHA of the file (required by GitHub).
// Pass branch="" to use the repository's default branch.
func (p *Processor) deleteFileViaContentsAPI(ctx context.Context, repo, path, sha, message, branch string) error {
	endpoint := fmt.Sprintf("repos/%s/contents/%s", repo, path)

	args := []string{
		"api", endpoint,
		"--method", "DELETE",
		"-f", fmt.Sprintf("message=%s", message),
		"-f", fmt.Sprintf("sha=%s", sha),
	}
	if branch != "" {
		args = append(args, "-f", fmt.Sprintf("branch=%s", branch))
	}

	_, err := p.runner.Run(ctx, args...)
	if err != nil {
		return fmt.Errorf("delete %s: %w", path, err)
	}
	return nil
}

func (p *Processor) getHeadSHA(ctx context.Context, repo string) (sha, branch string, err error) {
	// Get default branch name
	out, err := p.runner.Run(ctx, "repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if err != nil {
		return "", "", err
	}
	branch = strings.TrimSpace(string(out))
	if branch == "" {
		return "", "", fmt.Errorf("repository is empty (no default branch)")
	}

	// Get HEAD SHA
	out, err = p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch), "--jq", ".object.sha")
	if err != nil {
		return "", "", err
	}
	sha = strings.TrimSpace(string(out))
	return sha, branch, nil
}

func (p *Processor) createBlob(ctx context.Context, repo, content string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/blobs", repo),
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

func (p *Processor) createTree(ctx context.Context, repo, baseTree string, entries any) (string, error) {
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

	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/trees", repo),
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

func (p *Processor) createCommit(ctx context.Context, repo, message, treeSHA, parentSHA string) (string, error) {
	now := time.Now().UTC()

	user, err := p.getGitHubUser(ctx)
	if err != nil {
		return "", fmt.Errorf("get GitHub user: %w", err)
	}

	commitObj := buildCommitObject(message, treeSHA, parentSHA, user.name, user.email, now)
	sig, err := gpgSign(commitObj)
	if err != nil {
		return "", fmt.Errorf("sign commit: %w", err)
	}

	dateStr := now.Format(time.RFC3339)
	body := map[string]any{
		"message": message,
		"tree":    treeSHA,
		"parents": []string{parentSHA},
		"author": map[string]string{
			"name":  user.name,
			"email": user.email,
			"date":  dateStr,
		},
		"committer": map[string]string{
			"name":  user.name,
			"email": user.email,
			"date":  dateStr,
		},
		"signature": sig,
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

	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/commits", repo),
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

func buildCommitObject(message, treeSHA, parentSHA, name, email string, ts time.Time) string {
	unix := ts.Unix()
	tz := ts.Format("-0700")
	return fmt.Sprintf("tree %s\nparent %s\nauthor %s <%s> %d %s\ncommitter %s <%s> %d %s\n\n%s\n",
		treeSHA, parentSHA,
		name, email, unix, tz,
		name, email, unix, tz,
		message,
	)
}

func gpgSign(payload string) (string, error) {
	// Respect git config user.signingkey; fall back to GPG default key.
	args := []string{"--detach-sign", "--armor", "--batch"}
	if keyID := gitSigningKey(); keyID != "" {
		args = append(args, "-u", keyID)
	}
	cmd := exec.Command("gpg", args...)
	cmd.Stdin = strings.NewReader(payload)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gpg: %w: %s", err, errOut.String())
	}
	return out.String(), nil
}

func gitSigningKey() string {
	out, err := exec.Command("git", "config", "--get", "user.signingkey").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (p *Processor) updateRef(ctx context.Context, repo, branch, commitSHA string) error {
	_, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, branch),
		"--method", "PATCH",
		"-f", fmt.Sprintf("sha=%s", commitSHA),
	)
	return err
}

// createBranchAt creates or force-updates a branch pointing to the given SHA.
func (p *Processor) createBranchAt(ctx context.Context, repo, branch, sha string) error {
	_, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/refs", repo),
		"--method", "POST",
		"-f", fmt.Sprintf("ref=refs/heads/%s", branch),
		"-f", fmt.Sprintf("sha=%s", sha),
	)
	if err != nil {
		if strings.Contains(err.Error(), "Reference already exists") {
			_, err = p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, branch),
				"--method", "PATCH",
				"-f", fmt.Sprintf("sha=%s", sha),
				"-F", "force=true",
			)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// openPR opens a pull request from head into base. The head branch must already exist.
func (p *Processor) openPR(ctx context.Context, repo, base, head string, opts ApplyOptions) (string, error) {
	prTitle := opts.PRTitle
	if prTitle == "" {
		prTitle = resolveCommitMessage(opts)
	}
	prBody := opts.PRBody
	if prBody == "" {
		prBody = fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetID)
	}
	out, err := p.runner.Run(ctx, "pr", "create",
		"--repo", repo,
		"--base", base,
		"--head", head,
		"--title", prTitle,
		"--body", prBody,
	)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		existing, lookupErr := p.runner.Run(ctx, "pr", "view",
			"--repo", repo,
			head,
			"--json", "url", "--jq", ".url",
		)
		if lookupErr == nil {
			return strings.TrimSpace(string(existing)), nil
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// createPR creates a branch at commitSHA and opens a pull request.
func (p *Processor) createPR(ctx context.Context, repo, defaultBranch, commitSHA string, opts ApplyOptions) (string, error) {
	branchName := opts.Branch
	if branchName == "" {
		branchName = fmt.Sprintf("gh-infra/sync-%s", sanitizeBranchName(opts.FileSetID))
	}
	if err := p.createBranchAt(ctx, repo, branchName, commitSHA); err != nil {
		return "", fmt.Errorf("create branch %s: %w", branchName, err)
	}
	return p.openPR(ctx, repo, defaultBranch, branchName, opts)
}

// sanitizeBranchName converts an identity string into a valid Git branch name component.
func sanitizeBranchName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "..", "")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '~', '^', ':', '?', '*', '[', '\\':
			return -1
		}
		return r
	}, s)
	s = strings.Trim(s, "-.")
	return s
}
