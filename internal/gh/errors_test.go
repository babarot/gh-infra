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
			want: "gh: repo edit owner/repo (HTTP 404: Not Found)",
		},
		{
			name: "without APIError",
			err: ExitError{
				Cmd:      "repo edit owner/repo",
				ExitCode: 1,
				Stderr:   "something went wrong",
			},
			want: "gh: repo edit owner/repo (exit 1): something went wrong",
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
