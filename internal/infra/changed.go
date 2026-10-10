package infra

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// gitChanges is what changed in the git repository since a ref.
type gitChanges struct {
	toplevel string          // repository root, symlinks resolved
	paths    map[string]bool // changed files, absolute (under toplevel)
}

// runGit runs git with stdin closed and returns stdout, or an error carrying
// git's stderr.
var runGit = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// changedSince lists the files changed since ref: from the merge base of ref
// to the working tree (committed and uncommitted), plus untracked files.
// git runs at the repository root, since ls-files --others only lists the
// current directory's subtree.
func changedSince(ctx context.Context, ref string) (*gitChanges, error) {
	out, err := runGit(ctx, "", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	top := normalizePath(strings.TrimSpace(string(out)))

	diff, err := runGit(ctx, top, "diff", "--name-only", "--no-renames", "--merge-base", ref, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := runGit(ctx, top, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	changes := &gitChanges{toplevel: top, paths: map[string]bool{}}
	for _, out := range [][]byte{diff, untracked} {
		for line := range strings.SplitSeq(string(out), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				changes.paths[filepath.Join(top, filepath.FromSlash(line))] = true
			}
		}
	}
	return changes, nil
}

// normalizePath resolves symlinks when the path exists (macOS /tmp is
// /private/tmp), so manifest paths compare equal to paths under the git
// toplevel, which git reports resolved.
func normalizePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// within reports whether p is dir or under it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// touches reports whether a changed path is dep itself or, for a directory
// dependency, under it.
func (c *gitChanges) touches(dep string) bool {
	dep = normalizePath(dep)
	if c.paths[dep] {
		return true
	}
	for p := range c.paths {
		if within(p, dep) {
			return true
		}
	}
	return false
}

// selection is which parsed documents --changed-since keeps.
type selection struct {
	files map[string]bool // manifest files that changed: all their documents
	docs  map[docKey]bool // FileSet / File documents whose local files changed
}

type docKey struct {
	path  string
	index int
}

func (s selection) empty() bool { return len(s.files) == 0 && len(s.docs) == 0 }

// manifestFiles returns the manifest files holding a selected document.
func (s selection) manifestFiles() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for p := range s.files {
		add(p)
	}
	for k := range s.docs {
		add(k.path)
	}
	return out
}

// selectChanged picks the documents affected by changes: every document of a
// changed manifest file, and every FileSet or File document that reads a
// changed local file.
func selectChanged(parsed *manifest.ParseResult, changes *gitChanges) selection {
	sel := selection{files: map[string]bool{}, docs: map[docKey]bool{}}
	changedFile := func(p string) bool { return changes.paths[normalizePath(p)] }
	for _, d := range parsed.RepositoryDocs {
		if changedFile(d.SourcePath) {
			sel.files[d.SourcePath] = true
		}
	}
	for _, d := range parsed.FileDocs {
		if changedFile(d.SourcePath) {
			sel.files[d.SourcePath] = true
			continue
		}
		if slices.ContainsFunc(d.LocalDeps, changes.touches) {
			sel.docs[docKey{d.SourcePath, d.DocIndex}] = true
		}
	}
	return sel
}

// keep filters a parse of the selected manifest files down to the selected
// documents.
func (s selection) keep(parsed *manifest.ParseResult) *manifest.ParseResult {
	out := &manifest.ParseResult{Warnings: parsed.Warnings}
	for _, d := range parsed.RepositoryDocs {
		if s.files[d.SourcePath] {
			out.RepositoryDocs = append(out.RepositoryDocs, d)
			out.Repositories = append(out.Repositories, d.Resource)
		}
	}
	for _, d := range parsed.FileDocs {
		if s.files[d.SourcePath] || s.docs[docKey{d.SourcePath, d.DocIndex}] {
			out.FileDocs = append(out.FileDocs, d)
			out.FileSets = append(out.FileSets, d.Resource)
		}
	}
	return out
}

// parseChanged parses paths and keeps only what changed since ref. All
// manifests are first parsed without fetching github:// sources, to validate
// across every manifest and to select; then only the manifest files holding
// a selected document are parsed again for planning. It returns the kept
// result and the number of resources before and after selection.
func parseChanged(ctx context.Context, paths []string, ref string, opt manifest.ParseOptions) (*manifest.ParseResult, int, int, error) {
	changes, err := changedSince(ctx, ref)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("--changed-since %s: %w", ref, err)
	}
	for _, p := range paths {
		if !within(normalizePath(p), changes.toplevel) {
			return nil, 0, 0, fmt.Errorf("--changed-since: %s is not in the git repository at %s", tildePath(p), tildePath(changes.toplevel))
		}
	}

	all := &manifest.ParseResult{}
	selectOpt := opt
	selectOpt.SkipRemoteSources = true
	for _, p := range paths {
		result, err := manifest.ParseAll(p, selectOpt)
		if err != nil {
			return nil, 0, 0, err
		}
		all.Merge(result)
	}
	if err := all.Validate(); err != nil {
		return nil, 0, 0, err
	}
	total := len(all.RepositoryDocs) + len(all.FileDocs)

	sel := selectChanged(all, changes)
	if sel.empty() {
		return &manifest.ParseResult{}, total, 0, nil
	}

	reparsed := &manifest.ParseResult{}
	for _, f := range sel.manifestFiles() {
		result, err := manifest.ParseAll(f, opt)
		if err != nil {
			return nil, 0, 0, err
		}
		reparsed.Merge(result)
	}
	kept := sel.keep(reparsed)
	return kept, total, len(kept.RepositoryDocs) + len(kept.FileDocs), nil
}
