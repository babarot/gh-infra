package fileset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
)

// modeLookup returns the file modes of a repository at one commit. The
// Contents API does not report modes, so they are read from the tree of each
// file's parent directory, fetching every directory once.
type modeLookup struct {
	p      *Processor
	repo   string
	commit string // empty for a repository with no commits
	dirs   map[string]map[string]string
}

// newModeLookup returns a lookup at the HEAD of the default branch.
func (p *Processor) newModeLookup(ctx context.Context, repo string) (*modeLookup, error) {
	sha, _, err := p.getHeadSHA(ctx, repo)
	if err != nil {
		if isEmptyRepoErr(err) {
			return p.modeLookupAt(repo, ""), nil
		}
		return nil, fmt.Errorf("get HEAD: %w", err)
	}
	return p.modeLookupAt(repo, sha), nil
}

func (p *Processor) modeLookupAt(repo, commit string) *modeLookup {
	return &modeLookup{p: p, repo: repo, commit: commit, dirs: make(map[string]map[string]string)}
}

func (m *modeLookup) empty() bool {
	return m.commit == ""
}

// mode returns the mode of the entry at filePath, or "" if there is none.
func (m *modeLookup) mode(ctx context.Context, filePath string) (string, error) {
	if m.empty() {
		return "", nil
	}
	dir, name := path.Split(filePath)
	dir = strings.TrimSuffix(dir, "/")
	entries, ok := m.dirs[dir]
	if !ok {
		var err error
		entries, err = m.fetchDir(ctx, dir)
		if err != nil {
			return "", err
		}
		m.dirs[dir] = entries
	}
	return entries[name], nil
}

func (m *modeLookup) fetchDir(ctx context.Context, dir string) (map[string]string, error) {
	treeish := m.commit
	if dir != "" {
		segments := strings.Split(dir, "/")
		for i, s := range segments {
			segments[i] = url.PathEscape(s)
		}
		treeish += ":" + strings.Join(segments, "/")
	}
	out, err := m.p.runner.Run(ctx, "api", fmt.Sprintf("repos/%s/git/trees/%s", m.repo, treeish))
	if errors.Is(err, gh.ErrNotFound) {
		// The directory does not exist at this commit.
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tree of %q: %w", dir, err)
	}
	var raw struct {
		Tree []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse tree of %q: %w", dir, err)
	}
	entries := make(map[string]string, len(raw.Tree))
	for _, e := range raw.Tree {
		entries[e.Path] = e.Mode
	}
	return entries, nil
}

// resolveMode fills in the current mode of a file whose `executable` is set,
// and turns a NoOp into an update when only the mode differs.
func resolveMode(ctx context.Context, modes *modeLookup, c *Change) error {
	if c.Mode == "" || c.Type == ChangeDelete {
		return nil
	}
	if c.Type == ChangeCreate {
		// The Contents API used for empty repositories cannot set a mode.
		if modes.empty() && c.ModeChanged() {
			return errExecutableInEmptyRepo(c.Path)
		}
		return nil
	}

	mode, err := modes.mode(ctx, c.Path)
	if err != nil {
		return err
	}
	switch mode {
	case ModeFile, ModeExecutable:
	case "":
		return fmt.Errorf("%s: not found in the tree at HEAD", c.Path)
	default:
		return fmt.Errorf("%s: executable can only be set on regular files, but it has mode %s on GitHub", c.Path, mode)
	}
	c.CurrentMode = mode
	if c.Type == ChangeNoOp && c.Mode != mode {
		c.Type = ChangeUpdate
	}
	return nil
}

func isEmptyRepoErr(err error) bool {
	return strings.Contains(err.Error(), "repository is empty")
}

func errExecutableInEmptyRepo(path string) error {
	return fmt.Errorf("%s: executable files cannot be created in an empty repository; create the first commit without executable files", path)
}
