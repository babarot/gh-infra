package importer

// RefreshTracker reports import-time refresh progress.
type RefreshTracker interface {
	UpdateStatus(name, status string)
	Checkpoint(name, status string)
	Done(name string)
	Fail(name string)
	Error(name string, err error)
}

type noopRefreshTracker struct{}

func (noopRefreshTracker) UpdateStatus(string, string) {}
func (noopRefreshTracker) Checkpoint(string, string)   {}
func (noopRefreshTracker) Done(string)                 {}
func (noopRefreshTracker) Fail(string)                 {}
func (noopRefreshTracker) Error(string, error)         {}
