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
	Done(name string)
	Error(name string, err error)
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
func (noopRefreshTracker) Done(string)                 {}
func (noopRefreshTracker) Error(string, error)         {}
