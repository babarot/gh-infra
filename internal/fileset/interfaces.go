package fileset

import "time"

// ProgressReporter reports apply progress for file changes.
type ProgressReporter interface {
	Start(name string, fields []string)
	UpdateStatus(name, status string)
	Done(name string, elapsed time.Duration, count int)
	Error(name string, elapsed time.Duration, err error)
	// Step reports how one file of a repository ended, before Done or
	// Error reports the repository.
	Step(name string, outcome StepOutcome)
	// PullRequest reports the pull request opened for a repository.
	PullRequest(name, url string)
	Wait()
}

// StepOutcome is how one file ended, reported with Step.
type StepOutcome int

const (
	StepApplied StepOutcome = iota
	StepFailed
	StepSkipped // nothing to do on GitHub
)

// RefreshTracker reports plan-time refresh progress.
type RefreshTracker interface {
	UpdateStatus(name, status string)
	Done(name string, s PlanSummary)
	Error(name string, err error)
}

// PlanSummary is what one FileSet found for one repository, reported with
// Done.
type PlanSummary struct {
	Files int    // files with a change
	Via   string // how they are delivered; empty without changes
}

// SummarizePlan returns the PlanSummary of one repository's file changes.
func SummarizePlan(changes []Change) PlanSummary {
	var s PlanSummary
	for _, c := range changes {
		if c.Type == ChangeNoOp {
			continue
		}
		s.Files++
		if s.Via == "" {
			s.Via = c.Via
		}
	}
	return s
}

// ProgressWriter emits file apply fallback progress messages.
type ProgressWriter interface {
	Progress(msg string)
}

type noopProgressReporter struct{}

func (noopProgressReporter) Start(string, []string)             {}
func (noopProgressReporter) UpdateStatus(string, string)        {}
func (noopProgressReporter) Done(string, time.Duration, int)    {}
func (noopProgressReporter) Error(string, time.Duration, error) {}
func (noopProgressReporter) Step(string, StepOutcome)           {}
func (noopProgressReporter) Wait()                              {}
func (noopProgressReporter) PullRequest(string, string)         {}

type noopRefreshTracker struct{}

func (noopRefreshTracker) UpdateStatus(string, string) {}
func (noopRefreshTracker) Done(string, PlanSummary)    {}
func (noopRefreshTracker) Error(string, error)         {}

type noopProgressWriter struct{}

func (noopProgressWriter) Progress(string) {}
