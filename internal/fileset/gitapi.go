package fileset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/babarot/gh-infra/internal/manifest"
)

const maxCommitRetries = 3

// needsGitDataAPI reports whether any change has to change a file mode, which
// createCommitOnBranch cannot do.
func needsGitDataAPI(changes []Change) bool {
	for _, c := range changes {
		if c.ModeChanged() {
			return true
		}
	}
	return false
}

// commitFunc is a function that commits changes to a branch using a specific strategy.
type commitFunc func(ctx context.Context, repo, branch, headSHA, message string, changes []Change) error

// applyToRepo creates a verified commit for all file changes. Uses the GitHub
// GraphQL createCommitOnBranch mutation unless a file mode has to change, in
// which case the Git Data API is used and the commit is not signed. Falls back to
// Contents API for empty repositories.
// Returns (prURL, skipped, error); prURL is non-empty only for pull_request strategy,
// and skipped is true when the changes would not alter the HEAD tree, in which case
// no commit (and, for pull_request strategy, no branch or PR) is created.
// Retries up to maxCommitRetries times on HEAD conflict errors caused by concurrent commits,
// refetching the target branch's HEAD before each retry.
// For pull_request mode, branch creation and PR opening happen outside the retry loop so
// that only the commit itself is retried on conflict.
func (p *Processor) applyToRepo(ctx context.Context, repo string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, bool, error) {
	headSHA, defaultBranch, err := p.getHeadSHA(ctx, repo)
	if err != nil {
		if strings.Contains(err.Error(), "repository is empty") {
			return "", false, p.applyToEmptyRepo(ctx, repo, changes, opts)
		}
		return "", false, fmt.Errorf("get HEAD: %w", err)
	}

	if p.isNoopCommit(ctx, repo, headSHA, changes) {
		statusFn(noopCommitStatus)
		return "", true, nil
	}

	commit := commitFunc(p.commitViaGraphQL)
	if needsGitDataAPI(changes) {
		commit = p.commitViaGitDataAPI
	}
	message, err := resolveCommitMessage(opts, repo)
	if err != nil {
		return "", false, fmt.Errorf("render commit message: %w", err)
	}
	targetBranch := defaultBranch

	if opts.Via == manifest.ViaPullRequest {
		prBranch := opts.Branch
		if prBranch == "" {
			prBranch = fmt.Sprintf("gh-infra/sync-%s", sanitizeBranchName(opts.FileSetID))
		}
		statusFn("creating PR branch...")
		if err := p.createBranchAt(ctx, repo, prBranch, headSHA); err != nil {
			return "", false, fmt.Errorf("create PR branch: %w", err)
		}
		targetBranch = prBranch
	}

	skipped := false
	for attempt := range maxCommitRetries {
		// The concurrent commit that caused the HEAD conflict may already carry
		// the same content, so recheck against the refetched HEAD before retrying.
		if attempt > 0 && p.isNoopCommit(ctx, repo, headSHA, changes) {
			statusFn(noopCommitStatus)
			skipped = true
			break
		}
		statusFn("committing changes...")
		err := commit(ctx, repo, targetBranch, headSHA, message, changes)
		if err == nil {
			break
		}
		if !isHeadConflict(err) {
			return "", false, err
		}
		if attempt == maxCommitRetries-1 {
			return "", false, fmt.Errorf("commit retries exhausted for %s: %w", repo, err)
		}
		headSHA, err = p.getRefSHA(ctx, repo, targetBranch)
		if err != nil {
			return "", false, fmt.Errorf("get HEAD for retry: %w", err)
		}
	}

	if opts.Via == manifest.ViaPullRequest {
		// The PR branch exists at this point. If the retry was skipped, the
		// concurrent commit on it already carries the desired content, so the
		// PR is still opened (or the existing one returned).
		statusFn("creating pull request...")
		prURL, err := p.openPR(ctx, repo, defaultBranch, targetBranch, opts)
		return prURL, false, err
	}
	return "", skipped, nil
}

const noopCommitStatus = "no content change on GitHub, skipping commit"

// isNoopCommit reports whether committing changes on top of headSHA would produce
// a tree identical to headSHA's tree, i.e. an empty commit. This happens when the
// plan reports a difference that GitHub does not end up storing (#168).
//
// It creates a tree via the Git Data API with base_tree set to the HEAD tree and
// compares the resulting SHA. Only an unreferenced tree object is created; no
// commit or ref is touched. Any failure is treated as "not a noop" so the commit
// proceeds as before.
//
// Entries carry the mode the commit would give each file (see treeEntries), so a
// mode-only change is not mistaken for a noop. Known limitation: when
// createCommitOnBranch is used, the modes of files without `executable` are not
// looked up and are written as 100644, so an existing 100755 (or symlink) file
// whose content is otherwise unchanged yields a different tree and the commit
// proceeds. The check is conservative: it never skips a real change.
func (p *Processor) isNoopCommit(ctx context.Context, repo, headSHA string, changes []Change) bool {
	// The trees API takes content as a JSON string, which cannot carry
	// arbitrary bytes. Skip the check rather than compare a mangled blob.
	for _, c := range changes {
		if c.Type != ChangeDelete && !utf8.ValidString(c.Desired) {
			return false
		}
	}

	baseTree, err := p.commitTreeSHA(ctx, repo, headSHA)
	if err != nil || baseTree == "" {
		return false
	}
	var modes *modeLookup
	if needsGitDataAPI(changes) {
		modes = p.modeLookupAt(repo, headSHA)
	}
	entries, err := p.treeEntries(ctx, repo, changes, modes)
	if err != nil {
		return false
	}
	tree, err := p.createTree(ctx, repo, baseTree, entries)
	if err != nil {
		return false
	}
	return tree == baseTree
}

// isHeadConflict reports whether err is the error returned when a concurrent commit
// advances the branch between our HEAD fetch and the commit. createCommitOnBranch
// returns `Expected branch to point to "<sha>" but it did not.  Pull and try again.`;
// the Git Data API ref update returns `Update is not a fast forward`.
func isHeadConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return (strings.Contains(msg, "Expected branch to point to") && strings.Contains(msg, "but it did not")) ||
		strings.Contains(msg, "Update is not a fast forward")
}

// commitViaGitDataAPI commits changes with the Git Data API: it creates a tree
// on top of headSHA and a commit, then moves the branch to the commit. It is used
// only when a file mode has to change, which createCommitOnBranch cannot do.
// Unlike createCommitOnBranch, the commit is not signed.
func (p *Processor) commitViaGitDataAPI(ctx context.Context, repo, branch, headSHA, message string, changes []Change) error {
	baseTree, err := p.commitTreeSHA(ctx, repo, headSHA)
	if err != nil {
		return fmt.Errorf("get tree SHA: %w", err)
	}
	entries, err := p.treeEntries(ctx, repo, changes, p.modeLookupAt(repo, headSHA))
	if err != nil {
		return err
	}
	tree, err := p.createTree(ctx, repo, baseTree, entries)
	if err != nil {
		return fmt.Errorf("create tree: %w", err)
	}

	body, err := json.Marshal(map[string]any{
		"message": message,
		"tree":    tree,
		"parents": []string{headSHA},
	})
	if err != nil {
		return err
	}
	out, err := p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s/git/commits", repo),
		"--method", "POST",
		"--input", "-",
		"--jq", ".sha",
	)
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	commit := strings.TrimSpace(string(out))

	// Without force, the update fails unless it is a fast forward from headSHA,
	// which isHeadConflict treats like a createCommitOnBranch HEAD conflict.
	body, err = json.Marshal(map[string]any{"sha": commit})
	if err != nil {
		return err
	}
	if _, err := p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s/git/refs/heads/%s", repo, branch),
		"--method", "PATCH",
		"--input", "-",
	); err != nil {
		return fmt.Errorf("update ref: %w", err)
	}
	return nil
}

// treeEntries builds Trees API entries for changes. A file gets the mode its
// `executable` asks for. Otherwise it keeps its mode at the base commit when
// modes is non-nil, as createCommitOnBranch does, and gets 100644 when modes is
// nil or the file is new. Content that is not valid UTF-8 cannot be sent as a
// JSON string, so it is uploaded as a blob first.
func (p *Processor) treeEntries(ctx context.Context, repo string, changes []Change, modes *modeLookup) ([]map[string]any, error) {
	// map[string]any rather than a struct so that "sha": null (delete) is
	// marshaled instead of omitted.
	entries := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		entry := map[string]any{
			"path": c.Path,
			"mode": ModeFile,
			"type": "blob",
		}
		if c.Type == ChangeDelete {
			entry["sha"] = nil
			entries = append(entries, entry)
			continue
		}
		mode, err := entryMode(ctx, c, modes)
		if err != nil {
			return nil, err
		}
		entry["mode"] = mode
		if utf8.ValidString(c.Desired) {
			entry["content"] = c.Desired
		} else {
			sha, err := p.createBlob(ctx, repo, c.Desired)
			if err != nil {
				return nil, fmt.Errorf("create blob for %s: %w", c.Path, err)
			}
			entry["sha"] = sha
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func entryMode(ctx context.Context, c Change, modes *modeLookup) (string, error) {
	if c.Mode != "" {
		return c.Mode, nil
	}
	if modes == nil || c.Type == ChangeCreate {
		return ModeFile, nil
	}
	mode, err := modes.mode(ctx, c.Path)
	if err != nil {
		return "", err
	}
	if mode == ModeExecutable {
		return ModeExecutable, nil
	}
	return ModeFile, nil
}

// commitTreeSHA returns the SHA of the tree of the given commit.
func (p *Processor) commitTreeSHA(ctx context.Context, repo, commit string) (string, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/commits/%s", repo, commit), "--jq", ".tree.sha")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// createTree creates a tree from entries on top of baseTree and returns its SHA.
func (p *Processor) createTree(ctx context.Context, repo, baseTree string, entries []map[string]any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"base_tree": baseTree,
		"tree":      entries,
	})
	if err != nil {
		return "", err
	}
	out, err := p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s/git/trees", repo),
		"--method", "POST",
		"--input", "-",
		"--jq", ".sha",
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// createBlob uploads content as a blob and returns its SHA.
func (p *Processor) createBlob(ctx context.Context, repo, content string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"content":  base64.StdEncoding.EncodeToString([]byte(content)),
		"encoding": "base64",
	})
	if err != nil {
		return "", err
	}
	out, err := p.runner.RunWithStdin(ctx, body,
		"api", fmt.Sprintf("repos/%s/git/blobs", repo),
		"--method", "POST",
		"--input", "-",
		"--jq", ".sha",
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// commitViaGraphQL sends a createCommitOnBranch GraphQL mutation.
// GitHub automatically marks these commits as Verified regardless of token type.
func (p *Processor) commitViaGraphQL(ctx context.Context, repo, branch, headSHA, message string, changes []Change) error {
	type addition struct {
		Path     string `json:"path"`
		Contents string `json:"contents"` // base64-encoded
	}
	type deletion struct {
		Path string `json:"path"`
	}
	type fileChanges struct {
		Additions []addition `json:"additions,omitempty"`
		Deletions []deletion `json:"deletions,omitempty"`
	}

	var adds []addition
	var dels []deletion
	for _, c := range changes {
		if c.Type == ChangeDelete {
			dels = append(dels, deletion{Path: c.Path})
		} else {
			adds = append(adds, addition{
				Path:     c.Path,
				Contents: base64.StdEncoding.EncodeToString([]byte(c.Desired)),
			})
		}
	}

	headline, msgBody := splitCommitMessage(message)
	msgInput := map[string]string{"headline": headline}
	if msgBody != "" {
		msgInput["body"] = msgBody
	}

	input := map[string]any{
		"branch": map[string]string{
			"repositoryNameWithOwner": repo,
			"branchName":              branch,
		},
		"message":         msgInput,
		"expectedHeadOid": headSHA,
		"fileChanges":     fileChanges{Additions: adds, Deletions: dels},
	}

	const mutation = `mutation($input: CreateCommitOnBranchInput!) {
  createCommitOnBranch(input: $input) {
    commit { oid }
  }
}`

	body := map[string]any{
		"query":     mutation,
		"variables": map[string]any{"input": input},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp("", "gh-infra-graphql-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(bodyJSON); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	out, err := p.runner.Run(ctx, "api", "graphql", "--input", tmpFile.Name())
	if err != nil {
		return fmt.Errorf("graphql mutation: %w", err)
	}

	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err == nil && len(resp.Errors) > 0 {
		return fmt.Errorf("graphql: %s", resp.Errors[0].Message)
	}
	return nil
}

// applyToEmptyRepo uses Contents API as fallback for repos with no commits.
func (p *Processor) applyToEmptyRepo(ctx context.Context, repo string, changes []Change, opts ApplyOptions) error {
	// The Contents API cannot set a mode. Plan rejects this already; check
	// again in case the repository was emptied after plan.
	for _, c := range changes {
		if c.ModeChanged() {
			return errExecutableInEmptyRepo(c.Path)
		}
	}
	p.writer.Progress(fmt.Sprintf("Updating %s (empty repo, using fallback)...", repo))
	message, err := resolveCommitMessage(opts, repo)
	if err != nil {
		return fmt.Errorf("render commit message: %w", err)
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
// Pass sha="" for new files. Pass branch="" to use the repository's default branch.
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

func (p *Processor) getHeadSHA(ctx context.Context, repo string) (sha, branch string, err error) {
	out, err := p.runner.Run(ctx, "repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if err != nil {
		return "", "", err
	}
	branch = strings.TrimSpace(string(out))
	if branch == "" {
		return "", "", fmt.Errorf("repository is empty (no default branch)")
	}

	sha, err = p.getRefSHA(ctx, repo, branch)
	if err != nil {
		return "", "", err
	}
	return sha, branch, nil
}

// getRefSHA returns the commit SHA that the given branch currently points to.
func (p *Processor) getRefSHA(ctx context.Context, repo, branch string) (string, error) {
	out, err := p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch), "--jq", ".object.sha")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
		var err error
		prTitle, err = resolveCommitMessage(opts, repo)
		if err != nil {
			return "", fmt.Errorf("render PR title: %w", err)
		}
		prTitle, _ = splitCommitMessage(prTitle)
	} else if HasTemplate(prTitle, nil) {
		var err error
		prTitle, err = RenderCommitMessage(prTitle, repo, opts.SourceURL)
		if err != nil {
			return "", fmt.Errorf("render PR title: %w", err)
		}
	}
	prBody := opts.PRBody
	if prBody == "" {
		prBody = fmt.Sprintf("Automated file sync by gh-infra FileSet `%s`.", opts.FileSetID)
	} else if HasTemplate(prBody, nil) {
		var err error
		prBody, err = RenderCommitMessage(prBody, repo, opts.SourceURL)
		if err != nil {
			return "", fmt.Errorf("render PR body: %w", err)
		}
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

// splitCommitMessage splits a commit message into headline and body.
// The headline is the text before the first newline; body is everything after,
// with leading newlines stripped. Matches standard Git commit message convention.
func splitCommitMessage(msg string) (headline, body string) {
	if idx := strings.Index(msg, "\n"); idx >= 0 {
		headline = msg[:idx]
		body = strings.TrimLeft(msg[idx+1:], "\n")
	} else {
		headline = msg
	}
	return
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
