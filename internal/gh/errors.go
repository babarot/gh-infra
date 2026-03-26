package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ExitError is returned when the gh command exits with a non-zero status.
type ExitError struct {
	Cmd      string
	ExitCode int
	Stderr   string
	APIError *APIError
}

func (e *ExitError) Error() string {
	if e.APIError != nil {
		return fmt.Sprintf("%s (HTTP %d)", e.APIError.Message, e.APIError.Status)
	}
	// Non-API errors: show stderr without the full command
	stderr := strings.TrimPrefix(e.Stderr, "gh: ")
	if stderr != "" {
		return stderr
	}
	return fmt.Sprintf("gh exited with code %d", e.ExitCode)
}

// APIError represents a parsed GitHub API error response.
type APIError struct {
	Status  int
	Message string
	Errors  []string
}

var (
	ErrNotFound     = errors.New("gh: not found (404)")
	ErrUnauthorized = errors.New("gh: unauthorized (401)")
	ErrForbidden    = errors.New("gh: forbidden (403)")
	ErrValidation   = errors.New("gh: validation failed (422)")
	ErrNotInstalled = errors.New("gh: command not found")
	ErrNotAuthed    = errors.New("gh: not authenticated, run 'gh auth login'")
)

// tryParseAPIError attempts to parse stderr as a GitHub API JSON error.
func tryParseAPIError(stderr string) *APIError {
	// gh api outputs JSON errors like: {"message":"Not Found","documentation_url":"..."}
	var raw struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stderr), &raw); err != nil {
		return nil
	}
	if raw.Message == "" {
		return nil
	}

	apiErr := &APIError{
		Message: raw.Message,
	}
	for _, e := range raw.Errors {
		apiErr.Errors = append(apiErr.Errors, e.Message)
	}

	// Infer HTTP status from message
	switch {
	case strings.Contains(raw.Message, "Not Found"):
		apiErr.Status = 404
	case strings.Contains(raw.Message, "Unauthorized"):
		apiErr.Status = 401
	case strings.Contains(raw.Message, "Forbidden"):
		apiErr.Status = 403
	case strings.Contains(raw.Message, "Validation Failed"):
		apiErr.Status = 422
	}

	return apiErr
}
