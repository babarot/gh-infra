package importer

// DiagnosticPrinter reports target-level import warnings and errors.
type DiagnosticPrinter interface {
	Error(name, detail string)
	Warning(name, detail string)
}

// RefreshTracker reports import-time refresh progress.
type RefreshTracker interface {
	UpdateStatus(name, status string)
	Done(name string)
	Fail(name string)
}

type noopDiagnosticPrinter struct{}

func (noopDiagnosticPrinter) Error(string, string)   {}
func (noopDiagnosticPrinter) Warning(string, string) {}

type noopRefreshTracker struct{}

func (noopRefreshTracker) UpdateStatus(string, string) {}
func (noopRefreshTracker) Done(string)                 {}
func (noopRefreshTracker) Fail(string)                 {}
