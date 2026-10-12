package infra

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// slowCreateRunner holds `gh repo create` until another command arrives or a
// timeout passes, and records the commands run before the create finished.
type slowCreateRunner struct {
	mu      sync.Mutex
	created bool
	early   []string
	other   chan struct{}
	once    sync.Once
}

func (r *slowCreateRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	if slices.Equal(args[:min(2, len(args))], []string{"repo", "create"}) {
		select {
		case <-r.other:
		case <-time.After(200 * time.Millisecond):
		}
		r.mu.Lock()
		r.created = true
		r.mu.Unlock()
		return nil, nil
	}
	r.mu.Lock()
	if !r.created {
		r.early = append(r.early, cmd)
		r.once.Do(func() { close(r.other) })
	}
	r.mu.Unlock()
	return nil, nil
}

func (r *slowCreateRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func TestApply_FileSetWaitsForRepositoryCreate(t *testing.T) {
	for _, stream := range []bool{false, true} {
		runner := &slowCreateRunner{other: make(chan struct{})}
		var out bytes.Buffer
		printer := ui.NewStandardPrinterWith(&out, &out)

		repo := &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Owner: "o", Name: "new"},
			Spec:     manifest.RepositorySpec{Visibility: manifest.Ptr("private")},
		}
		fs := &manifest.FileSet{
			Metadata: manifest.FileSetMetadata{Owner: "o", Name: "files"},
			Spec: manifest.FileSetSpec{
				Repositories: []manifest.FileSetRepository{{Name: "new"}},
				Via:          "push",
			},
		}
		result := &PlanResult{
			RepoChanges: []repository.Change{{
				Type: repository.ChangeCreate, Resource: manifest.ResourceRepository,
				Name: "o/new", Field: "repository", NewValue: "o/new",
			}},
			FileChanges: []fileset.Change{{
				FileSet: fs, FileSetID: fs.Identity(), Target: "o/new",
				Path: "README.md", Type: fileset.ChangeCreate, Desired: "hi\n", Via: "push",
			}},
			TargetRepos: []*manifest.Repository{repo},
			Parsed:      &manifest.ParseResult{Repositories: []*manifest.Repository{repo}, FileSets: []*manifest.FileSet{fs}},
			engine:      newEngine(runner, nil, printer),
		}

		_, _ = Apply(result, ApplyOptions{Stream: stream})

		if len(runner.early) > 0 {
			t.Errorf("stream=%v: commands ran before `gh repo create` finished: %q", stream, runner.early)
		}
	}
}
