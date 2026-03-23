package gh

import (
	"testing"
)

func TestNewRunner(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		r := NewRunner(false)
		if r.DryRun {
			t.Error("expected DryRun=false")
		}
		if r.MaxRetries != defaultMaxRetries {
			t.Errorf("MaxRetries: got %d, want %d", r.MaxRetries, defaultMaxRetries)
		}
	})

	t.Run("dry run enabled", func(t *testing.T) {
		r := NewRunner(true)
		if !r.DryRun {
			t.Error("expected DryRun=true")
		}
	})
}

func TestGHRunner_DryRun(t *testing.T) {
	r := NewRunner(true)
	out, err := r.Run("repo", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output in dry-run, got %v", out)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			name:   "timeout is retryable",
			err:    &ExitError{Stderr: "connection timeout reached"},
			expect: true,
		},
		{
			name:   "connection reset is retryable",
			err:    &ExitError{Stderr: "connection reset by peer"},
			expect: true,
		},
		{
			name:   "connection refused is retryable",
			err:    &ExitError{Stderr: "dial tcp: connection refused"},
			expect: true,
		},
		{
			name:   "tls handshake is retryable",
			err:    &ExitError{Stderr: "TLS handshake timeout"},
			expect: true,
		},
		{
			name:   "rate limit is retryable",
			err:    &ExitError{Stderr: "API rate limit exceeded"},
			expect: true,
		},
		{
			name:   "abuse detection is retryable",
			err:    &ExitError{Stderr: "abuse detection mechanism triggered"},
			expect: true,
		},
		{
			name:   "eof is retryable",
			err:    &ExitError{Stderr: "unexpected EOF"},
			expect: true,
		},
		{
			name:   "broken pipe is retryable",
			err:    &ExitError{Stderr: "write: broken pipe"},
			expect: true,
		},
		{
			name:   "not found is not retryable",
			err:    &ExitError{Stderr: `{"message":"Not Found"}`},
			expect: false,
		},
		{
			name:   "validation error is not retryable",
			err:    &ExitError{Stderr: `{"message":"Validation Failed"}`},
			expect: false,
		},
		{
			name:   "generic error is not retryable",
			err:    &ExitError{Stderr: "something went wrong"},
			expect: false,
		},
		{
			name:   "non-ExitError is not retryable",
			err:    ErrNotInstalled,
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.expect {
				t.Errorf("isRetryable: got %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		expect string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			max:    10,
			expect: "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			max:    5,
			expect: "hello",
		},
		{
			name:   "long string truncated",
			input:  "hello world",
			max:    5,
			expect: "hello...",
		},
		{
			name:   "empty string",
			input:  "",
			max:    5,
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			if got != tt.expect {
				t.Errorf("truncate(%q, %d): got %q, want %q", tt.input, tt.max, got, tt.expect)
			}
		})
	}
}

func TestMockRunner_Implements_Runner(t *testing.T) {
	var _ Runner = &MockRunner{}
	var _ Runner = &GHRunner{}
}
