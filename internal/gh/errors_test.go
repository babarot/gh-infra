package gh

import (
	"testing"
)

func TestTryParseAPIError(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantNil    bool
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "Not Found returns status 404",
			stderr:     `{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`,
			wantStatus: 404,
			wantMsg:    "Not Found",
		},
		{
			name:       "Forbidden returns status 403",
			stderr:     `{"message":"Forbidden","documentation_url":"https://docs.github.com/rest"}`,
			wantStatus: 403,
			wantMsg:    "Forbidden",
		},
		{
			name:       "Validation Failed returns status 422",
			stderr:     `{"message":"Validation Failed","errors":[{"message":"name already exists"}]}`,
			wantStatus: 422,
			wantMsg:    "Validation Failed",
		},
		{
			name:    "invalid JSON returns nil",
			stderr:  "this is not json at all",
			wantNil: true,
		},
		{
			name:    "empty message returns nil",
			stderr:  `{"message":""}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tryParseAPIError(tt.stderr)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil APIError, got nil")
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status: got %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("message: got %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}

func TestTryParseAPIError_ValidationErrors(t *testing.T) {
	stderr := `{"message":"Validation Failed","errors":[{"message":"name already exists"},{"message":"field is invalid"}]}`
	got := tryParseAPIError(stderr)
	if got == nil {
		t.Fatal("expected non-nil APIError")
	}
	if len(got.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(got.Errors))
	}
	if got.Errors[0] != "name already exists" {
		t.Errorf("errors[0]: got %q, want %q", got.Errors[0], "name already exists")
	}
	if got.Errors[1] != "field is invalid" {
		t.Errorf("errors[1]: got %q, want %q", got.Errors[1], "field is invalid")
	}
}

func TestTryParseAPIError_ErrorsAsString(t *testing.T) {
	// Real response from fork-pr-approval on private repos: errors is a plain string
	stderr := `{"message":"Validation Failed","errors":"Fork PR approval is not allowed for private repositories.","documentation_url":"https://docs.github.com/rest","status":"422"}`
	got := tryParseAPIError(stderr)
	if got == nil {
		t.Fatal("expected non-nil APIError")
	}
	if got.Status != 422 {
		t.Errorf("status: got %d, want 422", got.Status)
	}
	if got.Message != "Validation Failed" {
		t.Errorf("message: got %q, want %q", got.Message, "Validation Failed")
	}
	if len(got.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(got.Errors))
	}
	if got.Errors[0] != "Fork PR approval is not allowed for private repositories." {
		t.Errorf("errors[0]: got %q", got.Errors[0])
	}
}

func TestTryParseAPIError_JSONFollowedByText(t *testing.T) {
	// gh cli outputs JSON + human-readable text concatenated on stderr
	stderr := `{"message":"Validation Failed","errors":"some error","status":"422"}gh: some error (Validation Failed)`
	got := tryParseAPIError(stderr)
	if got == nil {
		t.Fatal("expected non-nil APIError")
	}
	if got.Status != 422 {
		t.Errorf("status: got %d, want 422", got.Status)
	}
}

func TestTryParseAPIError_StatusAsNumber(t *testing.T) {
	stderr := `{"message":"Validation Failed","status":422}`
	got := tryParseAPIError(stderr)
	if got == nil {
		t.Fatal("expected non-nil APIError")
	}
	if got.Status != 422 {
		t.Errorf("status: got %d, want 422", got.Status)
	}
}

func TestTryParseAPIError_GHMessageFormat(t *testing.T) {
	// When stderr has no JSON, only the gh cli human-readable format
	tests := []struct {
		name       string
		stderr     string
		wantNil    bool
		wantStatus int
	}{
		{
			name:       "Validation Failed from gh cli",
			stderr:     "gh: Fork PR approval is not allowed for private repositories. (Validation Failed)",
			wantStatus: 422,
		},
		{
			name:       "Not Found from gh cli",
			stderr:     "gh: Resource not accessible (Not Found)",
			wantStatus: 404,
		},
		{
			name:    "unrecognized label",
			stderr:  "gh: something happened (Unknown Error)",
			wantNil: true,
		},
		{
			name:    "no parentheses",
			stderr:  "gh: something went wrong",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tryParseAPIError(tt.stderr)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil APIError")
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status: got %d, want %d", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain JSON", `{"message":"Not Found"}`, `{"message":"Not Found"}`},
		{"JSON + trailing text", `{"message":"err"}gh: err`, `{"message":"err"}`},
		{"no JSON", "plain text", ""},
		{"nested braces", `{"a":{"b":"c"}}trailing`, `{"a":{"b":"c"}}`},
		{"brace in string value", `{"message":"value with } inside","status":"422"}`, `{"message":"value with } inside","status":"422"}`},
		{"escaped quote in string", `{"message":"say \"hello\"","status":"404"}`, `{"message":"say \"hello\"","status":"404"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExitError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  ExitError
		want string
	}{
		{
			name: "with APIError",
			err: ExitError{
				Cmd:      "repo edit owner/repo",
				ExitCode: 1,
				Stderr:   `{"message":"Not Found"}`,
				APIError: &APIError{Status: 404, Message: "Not Found"},
			},
			want: "Not Found (HTTP 404)",
		},
		{
			name: "without APIError",
			err: ExitError{
				Cmd:      "repo edit owner/repo",
				ExitCode: 1,
				Stderr:   "something went wrong",
			},
			want: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
