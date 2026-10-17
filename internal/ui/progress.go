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
	UpdateStatus(name, status string)
	Done(name string, elapsed time.Duration, count int)
	Error(name string, elapsed time.Duration, err error)
	Wait()
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

func (r *StreamReporter) UpdateStatus(string, string) {}

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
	// One line per event: several errors (errors.Join) are joined with "; ".
	msg := strings.ReplaceAll(err.Error(), "\n", "; ")
	r.printer.StreamError(name, fmt.Sprintf("Error after %s: %s", FormatDuration(elapsed), msg))
}

func (r *StreamReporter) Wait() {
	// Stream mode has no buffered state to flush.
}
