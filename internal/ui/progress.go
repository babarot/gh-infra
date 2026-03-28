package ui

import (
	"fmt"
	"strings"
	"time"
)

// ProgressReporter abstracts how apply progress is reported.
// Implementations decide whether to use spinners, streaming lines, etc.
type ProgressReporter interface {
	Start(name string, fields []string)
	Done(name string, elapsed time.Duration, count int)
	Error(name string, elapsed time.Duration, err error)
	Wait()
}

// SpinnerReporter reports progress using animated spinners (bubbletea).
type SpinnerReporter struct {
	tracker *RefreshTracker
	tasks   map[string]RefreshTask // name → task (for key lookup)
	shared  bool                   // true when tracker is shared (don't call Wait)
}

// NewSpinnerReporter creates a spinner-based reporter for the given task names.
// Parts are joined with spaces to form labels: "verb name suffix" / "pastVerb name suffix".
func NewSpinnerReporter(names []string, verb, pastVerb, suffix string) *SpinnerReporter {
	tasks := make([]RefreshTask, len(names))
	taskMap := make(map[string]RefreshTask, len(names))
	for i, name := range names {
		label := verb + " " + name
		doneLabel := pastVerb + " " + name
		if suffix != "" {
			label += " " + suffix
			doneLabel += " " + suffix
		}
		t := RefreshTask{
			Name:      label,
			DoneLabel: doneLabel,
		}
		tasks[i] = t
		taskMap[name] = t
	}
	return &SpinnerReporter{
		tracker: RunRefresh(tasks),
		tasks:   taskMap,
	}
}

// NewSpinnerReporterWith creates a reporter that delegates to an existing tracker.
// Each name is mapped directly as a task key (no verb prefix).
func NewSpinnerReporterWith(tracker *RefreshTracker, names []string) *SpinnerReporter {
	taskMap := make(map[string]RefreshTask, len(names))
	for _, name := range names {
		taskMap[name] = RefreshTask{Name: name}
	}
	return &SpinnerReporter{
		tracker: tracker,
		tasks:   taskMap,
		shared:  true,
	}
}

func (r *SpinnerReporter) Start(name string, fields []string) {
	if t, ok := r.tasks[name]; ok && len(fields) > 0 {
		r.tracker.UpdateStatus(t.Name, "applying "+fields[0]+"...")
	}
}

func (r *SpinnerReporter) Done(name string, elapsed time.Duration, count int) {
	if t, ok := r.tasks[name]; ok {
		r.tracker.Done(t.Name)
	}
}

func (r *SpinnerReporter) Error(name string, elapsed time.Duration, err error) {
	if t, ok := r.tasks[name]; ok {
		r.tracker.Error(t.Name, err)
	}
}

func (r *SpinnerReporter) Canceled() <-chan struct{} {
	return r.tracker.Canceled()
}

func (r *SpinnerReporter) Wait() {
	if r.shared {
		return // shared tracker is waited on by the caller
	}
	r.tracker.Wait()
}

// StreamReporter reports progress as line-by-line streaming output.
type StreamReporter struct {
	printer Printer
	verb    string
	past    string
}

// NewStreamReporter creates a stream-based reporter.
func NewStreamReporter(printer Printer, verb, pastVerb string) *StreamReporter {
	return &StreamReporter{printer: printer, verb: verb, past: pastVerb}
}

func (r *StreamReporter) Start(name string, fields []string) {
	r.printer.StreamStart(name, fmt.Sprintf("%s... [%s]", r.verb, strings.Join(fields, ", ")))
}

func (r *StreamReporter) Done(name string, elapsed time.Duration, count int) {
	label := "change"
	if count != 1 {
		label = "changes"
	}
	r.printer.StreamDone(name, fmt.Sprintf("%s after %s [%d %s]", r.past, FormatDuration(elapsed), count, label))
}

func (r *StreamReporter) Error(name string, elapsed time.Duration, err error) {
	r.printer.StreamError(name, fmt.Sprintf("Error after %s: %s", FormatDuration(elapsed), err))
}

func (r *StreamReporter) Wait() {
	// Stream mode has no buffered state to flush.
}

// NoopReporter discards all progress events. Useful for tests.
type NoopReporter struct{}

func (NoopReporter) Start(string, []string)             {}
func (NoopReporter) Done(string, time.Duration, int)    {}
func (NoopReporter) Error(string, time.Duration, error) {}
func (NoopReporter) Wait()                              {}
