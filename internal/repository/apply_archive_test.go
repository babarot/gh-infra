package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
)

type archiveRetryRunner struct {
	gh.MockRunner
	failures int
	err      error
	calls    [][]string
	cancel   context.CancelFunc
}

func (r *archiveRetryRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, args)
	if r.cancel != nil {
		r.cancel()
	}
	if len(r.calls) <= r.failures {
		return nil, r.err
	}
	return nil, nil
}

func TestApplyArchiveTransientError(t *testing.T) {
	transient := &gh.ExitError{ExitCode: 1, Stderr: "GraphQL: An error occurred when trying to archive repository R_kgDOexample. (archiveRepository)\n"}
	tests := []struct {
		name      string
		failures  int
		err       error
		archived  bool
		wantCalls int
		wantErr   bool
	}{
		{"success", 0, nil, true, 1, false},
		{"transient then success", 1, transient, true, 2, false},
		{"wrapped transient then success", 1, fmt.Errorf("command failed: %w", transient), true, 2, false},
		{"transient exhausted", 5, transient, true, 3, true},
		{"authorization is not retried", 5, gh.ErrForbidden, true, 1, true},
		{"other GraphQL failure is not retried", 5, &gh.ExitError{ExitCode: 1, Stderr: "GraphQL: Resource not accessible by integration (archiveRepository)"}, true, 1, true},
		{"plain error is not retried", 5, errors.New(transient.Error()), true, 1, true},
		{"unarchive is not retried", 5, transient, false, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner := &archiveRetryRunner{failures: tt.failures, err: tt.err}
			p := NewProcessor(runner, nil)
			err := p.applyRepoSetting(context.Background(), Change{Field: "archived", NewValue: tt.archived}, newTestRepo("owner", "repo"))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if len(runner.calls) != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", len(runner.calls), tt.wantCalls)
			}
			action := "archive"
			if !tt.archived {
				action = "unarchive"
			}
			for _, args := range runner.calls {
				if !reflect.DeepEqual(args, []string{"repo", action, "owner/repo", "--yes"}) {
					t.Fatalf("unexpected command: %v", args)
				}
			}
			if tt.wantErr && !errors.Is(err, tt.err) {
				t.Fatalf("original error lost: %v", err)
			}
		})
	}
}

func TestApplyArchiveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &archiveRetryRunner{failures: 5, err: &gh.ExitError{Stderr: "GraphQL: An error occurred when trying to archive repository R_kgDOexample. (archiveRepository)"}, cancel: cancel}
	err := NewProcessor(runner, nil).applyRepoSetting(ctx, Change{Field: "archived", NewValue: true}, newTestRepo("owner", "repo"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls after cancellation: %d", len(runner.calls))
	}
}
