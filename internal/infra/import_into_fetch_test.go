package infra

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
)

// importRunner answers import --into for repositories o/a and o/b: each
// has description "remote" and nothing else, and fetching its file f.txt
// returns what contents says (an error, or content when nil).
type importRunner struct {
	contents map[string]error // "o/a" → error from the contents API
	onFile   func(repo string)
	mu       sync.Mutex
}

func (r *importRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(cmd, "repo view"):
		return []byte(`{"description":"remote","visibility":"PUBLIC"}`), nil
	case strings.Contains(cmd, "/contents/f.txt"):
		for _, repo := range []string{"o/a", "o/b"} {
			if !strings.Contains(cmd, "repos/"+repo+"/") {
				continue
			}
			if r.onFile != nil {
				r.onFile(repo)
			}
			r.mu.Lock()
			err := r.contents[repo]
			r.mu.Unlock()
			if err != nil {
				return nil, err
			}
			enc := base64.StdEncoding.EncodeToString([]byte("remote\n"))
			return fmt.Appendf(nil, `{"content":%q,"encoding":"base64"}`, enc), nil
		}
	}
	// Everything else of the repository is empty.
	switch {
	case strings.Contains(cmd, "--jq {"):
		return []byte("{}"), nil
	case strings.Contains(cmd, "--jq"):
		return nil, nil
	case strings.Contains(cmd, " list"):
		return []byte("[]"), nil
	case args[0] == "api":
		return []byte("{}"), nil
	}
	return nil, nil
}

func (r *importRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func writeImportManifests(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	var docs []string
	for _, name := range []string{"a", "b"} {
		docs = append(docs, fmt.Sprintf("apiVersion: gh-infra/v1\nkind: Repository\nmetadata:\n  owner: o\n  name: %s\nspec:\n  description: local\n", name))
	}
	docs = append(docs, "apiVersion: gh-infra/v1\nkind: FileSet\nmetadata:\n  owner: o\nspec:\n  repositories: [a, b]\n  files:\n    - path: f.txt\n      content: local\n")
	if err := os.WriteFile(filepath.Join(dir, "m.yaml"), []byte(strings.Join(docs, "---\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// diffTargets returns the targets with repository diffs and with file changes.
func diffTargets(d *ImportDiff) (repos, files []string) {
	for _, fd := range d.Plan.RepoDiffs {
		repos = append(repos, fd.Target)
	}
	for _, c := range d.Plan.FileChanges {
		files = append(files, c.Target)
	}
	return uniqueStrings(repos), uniqueStrings(files)
}

// A file that cannot be read is not "no change": its repository is
// reported and left out as a whole, while the others are imported.
func TestImportInto_FileFetchErrorSkipsTarget(t *testing.T) {
	for _, err := range []error{errors.New("HTTP 502"), fmt.Errorf("contents: %w", gh.ErrForbidden), gh.ErrRateLimited} {
		runner := &importRunner{contents: map[string]error{"o/a": err}}
		d, ierr := ImportInto(context.Background(), runner, nil, writeImportManifests(t))
		if ierr != nil {
			t.Fatalf("%v: ImportInto: %v", err, ierr)
		}
		if d.Skipped != 1 {
			t.Errorf("%v: Skipped = %d, want 1", err, d.Skipped)
		}
		repos, files := diffTargets(d)
		if strings.Join(repos, ",") != "o/b" || strings.Join(files, ",") != "o/b" {
			t.Errorf("%v: repo diffs for %v, file changes for %v; want o/b only", err, repos, files)
		}
	}
}

func TestImportInto_MissingFileIsNoChange(t *testing.T) {
	runner := &importRunner{contents: map[string]error{"o/a": gh.ErrNotFound}}
	d, err := ImportInto(context.Background(), runner, nil, writeImportManifests(t))
	if err != nil {
		t.Fatal(err)
	}
	if d.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", d.Skipped)
	}
	repos, _ := diffTargets(d)
	if strings.Join(repos, ",") != "o/a,o/b" {
		t.Errorf("repo diffs for %v, want both", repos)
	}
}

func TestImportInto_UnauthorizedStops(t *testing.T) {
	runner := &importRunner{contents: map[string]error{"o/a": gh.ErrUnauthorized}}
	_, err := ImportInto(context.Background(), runner, nil, writeImportManifests(t))
	if !errors.Is(err, gh.ErrUnauthorized) {
		t.Fatalf("err = %v, want unauthorized", err)
	}
}

// Canceled while fetching the last target's file, by a gh process that
// fails with an error of its own.
func TestImportInto_CanceledWhileFetchingFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &importRunner{contents: map[string]error{"o/a": nil, "o/b": errors.New("signal: killed")}}
	runner.onFile = func(repo string) {
		if repo == "o/b" {
			cancel()
		}
	}
	_, err := ImportInto(ctx, runner, nil, writeImportManifests(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
