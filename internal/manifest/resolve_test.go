package manifest

import (
	"context"
	"fmt"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
)

// ─── Forward resolution: ResolveBypassActors ───

func TestResolveBypassActors_RoleAdmin(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Role: "admin", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].ActorID != 5 {
		t.Errorf("ActorID = %d, want 5", got[0].ActorID)
	}
	if got[0].ActorType != "RepositoryRole" {
		t.Errorf("ActorType = %q, want RepositoryRole", got[0].ActorType)
	}
	if got[0].BypassMode != "always" {
		t.Errorf("BypassMode = %q, want always", got[0].BypassMode)
	}
}

func TestResolveBypassActors_RoleWrite(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Role: "write", BypassMode: "pull_request"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 4 {
		t.Errorf("ActorID = %d, want 4", got[0].ActorID)
	}
}

func TestResolveBypassActors_RoleMaintain(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Role: "maintain", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 2 {
		t.Errorf("ActorID = %d, want 2", got[0].ActorID)
	}
}

func TestResolveBypassActors_RoleInvalid(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Role: "invalid", BypassMode: "always"}}
	_, err := r.ResolveBypassActors(context.Background(), actors)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestResolveBypassActors_Team(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api orgs/myorg/teams/backend": []byte(`{"id":42}`),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Team: "backend", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 42 {
		t.Errorf("ActorID = %d, want 42", got[0].ActorID)
	}
	if got[0].ActorType != "Team" {
		t.Errorf("ActorType = %q, want Team", got[0].ActorType)
	}
}

func TestResolveBypassActors_TeamNotFound(t *testing.T) {
	mock := &gh.MockRunner{
		Errors: map[string]error{
			"api orgs/myorg/teams/ghost": fmt.Errorf("HTTP 404"),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Team: "ghost", BypassMode: "always"}}
	_, err := r.ResolveBypassActors(context.Background(), actors)
	if err == nil {
		t.Fatal("expected error for team not found")
	}
}

func TestResolveBypassActors_App(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{App: "github-actions", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 15368 {
		t.Errorf("ActorID = %d, want 15368", got[0].ActorID)
	}
	if got[0].ActorType != "Integration" {
		t.Errorf("ActorType = %q, want Integration", got[0].ActorType)
	}
}

func TestResolveBypassActors_AppUnknown(t *testing.T) {
	mock := &gh.MockRunner{
		Errors: map[string]error{
			"api apps/unknown-app": fmt.Errorf("HTTP 404"),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{App: "unknown-app", BypassMode: "always"}}
	_, err := r.ResolveBypassActors(context.Background(), actors)
	if err == nil {
		t.Fatal("expected error for unknown app")
	}
}

func TestResolveBypassActors_OrgAdmin(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{OrgAdmin: Ptr(true), BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 1 {
		t.Errorf("ActorID = %d, want 1", got[0].ActorID)
	}
	if got[0].ActorType != "OrganizationAdmin" {
		t.Errorf("ActorType = %q, want OrganizationAdmin", got[0].ActorType)
	}
}

func TestResolveBypassActors_CustomRole(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api orgs/myorg/custom-repository-roles": []byte(`{"custom_roles":[{"id":99,"name":"security-reviewer"}]}`),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{CustomRole: "security-reviewer", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 99 {
		t.Errorf("ActorID = %d, want 99", got[0].ActorID)
	}
	if got[0].ActorType != "RepositoryRole" {
		t.Errorf("ActorType = %q, want RepositoryRole", got[0].ActorType)
	}
}

func TestResolveBypassActors_CustomRoleNotFound(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api orgs/myorg/custom-repository-roles": []byte(`{"custom_roles":[{"id":99,"name":"other"}]}`),
		},
	}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{CustomRole: "no-such-role", BypassMode: "always"}}
	_, err := r.ResolveBypassActors(context.Background(), actors)
	if err == nil {
		t.Fatal("expected error for custom role not found")
	}
}

func TestResolveBypassActors_NoTypeSpecified(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{BypassMode: "always"}}
	_, err := r.ResolveBypassActors(context.Background(), actors)
	if err == nil {
		t.Fatal("expected error when no type is specified")
	}
}

func TestResolveBypassActors_MultipleTypesFirstWins(t *testing.T) {
	// When both Role and Team are set, the switch hits Role first.
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Role: "admin", Team: "backend", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Role branch is matched first
	if got[0].ActorType != "RepositoryRole" {
		t.Errorf("ActorType = %q, want RepositoryRole (role wins)", got[0].ActorType)
	}
}

// ─── Forward resolution: ResolveStatusChecks ───

func TestResolveStatusChecks_WithApp(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	r := NewResolver(mock, "myorg")

	checks := []RulesetStatusCheck{{Context: "ci/test", App: "github-actions"}}
	got, err := r.ResolveStatusChecks(context.Background(), checks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].IntegrationID != 15368 {
		t.Errorf("IntegrationID = %d, want 15368", got[0].IntegrationID)
	}
	if got[0].Context != "ci/test" {
		t.Errorf("Context = %q, want ci/test", got[0].Context)
	}
}

func TestResolveStatusChecks_WithoutApp(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	checks := []RulesetStatusCheck{{Context: "ci/test"}}
	got, err := r.ResolveStatusChecks(context.Background(), checks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].IntegrationID != 0 {
		t.Errorf("IntegrationID = %d, want 0", got[0].IntegrationID)
	}
}

func TestResolveStatusChecks_InvalidApp(t *testing.T) {
	mock := &gh.MockRunner{
		Errors: map[string]error{
			"api apps/bad-app": fmt.Errorf("HTTP 404"),
		},
	}
	r := NewResolver(mock, "myorg")

	checks := []RulesetStatusCheck{{Context: "ci/test", App: "bad-app"}}
	_, err := r.ResolveStatusChecks(context.Background(), checks)
	if err == nil {
		t.Fatal("expected error for invalid app in status check")
	}
}

// ─── ResolveAppID caching ───

func TestResolveAppID_Caching(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api apps/github-actions": []byte(`{"id":15368}`),
		},
	}
	r := NewResolver(mock, "myorg")

	id1, err := r.ResolveAppID(context.Background(), "github-actions")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	id2, err := r.ResolveAppID(context.Background(), "github-actions")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if id1 != id2 {
		t.Errorf("ids differ: %d vs %d", id1, id2)
	}
	// Only one API call should have been made
	if len(mock.Called) != 1 {
		t.Errorf("expected 1 API call, got %d", len(mock.Called))
	}
}

// ─── RoleNameFromID ───

func TestRoleNameFromID(t *testing.T) {
	tests := []struct {
		actorID   int
		actorType string
		want      string
	}{
		{5, "RepositoryRole", "admin"},
		{4, "RepositoryRole", "write"},
		{2, "RepositoryRole", "maintain"},
		{999, "RepositoryRole", ""},
		{5, "Team", ""},
	}
	for _, tt := range tests {
		got := RoleNameFromID(tt.actorID, tt.actorType)
		if got != tt.want {
			t.Errorf("RoleNameFromID(%d, %q) = %q, want %q", tt.actorID, tt.actorType, got, tt.want)
		}
	}
}

// ─── Reverse resolution: ReverseBypassActor ───

func TestReverseBypassActor_RepositoryRoleKnown(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 5, "RepositoryRole", "always", "myrepo")
	if got.Role != "admin" {
		t.Errorf("Role = %q, want admin", got.Role)
	}
	if got.BypassMode != "always" {
		t.Errorf("BypassMode = %q, want always", got.BypassMode)
	}
}

func TestReverseBypassActor_RepositoryRoleUnknown(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 999, "RepositoryRole", "always", "myrepo")
	if got.CustomRole != "id:999" {
		t.Errorf("CustomRole = %q, want id:999", got.CustomRole)
	}
}

func TestReverseBypassActor_Team(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api orgs/myorg/teams --paginate": []byte(`[{"id":42,"slug":"backend"},{"id":99,"slug":"frontend"}]`),
		},
	}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 42, "Team", "always", "myrepo")
	if got.Team != "backend" {
		t.Errorf("Team = %q, want backend", got.Team)
	}
}

func TestReverseBypassActor_Integration(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/commits/HEAD/check-runs --jq .check_runs": []byte(`[{"app":{"id":15368,"slug":"github-actions"}}]`),
		},
	}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 15368, "Integration", "always", "myrepo")
	if got.App != "github-actions" {
		t.Errorf("App = %q, want github-actions", got.App)
	}
}

func TestReverseBypassActor_OrganizationAdmin(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 1, "OrganizationAdmin", "always", "myrepo")
	if got.OrgAdmin == nil || !*got.OrgAdmin {
		t.Errorf("OrgAdmin = %v, want true", got.OrgAdmin)
	}
}

func TestReverseBypassActor_UnknownType(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	got := r.ReverseBypassActor(context.Background(), 1, "SomeNewType", "always", "myrepo")
	if got.Role != "unknown:SomeNewType:1" {
		t.Errorf("Role = %q, want unknown:SomeNewType:1", got.Role)
	}
}

// ─── Reverse resolution: ReverseStatusCheck ───

func TestReverseStatusCheck_IntegrationIDZero(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	got := r.ReverseStatusCheck(context.Background(), "ci/test", 0, "myrepo")
	if got.Context != "ci/test" {
		t.Errorf("Context = %q, want ci/test", got.Context)
	}
	if got.App != "" {
		t.Errorf("App = %q, want empty", got.App)
	}
}

func TestReverseStatusCheck_IntegrationIDNonZero(t *testing.T) {
	mock := &gh.MockRunner{
		Responses: map[string][]byte{
			"api repos/myorg/myrepo/commits/HEAD/check-runs --jq .check_runs": []byte(`[{"app":{"id":15368,"slug":"github-actions"}}]`),
		},
	}
	r := NewResolver(mock, "myorg")

	got := r.ReverseStatusCheck(context.Background(), "ci/test", 15368, "myrepo")
	if got.App != "github-actions" {
		t.Errorf("App = %q, want github-actions", got.App)
	}
}

func TestReverseStatusCheck_CheckRunsFails(t *testing.T) {
	mock := &gh.MockRunner{
		Errors: map[string]error{
			"api repos/myorg/myrepo/commits/HEAD/check-runs --jq .check_runs": fmt.Errorf("API error"),
		},
	}
	r := NewResolver(mock, "myorg")

	got := r.ReverseStatusCheck(context.Background(), "ci/test", 15368, "myrepo")
	if got.App != "id:15368" {
		t.Errorf("App = %q, want id:15368", got.App)
	}
}

// ─── id: prefix resolution (private/unresolvable entities) ───

func TestParseIDPrefix(t *testing.T) {
	tests := []struct {
		input string
		id    int
		ok    bool
	}{
		{"id:12345", 12345, true},
		{"id:0", 0, true},
		{"id:abc", 0, false},
		{"github-actions", 0, false},
		{"id:", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		id, ok := parseIDPrefix(tt.input)
		if ok != tt.ok || id != tt.id {
			t.Errorf("parseIDPrefix(%q) = (%d, %v), want (%d, %v)", tt.input, id, ok, tt.id, tt.ok)
		}
	}
}

func TestResolveAppID_IDPrefix(t *testing.T) {
	// No API call should be made — the mock has no responses
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	id, err := r.ResolveAppID(context.Background(), "id:12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12345 {
		t.Errorf("id = %d, want 12345", id)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(mock.Called))
	}
}

func TestResolveBypassActors_AppIDPrefix(t *testing.T) {
	// Simulate private app: import exported "id:99999", plan should resolve without API
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{App: "id:99999", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 99999 {
		t.Errorf("ActorID = %d, want 99999", got[0].ActorID)
	}
	if got[0].ActorType != "Integration" {
		t.Errorf("ActorType = %q, want Integration", got[0].ActorType)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(mock.Called))
	}
}

func TestResolveBypassActors_TeamIDPrefix(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{Team: "id:42", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 42 {
		t.Errorf("ActorID = %d, want 42", got[0].ActorID)
	}
	if got[0].ActorType != "Team" {
		t.Errorf("ActorType = %q, want Team", got[0].ActorType)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(mock.Called))
	}
}

func TestResolveBypassActors_CustomRoleIDPrefix(t *testing.T) {
	mock := &gh.MockRunner{}
	r := NewResolver(mock, "myorg")

	actors := []RulesetBypassActor{{CustomRole: "id:99", BypassMode: "always"}}
	got, err := r.ResolveBypassActors(context.Background(), actors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ActorID != 99 {
		t.Errorf("ActorID = %d, want 99", got[0].ActorID)
	}
	if len(mock.Called) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(mock.Called))
	}
}
