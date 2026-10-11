package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/infra"
)

func TestFinishPlan_ExitCodes(t *testing.T) {
	noChanges := &infra.PlanResult{}
	changes := &infra.PlanResult{HasChanges: true}
	failed := &infra.PlanResult{HasChanges: true, FailedTargets: []string{"org/repo"}}
	other := errors.New("boom")

	detailed := planCommandOptions{DetailedExitCode: true}
	ci := planCommandOptions{CI: true}
	both := planCommandOptions{CI: true, DetailedExitCode: true}
	none := planCommandOptions{}
	jsonDetailed := planCommandOptions{DetailedExitCode: true, Output: infra.OutputJSON}

	for _, tt := range []struct {
		name   string
		result *infra.PlanResult
		err    error
		opts   planCommandOptions
		want   int // exit status main would use: 0, or 1 for a plain error
	}{
		{"detailed: no changes", noChanges, nil, detailed, 0},
		{"detailed: changes", changes, nil, detailed, 2},
		{"detailed: json changes", changes, nil, jsonDetailed, 2},
		{"detailed: failed targets win over changes", failed, nil, detailed, 1},
		{"detailed: error", nil, other, detailed, 1},
		{"detailed: interrupted", nil, context.Canceled, detailed, 1},
		{"detailed: no resources", nil, fmt.Errorf("wrap: %w", infra.ErrNoResources), detailed, 1},
		{"ci: changes", changes, nil, ci, 1},
		{"ci: no changes", noChanges, nil, ci, 0},
		{"ci: interrupted", nil, context.Canceled, ci, 0},
		{"ci: no resources", nil, infra.ErrNoResources, ci, 0},
		{"both: changes", changes, nil, both, 2},
		{"none: changes", changes, nil, none, 0},
		{"none: failed targets", failed, nil, none, 1},
		{"none: error", nil, other, none, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := finishPlan(tt.result, tt.err, tt.opts)
			got := 0
			if err != nil {
				got = 1
				if code, ok := ExitCode(err); ok {
					got = code
				}
			}
			if got != tt.want {
				t.Errorf("exit = %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
}

func TestPlanCmd_CIDeprecated(t *testing.T) {
	f := newPlanCmd().Flags().Lookup("ci")
	if f == nil || f.Deprecated == "" || !f.Hidden {
		t.Fatalf("expected --ci to be deprecated and hidden, got %+v", f)
	}
}

func TestFinishPlan_NoResourcesIsNotInterrupted(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	_ = finishPlan(nil, infra.ErrNoResources, planCommandOptions{})
	os.Stderr = orig
	w.Close()
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "Interrupted") {
		t.Errorf("no resources should not print Interrupted, got %q", out)
	}
}
