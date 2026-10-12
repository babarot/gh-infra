package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

// Properties of plan and apply (#191), checked against fakeGitHub for a table
// of states and manifests:
//
//  1. Apply converges: after applying the plan, a second plan has no changes.
//  2. Apply touches only what the plan showed: every field of the fake's
//     state that apply changed belongs to a change in the plan. This compares
//     the fake's state, so it also sees values export does not write ("",
//     secrets).
//  3. The plan applies: every planned change succeeds.

// propState is a repository whose settings differ from GitHub's defaults.
func propState() fakeState {
	return fakeState{
		Repo: fakeRepo{
			Description: "a repository", Homepage: "https://example.com", Visibility: "public",
			Topics:    []string{"go", "cli"},
			HasIssues: true, HasProjects: false, HasWiki: true, HasDiscussions: true,
			HasPullRequests: true, PullRequestCreationPolicy: manifest.PullRequestCreationAll,
			AllowMergeCommit: false, AllowSquashMerge: true, AllowRebaseMerge: true, AllowAutoMerge: true,
			DeleteBranchOnMerge: true,
			MergeCommitTitle:    "PR_TITLE", MergeCommitMessage: "PR_BODY",
			SquashMergeCommitTitle: "PR_TITLE", SquashMergeCommitMessage: "COMMIT_MESSAGES",
		},
		ImmutableReleases:   true,
		VulnerabilityAlerts: true, AutomatedSecurityFixes: true, PrivateVulnerabilityReporting: false,
		Actions:         fakeActions{Enabled: true, AllowedActions: "selected", SHAPinningRequired: true},
		Workflow:        fakeWorkflow{DefaultWorkflowPermissions: "read", CanApprovePullRequestReviews: false},
		SelectedActions: fakeSelectedActions{GithubOwnedAllowed: true, VerifiedAllowed: false, PatternsAllowed: []string{"actions/*"}},
		ForkPRApproval:  "first_time_contributors",
		Labels: []fakeLabel{
			{Name: "bug", Color: "d73a4a", Description: "a bug"},
			{Name: "chore", Color: "ffffff", Description: ""},
		},
		Secrets:   map[string]string{"TOKEN": "old"},
		Variables: map[string]string{"ENV": "prod", "REGION": "us"},
		Milestones: []fakeMilestone{
			{Number: 1, Title: "v1", Description: "first", State: "closed", DueOn: manifest.Ptr("2025-12-31T00:00:00Z")},
			{Number: 2, Title: "v2", State: "open"},
		},
	}
}

func TestApplyProperties(t *testing.T) {
	ptr := manifest.Ptr[string]
	bptr := manifest.Ptr[bool]
	for _, tt := range []struct {
		name   string
		state  func(*fakeState)
		spec   manifest.RepositorySpec
		recon  *manifest.RepositoryReconcile
		noPlan bool // the manifest matches the state
	}{
		{name: "the same as GitHub", noPlan: true, spec: manifest.RepositorySpec{
			Description: ptr("a repository"), Topics: []string{"cli", "go"},
			Features: &manifest.Features{Issues: bptr(true), Wiki: bptr(true)},
			Labels:   []manifest.Label{{Name: "bug", Color: "d73a4a"}},
		}},
		{name: "repository settings", spec: manifest.RepositorySpec{
			Description: ptr("new"), Homepage: ptr(""), Topics: []string{"go", "tool"},
			ReleaseImmutability: bptr(false),
		}},
		{name: "visibility to private", spec: manifest.RepositorySpec{Visibility: ptr("private")}},
		{name: "archive", spec: manifest.RepositorySpec{Archived: bptr(true)}},
		// An archived repository is read-only, so the archive goes last and
		// the unarchive first.
		{name: "archive with other changes", spec: manifest.RepositorySpec{
			Archived: bptr(true), Description: ptr("old project"), Topics: []string{"archived"},
			Features: &manifest.Features{Wiki: bptr(false)},
			Labels:   []manifest.Label{{Name: "bug", Color: "ff0000"}},
			Secrets:  []manifest.Secret{{Name: "NEW", Value: "s3cret"}},
		}},
		{name: "unarchive with other changes", state: func(s *fakeState) {
			s.Repo.Archived = true
		}, spec: manifest.RepositorySpec{
			Archived: bptr(false), Description: ptr("back"), Homepage: ptr("https://example.com"), Visibility: ptr("private"),
			Labels: []manifest.Label{{Name: "new", Color: "00ff00"}},
		}},
		{name: "topics cleared", spec: manifest.RepositorySpec{Topics: []string{}, TopicsSet: true}},
		{name: "features", spec: manifest.RepositorySpec{Features: &manifest.Features{
			Issues: bptr(false), Projects: bptr(true), Discussions: bptr(false),
			PullRequests: &manifest.PullRequests{Enabled: bptr(true), Creation: ptr(manifest.PullRequestCreationCollaboratorsOnly)},
		}}},
		{name: "pull requests disabled", spec: manifest.RepositorySpec{Features: &manifest.Features{
			PullRequests: &manifest.PullRequests{Enabled: bptr(false)},
		}}},
		{name: "merge strategy", spec: manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{
			AllowMergeCommit: bptr(true), AllowRebaseMerge: bptr(false), AllowAutoMerge: bptr(false), AutoDeleteHeadBranches: bptr(false),
			MergeCommitTitle: ptr("MERGE_MESSAGE"), MergeCommitMessage: ptr("PR_TITLE"),
		}}},
		{name: "one half of a commit message pair", spec: manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{
			SquashMergeCommitMessage: ptr("PR_BODY"),
		}}},
		{name: "security", spec: manifest.RepositorySpec{Security: &manifest.Security{
			AutomatedSecurityFixes: bptr(false), PrivateVulnerabilityReporting: bptr(true),
		}}},
		{name: "vulnerability alerts off", spec: manifest.RepositorySpec{Security: &manifest.Security{
			VulnerabilityAlerts: bptr(false), AutomatedSecurityFixes: bptr(false),
		}}},
		{name: "security turned on", state: func(s *fakeState) {
			s.VulnerabilityAlerts, s.AutomatedSecurityFixes = false, false
		}, spec: manifest.RepositorySpec{Security: &manifest.Security{
			VulnerabilityAlerts: bptr(true), AutomatedSecurityFixes: bptr(true),
		}}},
		{name: "actions permissions", spec: manifest.RepositorySpec{Actions: &manifest.Actions{
			Enabled: bptr(true), AllowedActions: ptr("all"), SHAPinningRequired: bptr(false),
			WorkflowPermissions: ptr("write"), CanApprovePullRequests: bptr(true), ForkPRApproval: ptr("all_external_contributors"),
		}}},
		{name: "selected actions", spec: manifest.RepositorySpec{Actions: &manifest.Actions{
			Enabled: bptr(true), AllowedActions: ptr("selected"),
			SelectedActions: &manifest.SelectedActions{VerifiedAllowed: bptr(true), PatternsAllowed: []string{"actions/*", "docker/*"}},
		}}},
		{name: "selected actions from all", state: func(s *fakeState) {
			s.Actions.AllowedActions = "all"
		}, spec: manifest.RepositorySpec{Actions: &manifest.Actions{
			Enabled: bptr(true), AllowedActions: ptr("selected"),
			SelectedActions: &manifest.SelectedActions{GithubOwnedAllowed: bptr(false)},
		}}},
		{name: "actions disabled", spec: manifest.RepositorySpec{Actions: &manifest.Actions{Enabled: bptr(false)}}},
		{name: "actions disabled with permissions left in the manifest", spec: manifest.RepositorySpec{Actions: &manifest.Actions{
			Enabled: bptr(false), AllowedActions: ptr("all"), SHAPinningRequired: bptr(false),
		}}},
		{name: "actions enabled again", state: func(s *fakeState) {
			s.Actions.Enabled = false
		}, spec: manifest.RepositorySpec{Actions: &manifest.Actions{Enabled: bptr(true), AllowedActions: ptr("all")}}},
		{name: "labels", spec: manifest.RepositorySpec{Labels: []manifest.Label{
			{Name: "bug", Color: "ff0000"},
			{Name: "Chore", Color: "ffffff", Description: ptr("tidy up")},
			{Name: "new", Color: "00ff00", Description: ptr("added")},
		}}},
		{name: "label description cleared", spec: manifest.RepositorySpec{Labels: []manifest.Label{
			{Name: "bug", Color: "d73a4a", Description: ptr("")},
		}}},
		{name: "labels authoritative", recon: &manifest.RepositoryReconcile{Labels: manifest.Ptr(manifest.CollectionReconcileAuthoritative)},
			spec: manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "d73a4a", Description: ptr("a bug")}}}},
		{name: "secrets and variables", spec: manifest.RepositorySpec{
			Secrets:   []manifest.Secret{{Name: "NEW", Value: "s3cret"}},
			Variables: []manifest.Variable{{Name: "ENV", Value: "dev"}, {Name: "NEW", Value: "x"}},
		}},
		{name: "milestones", spec: manifest.RepositorySpec{Milestones: []manifest.Milestone{
			{Title: "v1", State: ptr("open"), DueOn: ptr("")},
			{Title: "v2", Description: ptr("second"), DueOn: ptr("2026-06-30")},
			{Title: "v3"},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := propState()
			if tt.state != nil {
				tt.state(&s)
			}
			checkApplyProperties(t, s, &manifest.Repository{
				Metadata:  manifest.RepositoryMetadata{Owner: "o", Name: "r"},
				Reconcile: tt.recon,
				Spec:      tt.spec,
			}, tt.noPlan)
		})
	}
}

// checkApplyProperties plans and applies repo against a fake GitHub in state
// s, and checks the three properties.
func checkApplyProperties(t *testing.T, s fakeState, repo *manifest.Repository, noPlan bool) {
	t.Helper()
	ctx := context.Background()
	fake := newFakeGitHub(repo.Metadata.Owner, repo.Metadata.Name, s)
	p := NewProcessor(fake, manifest.NewResolver(fake))
	before := fake.state()

	changes, targets, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if noPlan != (len(changes) == 0) {
		t.Fatalf("plan has %d changes, want none: %v", len(changes), noPlan)
	}

	// 3. Every planned change applies.
	for _, r := range p.Apply(ctx, changes, targets, nil) {
		if r.Err != nil {
			t.Errorf("apply %s %s: %v", r.Change.Resource, r.Change.Field, r.Err)
		}
	}
	after := fake.state()

	// 2. Only what the plan showed changed.
	allowed := allowedPaths(changes)
	b, a := flattenState(before), flattenState(after)
	for _, path := range changedPaths(before, after) {
		if !allowed.covers(path) && !allowed.coversNewObject(path, b, a) {
			t.Errorf("apply changed %s, which the plan does not show\nplan: %s\nwrites: %s",
				path, formatChanges(changes), strings.Join(fake.writes, "\n  "))
		}
	}
	// fetch reads secret names only, so property 1 cannot check their
	// values: a changed secret must hold the manifest's value.
	for _, sec := range repo.Spec.Secrets {
		if v, ok := after.Secrets[sec.Name]; ok && v != before.Secrets[sec.Name] && v != sec.Value {
			t.Errorf("secret %s = %q, want %q", sec.Name, v, sec.Value)
		}
	}
	for name, v := range after.Secrets {
		if old, ok := before.Secrets[name]; ok && old != v && !specHasSecret(repo.Spec, name) {
			t.Errorf("secret %s changed though the manifest does not set it", name)
		}
	}

	// 1. A second plan has no changes, and applies nothing.
	writes := len(fake.writes)
	again, _, err := p.Plan(ctx, []*manifest.Repository{repo}, PlanOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) > 0 {
		t.Errorf("plan after apply still has changes: %s\nwrites: %s", formatChanges(again), strings.Join(fake.writes, "\n  "))
	}
	if len(fake.writes) != writes {
		t.Errorf("plan wrote to GitHub: %v", fake.writes[writes:])
	}
}

func specHasSecret(spec manifest.RepositorySpec, name string) bool {
	for _, s := range spec.Secrets {
		if s.Name == name {
			return true
		}
	}
	return false
}

// flattenState lists the fake's state as path -> value, keyed the way
// allowedPaths names them.
func flattenState(s fakeState) map[string]string {
	out := map[string]string{}
	for k, v := range mustJSONMap(s.Repo) {
		if k == "topics" {
			continue
		}
		out["repo."+k] = fmt.Sprint(v)
	}
	topics := append([]string(nil), s.Repo.Topics...)
	sort.Strings(topics)
	out["repo.topics"] = strings.Join(topics, ",")
	out["immutable_releases"] = fmt.Sprint(s.ImmutableReleases)
	out["vulnerability_alerts"] = fmt.Sprint(s.VulnerabilityAlerts)
	out["automated_security_fixes"] = fmt.Sprint(s.AutomatedSecurityFixes)
	out["private_vulnerability_reporting"] = fmt.Sprint(s.PrivateVulnerabilityReporting)
	for prefix, v := range map[string]any{"actions.": s.Actions, "workflow.": s.Workflow, "selected_actions.": s.SelectedActions} {
		for k, x := range mustJSONMap(v) {
			out[prefix+k] = fmt.Sprint(x)
		}
	}
	out["fork_pr_approval"] = s.ForkPRApproval
	// Labels are keyed by lowercase name, as GitHub matches them; a case-only
	// rename is a change of name.
	for _, l := range s.Labels {
		key := "labels." + strings.ToLower(l.Name) + "."
		out[key+"name"] = l.Name
		out[key+"color"] = l.Color
		out[key+"description"] = l.Description
	}
	for n, v := range s.Secrets {
		out["secrets."+n] = v
	}
	for n, v := range s.Variables {
		out["variables."+n] = v
	}
	for _, m := range s.Milestones {
		key := "milestones." + m.Title + "."
		out[key+"description"] = m.Description
		out[key+"state"] = m.State
		out[key+"due_on"] = derefStr(m.DueOn)
	}
	flattenProtections(out, s.Protections)
	flattenRulesets(out, s.Rulesets)
	return out
}

// changedPaths lists the paths whose value differs, including ones added or
// removed.
func changedPaths(before, after fakeState) []string {
	b, a := flattenState(before), flattenState(after)
	var paths []string
	for k, v := range a {
		if old, ok := b[k]; !ok || old != v {
			paths = append(paths, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			paths = append(paths, k)
		}
	}
	sort.Strings(paths)
	return paths
}

type pathSet map[string]bool

// covers reports whether path is one of the set or under a prefix in it
// (a prefix ends with ".").
func (s pathSet) covers(path string) bool {
	if s[path] {
		return true
	}
	for p := range s {
		if strings.HasSuffix(p, ".") && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// repositoryFieldPaths maps a planned Repository or Actions field to the
// state it may change. A field listed with more than one path changes them
// together on GitHub.
var repositoryFieldPaths = map[string][]string{
	"Repository:description":                        {"repo.description"},
	"Repository:homepage":                           {"repo.homepage"},
	"Repository:visibility":                         {"repo.visibility"},
	"Repository:archived":                           {"repo.archived"},
	"Repository:topics":                             {"repo.topics"},
	"Repository:release_immutability":               {"immutable_releases"},
	"Repository:vulnerability_alerts":               {"vulnerability_alerts", "automated_security_fixes"}, // turning alerts off turns updates off
	"Repository:automated_security_fixes":           {"automated_security_fixes"},
	"Repository:private_vulnerability_reporting":    {"private_vulnerability_reporting"},
	"Repository:issues":                             {"repo.has_issues"},
	"Repository:projects":                           {"repo.has_projects"},
	"Repository:wiki":                               {"repo.has_wiki"},
	"Repository:discussions":                        {"repo.has_discussions"},
	"Repository:pull_requests":                      {"repo.has_pull_requests"},
	"Repository:pull_requests.creation":             {"repo.pull_request_creation_policy"},
	"Repository:allow_merge_commit":                 {"repo.allow_merge_commit"},
	"Repository:allow_squash_merge":                 {"repo.allow_squash_merge"},
	"Repository:allow_rebase_merge":                 {"repo.allow_rebase_merge"},
	"Repository:allow_auto_merge":                   {"repo.allow_auto_merge"},
	"Repository:auto_delete_head_branches":          {"repo.delete_branch_on_merge"},
	"Repository:merge_commit_title":                 {"repo.merge_commit_title"},
	"Repository:merge_commit_message":               {"repo.merge_commit_message"},
	"Repository:squash_merge_commit_title":          {"repo.squash_merge_commit_title"},
	"Repository:squash_merge_commit_message":        {"repo.squash_merge_commit_message"},
	"Actions:enabled":                               {"actions.enabled"},
	"Actions:allowed_actions":                       {"actions.allowed_actions"},
	"Actions:sha_pinning_required":                  {"actions.sha_pinning_required"},
	"Actions:workflow_permissions":                  {"workflow.default_workflow_permissions"},
	"Actions:can_approve_pull_requests":             {"workflow.can_approve_pull_request_reviews"},
	"Actions:fork_pr_approval":                      {"fork_pr_approval"},
	"Actions:selected_actions.github_owned_allowed": {"selected_actions.github_owned_allowed"},
	"Actions:selected_actions.verified_allowed":     {"selected_actions.verified_allowed"},
	"Actions:selected_actions.patterns_allowed":     {"selected_actions.patterns_allowed"},
}

// allowedPaths lists the state the planned changes may change. A planned
// field the table does not know allows nothing, so the test fails on it.
func allowedPaths(changes []Change) pathSet {
	set := pathSet{}
	var add func(resource string, c Change)
	add = func(resource string, c Change) {
		if len(c.Children) > 0 {
			for _, child := range c.Children {
				add(resource, child)
			}
			return
		}
		switch resource {
		case manifest.ResourceLabel:
			set["labels."+strings.ToLower(c.Field)+"."] = true
		case manifest.ResourceMilestone:
			set["milestones."+c.Field+"."] = true
		case manifest.ResourceSecret:
			set["secrets."+c.Field] = true
		case manifest.ResourceVariable:
			set["variables."+c.Field] = true
		default:
			for _, p := range repositoryFieldPaths[resource+":"+c.Field] {
				set[p] = true
			}
		}
	}
	for _, c := range changes {
		if resource, key, ok := splitApplyResource(c.Resource); ok {
			addKeyedPaths(set, resource, key, c)
			continue
		}
		add(c.Resource, c)
	}
	return set
}

func formatChanges(changes []Change) string {
	var parts []string
	for _, c := range changes {
		if len(c.Children) > 0 {
			for _, ch := range c.Children {
				parts = append(parts, fmt.Sprintf("%s %s.%s=%v", c.Resource, c.Field, ch.Field, ch.NewValue))
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s %s=%v", c.Type, c.Resource, c.Field, c.NewValue))
	}
	return strings.Join(parts, "; ")
}
