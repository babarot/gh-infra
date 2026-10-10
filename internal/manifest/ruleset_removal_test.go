package manifest

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestRuleset_RemovalForms(t *testing.T) {
	src := `
- name: main
  bypass_actors: []
  rules:
    pull_request: false
    required_status_checks: false
- name: other
  rules:
    pull_request:
      required_approving_review_count: 1
`
	var rs []Ruleset
	if err := yaml.UnmarshalWithOptions([]byte(src), &rs, yaml.DisallowUnknownField()); err != nil {
		t.Fatal(err)
	}
	if !rs[0].BypassActorsSet || !rs[0].HasBypassActors() || len(rs[0].BypassActors) != 0 {
		t.Errorf("bypass_actors: [] should be tracked as set: %+v", rs[0])
	}
	if !rs[0].Rules.PullRequestDisabled() || rs[0].Rules.PullRequestEnabled() {
		t.Errorf("pull_request: false should disable the rule: %+v", rs[0].Rules.PullRequest)
	}
	if !rs[0].Rules.StatusChecksDisabled() {
		t.Errorf("required_status_checks: false should disable the rule: %+v", rs[0].Rules.RequiredStatusChecks)
	}
	if rs[1].BypassActorsSet || rs[1].HasBypassActors() {
		t.Error("omitted bypass_actors should not be set")
	}
	if !rs[1].Rules.PullRequestEnabled() || *rs[1].Rules.PullRequest.RequiredApprovingReviewCount != 1 {
		t.Errorf("object form should enable the rule: %+v", rs[1].Rules.PullRequest)
	}

	out, err := yaml.Marshal(rs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bypass_actors: []", "pull_request: false", "required_status_checks: false", "required_approving_review_count: 1"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("marshal lost %q:\n%s", want, out)
		}
	}
	if strings.Index(string(out), "bypass_actors") > strings.Index(string(out), "rules:") {
		t.Errorf("bypass_actors should stay before rules:\n%s", out)
	}
}

func TestRuleset_RemovalFormsRejectTrue(t *testing.T) {
	for _, src := range []string{
		"- name: x\n  rules:\n    pull_request: true\n",
		"- name: x\n  rules:\n    required_status_checks: true\n",
		"- name: x\n  bypass_actors:\n  rules: {}\n",
	} {
		var rs []Ruleset
		if err := yaml.Unmarshal([]byte(src), &rs); err == nil {
			t.Errorf("expected an error for %q", src)
		}
	}
}
