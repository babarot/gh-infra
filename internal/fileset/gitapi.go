package fileset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// applyToRepo creates a verified commit for all file changes using the GitHub GraphQL
// createCommitOnBranch mutation. Falls back to Contents API for empty repositories.
// Returns (prURL, error); prURL is non-empty only for pull_request strategy.
func (p *Processor) applyToRepo(ctx context.Context, repo string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, error) {
	headSHA, defaultBranch, err := p.getHeadSHA(ctx, repo)
	if err != nil {
		if strings.Contains(err.Error(), "repository is empty") {
			return "", p.applyToEmptyRepo(ctx, repo, changes, opts)
		}
		return "", fmt.Errorf("get HEAD: %w", err)
	}
	return p.applyViaGraphQL(ctx, repo, defaultBranch, headSHA, changes, opts, statusFn)
}

// applyViaGraphQL creates a verified commit using the GitHub GraphQL createCommitOnBranch
// mutation. All file changes are committed atomically in a single call.
func (p *Processor) applyViaGraphQL(ctx context.Context, repo, defaultBranch, headSHA string, changes []Change, opts ApplyOptions, statusFn func(string)) (string, error) {
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

	statusFn("committing changes...")
	if err := p.commitViaGraphQL(ctx, repo, targetBranch, headSHA, message, changes); err != nil {
		return "", err
	}

	if opts.Via == manifest.ViaPullRequest {
		statusFn("creating pull request...")
		return p.openPR(ctx, repo, defaultBranch, targetBranch, opts)
	}
	return "", nil
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

	input := map[string]any{
		"branch": map[string]string{
			"repositoryNameWithOwner": repo,
			"branchName":              branch,
		},
		"message":         map[string]string{"headline": message},
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

	out, err = p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch), "--jq", ".object.sha")
	if err != nil {
		return "", "", err
	}
	sha = strings.TrimSpace(string(out))
	return sha, branch, nil
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
