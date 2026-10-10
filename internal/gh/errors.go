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
		return fmt.Sprintf("%s (HTTP %d)", e.APIError.summary(), e.APIError.Status)
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
	Status           int
	Message          string
	Errors           []string
	DocumentationURL string
}

// summary returns the message and the error details on one line. GitHub
// puts some details in the message after a blank line, and others in the
// errors field, so both are kept.
func (a *APIError) summary() string {
	msg := strings.Join(strings.Fields(a.Message), " ")
	if a.DocumentationURL != "" {
		msg = strings.ReplaceAll(msg, "See `documentation_url`.", "See "+a.DocumentationURL+".")
	}
	var details []string
	for _, e := range a.Errors {
		if e != "" && !strings.Contains(msg, e) {
			details = append(details, e)
		}
	}
	if len(details) > 0 {
		msg += ": " + strings.Join(details, "; ")
	}
	return msg
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
// gh cli may output stderr as JSON followed by a human-readable line
// (e.g., `{"message":"..."}gh: ... (Validation Failed)`), so we extract
// the JSON prefix before parsing.
func tryParseAPIError(stderr string) *APIError {
	jsonStr := extractJSON(stderr)
	if jsonStr == "" {
		// No JSON found — try to parse the gh cli human-readable format:
		// "gh: <message> (<Status Label>)"
		return tryParseGHMessage(stderr)
	}

	// Parse with errors as an array of objects (common case).
	var raw struct {
		Message          string `json:"message"`
		Status           any    `json:"status"` // may be int or string
		Errors           json.RawMessage
		DocumentationURL string `json:"documentation_url"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}
	if raw.Message == "" {
		return nil
	}

	apiErr := &APIError{
		Message:          raw.Message,
		DocumentationURL: raw.DocumentationURL,
		Errors:           parseErrorDetails(raw.Errors),
	}

	// Infer HTTP status from the status field or message.
	apiErr.Status = inferHTTPStatus(raw.Status, raw.Message)

	return apiErr
}

// parseErrorDetails reads the errors field of an API error response, which
// may be a string, an array of strings, or an array of objects. Objects
// without a message, such as {"resource":"Label","field":"name",
// "code":"already_exists"}, are described by their resource, field and code.
func parseErrorDetails(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var details []string
	for _, item := range items {
		var str string
		if json.Unmarshal(item, &str) == nil {
			if str != "" {
				details = append(details, str)
			}
			continue
		}
		var obj struct {
			Message  string `json:"message"`
			Resource string `json:"resource"`
			Field    string `json:"field"`
			Code     string `json:"code"`
		}
		if json.Unmarshal(item, &obj) != nil {
			continue
		}
		switch {
		case obj.Message != "":
			details = append(details, obj.Message)
		case obj.Code != "":
			subject := strings.Trim(obj.Resource+"."+obj.Field, ".")
			if subject == "" {
				details = append(details, obj.Code)
			} else {
				details = append(details, subject+": "+obj.Code)
			}
		}
	}
	return details
}

// extractJSON returns the leading JSON object from s, or "" if none found.
// This handles the case where gh cli appends a human-readable line after the JSON.
// It correctly skips braces inside JSON string literals.
func extractJSON(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// inferHTTPStatus determines the HTTP status code from the API response.
// It first checks the status field (which may be an int or string like "422"),
// then falls back to inferring from the message text.
func inferHTTPStatus(status any, message string) int {
	// Try to use the explicit status field.
	switch v := status.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		if n := parseStatusString(v); n > 0 {
			return n
		}
	}

	// Fall back to message-based inference.
	// gh cli may report only a human-readable suffix such as "(HTTP 403)"
	// instead of a symbolic label like "(Forbidden)", so handle both forms.
	switch {
	case strings.Contains(message, "HTTP 404"):
		return 404
	case strings.Contains(message, "HTTP 401"):
		return 401
	case strings.Contains(message, "HTTP 403"):
		return 403
	case strings.Contains(message, "HTTP 422"):
		return 422
	case strings.Contains(message, "Not Found"):
		return 404
	case strings.Contains(message, "Unauthorized"):
		return 401
	case strings.Contains(message, "Forbidden"):
		return 403
	case strings.Contains(message, "Validation Failed"):
		return 422
	}
	return 0
}

func parseStatusString(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		return n
	}
	return 0
}

// tryParseGHMessage parses the gh cli human-readable error format:
// "gh: <detail> (<Status Label>)"
// e.g., "gh: Fork PR approval is not allowed for private repositories. (Validation Failed)"
func tryParseGHMessage(stderr string) *APIError {
	// Extract the parenthesized status label at the end.
	lastOpen := strings.LastIndex(stderr, "(")
	lastClose := strings.LastIndex(stderr, ")")
	if lastOpen < 0 || lastClose < 0 || lastClose <= lastOpen {
		return nil
	}
	label := stderr[lastOpen+1 : lastClose]

	// Extract the detail message (between "gh: " prefix and the label).
	msg := stderr
	if idx := strings.Index(msg, "gh: "); idx >= 0 {
		msg = msg[idx+4:]
	}
	if idx := strings.LastIndex(msg, "("); idx > 0 {
		msg = strings.TrimSpace(msg[:idx])
	}

	status := inferHTTPStatus(nil, label)
	if status == 0 {
		return nil
	}

	apiErr := &APIError{
		Message: label,
		Status:  status,
	}
	if msg != "" && msg != label {
		apiErr.Errors = append(apiErr.Errors, msg)
	}
	return apiErr
}
