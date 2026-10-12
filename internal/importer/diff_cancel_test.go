package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// cancelOnFileRunner answers the repository fetch with empty state and, on
// the file fetch, cancels the run and fails as a killed gh process would.
type cancelOnFileRunner struct{ cancel context.CancelFunc }

func (r cancelOnFileRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	switch {
	case strings.Contains(cmd, "/contents/"):
		r.cancel()
		return nil, errors.New("signal: killed")
	case strings.HasPrefix(cmd, "repo view"):
		return []byte(`{"visibility":"PUBLIC"}`), nil
	case strings.Contains(cmd, "--jq {"), args[0] == "api" && !strings.Contains(cmd, "--jq"):
		return []byte("{}"), nil
	case strings.Contains(cmd, " list"):
		return []byte("[]"), nil
	}
	return nil, nil
}

func (r cancelOnFileRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

// An interrupt while fetching a file is told by ctx, not by the error of
// the killed gh process, so the target is not just skipped.
func TestDiff_CanceledWhileFetchingFiles(t *testing.T) {
	dir := t.TempDir()
	doc := "apiVersion: gh-infra/v1\nkind: FileSet\nmetadata:\n  owner: o\nspec:\n  repositories: [r]\n  files:\n    - path: f.txt\n      content: local\n"
	if err := os.WriteFile(filepath.Join(dir, "fs.yaml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.ParseAll(dir, manifest.ParseOptions{Resolver: &manifest.SourceResolver{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = Diff(ctx, DiffOptions{
		Targets:     []TargetMatches{{Target: Target{Owner: "o", Name: "r"}, Matches: FindMatches(parsed, "o/r")}},
		Runner:      cancelOnFileRunner{cancel: cancel},
		AllFileDocs: parsed.FileDocs,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
