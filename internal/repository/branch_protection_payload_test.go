package repository

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// orgProtectionResp is a GET branch protection response with everything
// set, in the shape GitHub returns for an organization repository.
const orgProtectionResp = `{
  "url": "https://api.github.com/repos/o/r/branches/main/protection",
  "required_status_checks": {
    "url": "x", "strict": true, "contexts": ["ci", "lint"], "contexts_url": "x",
    "checks": [{"context": "ci", "app_id": 15368}, {"context": "lint", "app_id": null}]
  },
  "required_pull_request_reviews": {
    "url": "x",
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "require_last_push_approval": true,
    "required_approving_review_count": 2,
    "dismissal_restrictions": {"url": "x", "users": [{"login": "alice", "id": 1}], "teams": [{"slug": "core", "id": 2}], "apps": []},
    "bypass_pull_request_allowances": {"users": [], "teams": [], "apps": [{"slug": "bot", "id": 3}]}
  },
  "restrictions": {"url": "x", "users": [{"login": "bob"}], "teams": [], "apps": []},
  "required_signatures": {"url": "x", "enabled": true},
  "enforce_admins": {"url": "x", "enabled": true},
  "required_linear_history": {"enabled": true},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false},
  "block_creations": {"enabled": true},
  "required_conversation_resolution": {"enabled": true},
  "lock_branch": {"enabled": false},
  "allow_fork_syncing": {"enabled": true}
}`

// userProtectionResp is what a personal repository returns: no restrictions
// and no dismissal restrictions.
const userProtectionResp = `{
  "required_status_checks": {"strict": false, "contexts": [], "checks": []},
  "required_pull_request_reviews": {"dismiss_stale_reviews": false, "require_code_owner_reviews": false, "require_last_push_approval": false, "required_approving_review_count": 1},
  "required_signatures": {"enabled": false},
  "enforce_admins": {"enabled": false},
  "required_linear_history": {"enabled": false},
  "allow_force_pushes": {"enabled": false},
  "allow_deletions": {"enabled": false},
  "block_creations": {"enabled": false},
  "required_conversation_resolution": {"enabled": false},
  "lock_branch": {"enabled": false},
  "allow_fork_syncing": {"enabled": false}
}`

func jsonRoundTrip(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProtectionPutForm(t *testing.T) {
	put, err := protectionPutForm([]byte(orgProtectionResp))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"required_status_checks": map[string]any{
			"strict": true,
			"checks": []any{
				map[string]any{"context": "ci", "app_id": float64(15368)},
				map[string]any{"context": "lint", "app_id": float64(-1)},
			},
		},
		"enforce_admins": true,
		"required_pull_request_reviews": map[string]any{
			"dismiss_stale_reviews":           true,
			"require_code_owner_reviews":      false,
			"require_last_push_approval":      true,
			"required_approving_review_count": float64(2),
			"dismissal_restrictions":          map[string]any{"users": []any{"alice"}, "teams": []any{"core"}, "apps": []any{}},
			"bypass_pull_request_allowances":  map[string]any{"users": []any{}, "teams": []any{}, "apps": []any{"bot"}},
		},
		"restrictions":                     map[string]any{"users": []any{"bob"}, "teams": []any{}, "apps": []any{}},
		"required_linear_history":          true,
		"allow_force_pushes":               false,
		"allow_deletions":                  false,
		"block_creations":                  true,
		"required_conversation_resolution": true,
		"lock_branch":                      false,
		"allow_fork_syncing":               true,
	}
	if got := jsonRoundTrip(t, put); !reflect.DeepEqual(got, want) {
		t.Errorf("put form =\n%v\nwant\n%v", got, want)
	}
}

func TestProtectionPutForm_PersonalRepository(t *testing.T) {
	put, err := protectionPutForm([]byte(userProtectionResp))
	if err != nil {
		t.Fatal(err)
	}
	got := jsonRoundTrip(t, put)
	if got["restrictions"] != nil {
		t.Errorf("restrictions = %v, want null", got["restrictions"])
	}
	reviews, _ := got["required_pull_request_reviews"].(map[string]any)
	if _, ok := reviews["dismissal_restrictions"]; ok {
		t.Error("dismissal_restrictions must be left out when GitHub did not return it")
	}
	if _, ok := reviews["bypass_pull_request_allowances"]; ok {
		t.Error("bypass_pull_request_allowances must be left out when GitHub did not return it")
	}
}

func TestBuildBranchProtectionUpdatePayload(t *testing.T) {
	t.Run("only set fields change", func(t *testing.T) {
		payload, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{
			Pattern:         "main",
			RequiredReviews: manifest.Ptr(3),
			AllowDeletions:  manifest.Ptr(true),
		}, []byte(orgProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		got := jsonRoundTrip(t, payload)
		reviews, _ := got["required_pull_request_reviews"].(map[string]any)
		if reviews["required_approving_review_count"] != float64(3) || reviews["dismiss_stale_reviews"] != true || reviews["require_last_push_approval"] != true {
			t.Errorf("reviews = %v, want count 3 and the other review settings kept", reviews)
		}
		if _, ok := reviews["dismissal_restrictions"]; !ok {
			t.Error("dismissal_restrictions dropped")
		}
		if got["allow_deletions"] != true || got["enforce_admins"] != true || got["lock_branch"] != false || got["block_creations"] != true {
			t.Errorf("settings = %v", got)
		}
		checks, _ := got["required_status_checks"].(map[string]any)
		if _, ok := checks["contexts"]; ok {
			t.Error("contexts must not be sent with checks")
		}
		if list, _ := checks["checks"].([]any); len(list) != 2 {
			t.Errorf("status checks = %v, want the current checks kept", checks)
		}
		if got["restrictions"] == nil {
			t.Error("restrictions dropped")
		}
	})

	t.Run("status checks without contexts keep the current checks", func(t *testing.T) {
		payload, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{
			Pattern:             "main",
			RequireStatusChecks: &manifest.StatusChecks{Strict: false},
		}, []byte(orgProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		checks, _ := jsonRoundTrip(t, payload)["required_status_checks"].(map[string]any)
		if checks["strict"] != false {
			t.Errorf("strict = %v, want false", checks["strict"])
		}
		if list, _ := checks["checks"].([]any); len(list) != 2 {
			t.Errorf("checks = %v, want kept", checks)
		}
	})

	t.Run("contexts replace the checks", func(t *testing.T) {
		payload, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{
			Pattern:             "main",
			RequireStatusChecks: &manifest.StatusChecks{Strict: true, Contexts: []string{"build"}},
		}, []byte(orgProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		checks, _ := jsonRoundTrip(t, payload)["required_status_checks"].(map[string]any)
		if _, ok := checks["checks"]; ok {
			t.Error("checks must not be sent with contexts")
		}
		if ctx, _ := checks["contexts"].([]any); len(ctx) != 1 || ctx[0] != "build" {
			t.Errorf("contexts = %v", checks["contexts"])
		}
	})

	t.Run("restrict_pushes", func(t *testing.T) {
		off, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{Pattern: "main", RestrictPushes: manifest.Ptr(false)}, []byte(orgProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		if off["restrictions"] != nil {
			t.Errorf("restrict_pushes: false should remove restrictions, got %v", off["restrictions"])
		}
		on, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{Pattern: "main", RestrictPushes: manifest.Ptr(true)}, []byte(userProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		if on["restrictions"] == nil {
			t.Error("restrict_pushes: true should add empty restrictions")
		}
	})

	t.Run("manifest matching the current protection changes nothing", func(t *testing.T) {
		payload, err := buildBranchProtectionUpdatePayload(&manifest.BranchProtection{
			Pattern:                 "main",
			RequiredReviews:         manifest.Ptr(1),
			DismissStaleReviews:     manifest.Ptr(false),
			RequireCodeOwnerReviews: manifest.Ptr(false),
			EnforceAdmins:           manifest.Ptr(false),
			AllowForcePushes:        manifest.Ptr(false),
			AllowDeletions:          manifest.Ptr(false),
		}, []byte(userProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		want, err := protectionPutForm([]byte(userProtectionResp))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(jsonRoundTrip(t, payload), jsonRoundTrip(t, want)) {
			t.Errorf("payload = %v, want the current protection", payload)
		}
	})
}

func TestBuildBranchProtectionPayload_CreateRestrictPushes(t *testing.T) {
	payload := buildBranchProtectionPayload(&manifest.BranchProtection{Pattern: "main", RestrictPushes: manifest.Ptr(true)})
	if payload["restrictions"] == nil {
		t.Error("restrict_pushes: true should send empty restrictions on create")
	}
	if payload["required_status_checks"] != nil || payload["required_pull_request_reviews"] != nil {
		t.Errorf("create payload = %v", payload)
	}
}
