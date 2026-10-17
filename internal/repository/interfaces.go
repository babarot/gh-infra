package repository

import "time"

// ProgressReporter reports apply progress for repository changes.
type ProgressReporter interface {
	Start(name string, fields []string)
	UpdateStatus(name, status string)
	Done(name string, elapsed time.Duration, count int)
	Error(name string, elapsed time.Duration, err error)
	Wait()
}

// RefreshTracker reports plan-time refresh progress.
type RefreshTracker interface {
	UpdateStatus(name, status string)
	Checkpoint(name, status string)
	Done(name string, s PlanSummary)
	Error(name string, err error)
}

// PlanSummary is what the plan of one repository found, reported with Done.
type PlanSummary struct {
	Creates, Updates, Deletes int  // counted as CountChanges counts them
	NewRepository             bool // the plan creates the repository
	DeleteRepository          bool // the plan deletes the repository
}

// SummarizePlan returns the PlanSummary of one repository's changes.
func SummarizePlan(changes []Change) PlanSummary {
	var s PlanSummary
	s.Creates, s.Updates, s.Deletes = CountChanges(changes)
	for _, c := range changes {
		if c.Field != "repository" {
			continue
		}
		switch c.Type {
		case ChangeCreate:
			s.NewRepository = true
		case ChangeDelete:
			s.DeleteRepository = true
		}
	}
	return s
}

type noopProgressReporter struct{}

func (noopProgressReporter) Start(string, []string)             {}
func (noopProgressReporter) UpdateStatus(string, string)        {}
func (noopProgressReporter) Done(string, time.Duration, int)    {}
func (noopProgressReporter) Error(string, time.Duration, error) {}
func (noopProgressReporter) Wait()                              {}

type noopRefreshTracker struct{}

func (noopRefreshTracker) UpdateStatus(string, string) {}
func (noopRefreshTracker) Checkpoint(string, string)   {}
func (noopRefreshTracker) Done(string, PlanSummary)    {}
func (noopRefreshTracker) Error(string, error)         {}
