package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

const currentRulesetResp = `{
  "id": 7, "node_id": "RRS_x", "source": "o/r", "source_type": "Repository",
  "name": "main", "target": "branch", "enforcement": "evaluate",
  "bypass_actors": [{"actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always"}],
  "conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"], "exclude": []}},
  "rules": [
    {"type": "deletion"},
    {"type": "pull_request", "parameters": {"required_approving_review_count": 1, "dismiss_stale_reviews_on_push": true,
      "require_code_owner_review": false, "require_last_push_approval": false, "required_review_thread_resolution": false,
      "allowed_merge_methods": ["squash"]}},
    {"type": "commit_message_pattern", "parameters": {"operator": "starts_with", "pattern": "feat"}},
    {"type": "update", "parameters": {"update_allows_fetch_and_merge": true}}
  ]
}`

func updatePayload(t *testing.T, rs manifest.Ruleset) map[string]any {
	t.Helper()
	resolver := manifest.NewResolver(&gh.MockRunner{}, "o")
	payload, err := buildRulesetUpdatePayload(context.Background(), &rs, []byte(currentRulesetResp), resolver)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func rulesByType(t *testing.T, payload map[string]any) (types []string, byType map[string]map[string]any) {
	t.Helper()
	byType = map[string]map[string]any{}
	rules, ok := payload["rules"].([]any)
	if !ok {
		t.Fatalf("rules = %T", payload["rules"])
	}
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("rule = %T", r)
		}
		typ, _ := rule["type"].(string)
		types = append(types, typ)
		params, _ := rule["parameters"].(map[string]any)
		byType[typ] = params
	}
	return types, byType
}

func TestBuildRulesetUpdatePayload_KeepsWhatTheManifestLeavesOut(t *testing.T) {
	out := updatePayload(t, manifest.Ruleset{
		Name:  "main",
		Rules: manifest.RulesetRules{NonFastForward: manifest.Ptr(true)},
	})

	if out["target"] != "branch" || out["enforcement"] != "evaluate" {
		t.Errorf("target/enforcement = %v/%v, want the current branch/evaluate", out["target"], out["enforcement"])
	}
	for _, key := range []string{"id", "node_id", "source", "source_type"} {
		if _, ok := out[key]; ok {
			t.Errorf("read-only field %q must not be sent", key)
		}
	}
	if actors, _ := out["bypass_actors"].([]any); len(actors) != 1 {
		t.Errorf("bypass_actors = %v, want the current actor kept", out["bypass_actors"])
	}
	if out["conditions"] == nil {
		t.Error("conditions should be kept")
	}

	types, byType := rulesByType(t, out)
	want := []string{"deletion", "pull_request", "commit_message_pattern", "update", "non_fast_forward"}
	if len(types) != len(want) {
		t.Fatalf("rules = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("rules = %v, want %v (current rules kept in place, new ones appended)", types, want)
		}
	}
	if byType["commit_message_pattern"]["pattern"] != "feat" {
		t.Errorf("unmodelled rule changed: %v", byType["commit_message_pattern"])
	}
	if byType["update"]["update_allows_fetch_and_merge"] != true {
		t.Errorf("update parameter reset: %v", byType["update"])
	}
}

func TestBuildRulesetUpdatePayload_AppliesWhatTheManifestSets(t *testing.T) {
	out := updatePayload(t, manifest.Ruleset{
		Name:            "main",
		Enforcement:     manifest.Ptr("active"),
		BypassActorsSet: true,
		Rules: manifest.RulesetRules{
			Deletion: manifest.Ptr(false),
			PullRequest: &manifest.RulesetPullRequest{
				RequiredApprovingReviewCount: manifest.Ptr(2),
			},
			Update: &manifest.RulesetUpdate{Enabled: manifest.Ptr(false)},
		},
	})

	if out["enforcement"] != "active" {
		t.Errorf("enforcement = %v, want active", out["enforcement"])
	}
	if actors, ok := out["bypass_actors"].([]any); !ok || len(actors) != 0 {
		t.Errorf("bypass_actors = %v, want [] (explicitly set)", out["bypass_actors"])
	}
	types, byType := rulesByType(t, out)
	if len(types) != 2 || types[0] != "pull_request" || types[1] != "commit_message_pattern" {
		t.Fatalf("rules = %v, want deletion and update removed", types)
	}
	pr := byType["pull_request"]
	if pr["required_approving_review_count"] != float64(2) {
		t.Errorf("review count = %v, want 2", pr["required_approving_review_count"])
	}
	if pr["dismiss_stale_reviews_on_push"] != true {
		t.Errorf("unset parameter should keep its current value: %v", pr)
	}
	if methods, _ := pr["allowed_merge_methods"].([]any); len(methods) != 1 {
		t.Errorf("unmodelled parameter dropped: %v", pr)
	}
}

func TestBuildRulesetUpdatePayload_RemovesPullRequestWithFalse(t *testing.T) {
	out := updatePayload(t, manifest.Ruleset{
		Name:  "main",
		Rules: manifest.RulesetRules{PullRequest: &manifest.RulesetPullRequest{Disabled: true}},
	})
	types, _ := rulesByType(t, out)
	for _, typ := range types {
		if typ == "pull_request" {
			t.Fatalf("rules = %v, want pull_request removed", types)
		}
	}
}

func TestBuildRulesetUpdatePayload_NewPullRequestGetsDefaults(t *testing.T) {
	current := `{"name":"main","target":"branch","enforcement":"active","bypass_actors":[],"rules":[]}`
	rs := manifest.Ruleset{Name: "main", Rules: manifest.RulesetRules{
		PullRequest: &manifest.RulesetPullRequest{RequiredApprovingReviewCount: manifest.Ptr(1)},
	}}
	payload, err := buildRulesetUpdatePayload(context.Background(), &rs, []byte(current), nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := payload["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules = %v", payload["rules"])
	}
	rule, _ := rules[0].(map[string]any)
	params, _ := rule["parameters"].(map[string]any)
	if len(params) != 5 || params["require_code_owner_review"] != false || params["required_approving_review_count"] != 1 {
		t.Errorf("parameters = %v, want all five with defaults", params)
	}
}

func TestBuildRulesetPayload_CreateSkipsDisabledRules(t *testing.T) {
	rs := manifest.Ruleset{Name: "main", Rules: manifest.RulesetRules{
		PullRequest:          &manifest.RulesetPullRequest{Disabled: true},
		RequiredStatusChecks: &manifest.RulesetStatusChecks{Disabled: true},
		Deletion:             manifest.Ptr(true),
	}}
	payload, err := buildRulesetPayload(context.Background(), &rs, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := payload["rules"].([]map[string]any)
	if len(rules) != 1 || rules[0]["type"] != "deletion" {
		t.Errorf("rules = %v, want only deletion", rules)
	}
	if payload["enforcement"] != "active" || payload["target"] != "branch" {
		t.Errorf("create defaults changed: %v", payload)
	}
}
