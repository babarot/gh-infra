package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/babarot/gh-infra/internal/gh"
)

// fakeGitHub is a gh.Runner that answers the gh commands gh-infra uses for
// one repository and applies writes to its state, so a test can apply and
// then fetch again.
//
// The state is kept in the shapes of the GitHub API, not of CurrentState:
// copying fetch's reading would let fetch and apply agree on the same
// mistake. A command it does not know fails, so a wrong command fails the
// test instead of succeeding with no output.
//
// Behaviors of GitHub that gh-infra works around are reproduced here, each
// with the reason. What it cannot catch is a wrong assumption about GitHub
// shared by gh-infra and this fake.
type fakeGitHub struct {
	mu    sync.Mutex
	owner string
	name  string
	s     fakeState
	// writes records each command that changed the state, for failure output.
	writes []string
	// missing: the repository does not exist until gh repo create.
	missing bool
	// defaultLabels are the labels GitHub creates in a new repository; an
	// organization can change them.
	defaultLabels []fakeLabel
}

type fakeState struct {
	Repo fakeRepo
	// Single-value endpoints.
	ImmutableReleases             bool
	VulnerabilityAlerts           bool
	AutomatedSecurityFixes        bool
	PrivateVulnerabilityReporting bool

	Actions         fakeActions
	Workflow        fakeWorkflow
	SelectedActions fakeSelectedActions
	// ForkPRApproval is stored even for a private repository; GitHub does
	// not report it there.
	ForkPRApproval string

	Labels     []fakeLabel
	Secrets    map[string]string // name -> value; GitHub returns names only
	Variables  map[string]string
	Milestones []fakeMilestone

	Branches      []string
	Protections   map[string]*fakeProtection
	Rulesets      []*fakeRuleset
	NextRulesetID int
	Org           fakeOrg
}

// fakeRepo holds the repository fields under their REST names.
type fakeRepo struct {
	Description               string   `json:"description"`
	Homepage                  string   `json:"homepage"`
	Visibility                string   `json:"visibility"`
	Archived                  bool     `json:"archived"`
	Topics                    []string `json:"topics"`
	HasIssues                 bool     `json:"has_issues"`
	HasProjects               bool     `json:"has_projects"`
	HasWiki                   bool     `json:"has_wiki"`
	HasDiscussions            bool     `json:"has_discussions"`
	HasPullRequests           bool     `json:"has_pull_requests"`
	PullRequestCreationPolicy string   `json:"pull_request_creation_policy"`
	AllowMergeCommit          bool     `json:"allow_merge_commit"`
	AllowSquashMerge          bool     `json:"allow_squash_merge"`
	AllowRebaseMerge          bool     `json:"allow_rebase_merge"`
	AllowAutoMerge            bool     `json:"allow_auto_merge"`
	DeleteBranchOnMerge       bool     `json:"delete_branch_on_merge"`
	MergeCommitTitle          string   `json:"merge_commit_title"`
	MergeCommitMessage        string   `json:"merge_commit_message"`
	SquashMergeCommitTitle    string   `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage  string   `json:"squash_merge_commit_message"`
	DefaultBranch             string   `json:"default_branch"`
}

type fakeActions struct {
	Enabled            bool   `json:"enabled"`
	AllowedActions     string `json:"allowed_actions"`
	SHAPinningRequired bool   `json:"sha_pinning_required"`
}

type fakeWorkflow struct {
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
}

type fakeSelectedActions struct {
	GithubOwnedAllowed bool     `json:"github_owned_allowed"`
	VerifiedAllowed    bool     `json:"verified_allowed"`
	PatternsAllowed    []string `json:"patterns_allowed"`
}

type fakeLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type fakeMilestone struct {
	Number      int     `json:"number"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	State       string  `json:"state"`
	DueOn       *string `json:"due_on"`
}

func newFakeGitHub(owner, name string, s fakeState) *fakeGitHub {
	if s.Secrets == nil {
		s.Secrets = map[string]string{}
	}
	if s.Variables == nil {
		s.Variables = map[string]string{}
	}
	if s.Repo.DefaultBranch == "" {
		s.Repo.DefaultBranch = "main"
	}
	if s.Protections == nil {
		s.Protections = map[string]*fakeProtection{}
	}
	for _, rs := range s.Rulesets {
		s.NextRulesetID = max(s.NextRulesetID, rs.ID)
	}
	return &fakeGitHub{owner: owner, name: name, s: s}
}

// githubDefaultLabels are the labels GitHub creates in a new repository of a
// user (seen on a new repository, 2026-10).
var githubDefaultLabels = []fakeLabel{
	{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	{Name: "accessibility", Color: "c5def5", Description: "Accessibility related issues"},
	{Name: "documentation", Color: "0075ca", Description: "Improvements or additions to documentation"},
	{Name: "duplicate", Color: "cfd3d7", Description: "This issue or pull request already exists"},
	{Name: "enhancement", Color: "a2eeef", Description: "New feature or request"},
	{Name: "good first issue", Color: "7057ff", Description: "Good for newcomers"},
	{Name: "help wanted", Color: "008672", Description: "Extra attention is needed"},
	{Name: "invalid", Color: "e4e669", Description: "This doesn't seem right"},
	{Name: "question", Color: "d876e3", Description: "Further information is requested"},
	{Name: "wontfix", Color: "ffffff", Description: "This will not be worked on"},
}

// newMissingFakeGitHub is a fake for a repository that does not exist yet.
// gh repo create makes it as GitHub does: empty, with no branch, the
// features and merge methods on, Actions allowed, and defaultLabels.
func newMissingFakeGitHub(owner, name string, defaultLabels []fakeLabel) *fakeGitHub {
	f := newFakeGitHub(owner, name, fakeState{})
	f.missing = true
	f.defaultLabels = defaultLabels
	return f
}

func (f *fakeGitHub) Run(_ context.Context, args ...string) ([]byte, error) {
	return f.handle(nil, args)
}

func (f *fakeGitHub) RunWithStdin(_ context.Context, body []byte, args ...string) ([]byte, error) {
	return f.handle(body, args)
}

// state returns a copy of the current state.
func (f *fakeGitHub) state() fakeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, _ := json.Marshal(f.s)
	var c fakeState
	_ = json.Unmarshal(data, &c)
	return c
}

func (f *fakeGitHub) handle(body []byte, args []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := strings.Join(args, " ")
	// GitHub makes an archived repository read-only, except unarchiving it.
	// Some of its APIs still accept writes (secrets, variables, topics,
	// Actions permissions), but gh-infra does not change an archived
	// repository, so the fake refuses every write: it holds gh-infra's rule,
	// not every detail of GitHub's. Writes change the state through pointers
	// too, so restore a deep copy.
	var before []byte
	readOnly := f.s.Repo.Archived && !strings.HasPrefix(cmd, "repo unarchive ")
	if readOnly {
		before, _ = json.Marshal(f.s)
	}
	if f.missing && f.aboutRepository(args) && !strings.HasPrefix(cmd, "repo create ") {
		// gh repo view reports a missing repository through GraphQL, the
		// REST endpoints with 404.
		if strings.HasPrefix(cmd, "repo view ") {
			return nil, fmt.Errorf("GraphQL: Could not resolve to a Repository with the name '%s'. (repository)", f.fullName())
		}
		return nil, fakeStatus(gh.ErrNotFound, "Not Found")
	}
	out, wrote, err := f.dispatch(body, args)
	if readOnly && wrote {
		f.s = fakeState{}
		_ = json.Unmarshal(before, &f.s)
		return nil, fakeStatus(gh.ErrForbidden, "Repository was archived so is read-only")
	}
	if err == nil && wrote {
		f.writes = append(f.writes, cmd)
	}
	if err != nil && !isStatusError(err) {
		return nil, fmt.Errorf("fake GitHub: %s: %w", cmd, err)
	}
	return out, err
}

// errUnknown marks a command the fake does not know; the test fails on it.
var errUnknown = fmt.Errorf("unknown command")

func isStatusError(err error) bool {
	for _, e := range []error{gh.ErrNotFound, gh.ErrValidation, gh.ErrConflict, gh.ErrForbidden} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

func fakeStatus(e error, msg string) error { return fmt.Errorf("%s: %w", msg, e) }

func (f *fakeGitHub) fullName() string { return f.owner + "/" + f.name }

// aboutRepository reports whether a command reads or writes the repository,
// as opposed to its owner (teams, apps, roles).
func (f *fakeGitHub) aboutRepository(args []string) bool {
	if len(args) > 1 && args[0] == "api" {
		return strings.HasPrefix(strings.TrimPrefix(args[1], "/"), "repos/")
	}
	return true
}

func (f *fakeGitHub) dispatch(body []byte, args []string) ([]byte, bool, error) {
	if len(args) < 2 {
		return nil, false, errUnknown
	}
	switch args[0] {
	case "api":
		return f.api(body, args[1:])
	case "repo":
		return f.repoCmd(args[1:])
	case "label":
		return f.labelCmd(args[1:])
	case "secret":
		return f.secretCmd(args[1:])
	case "variable":
		return f.variableCmd(args[1:])
	}
	return nil, false, errUnknown
}

// fakeFlag returns the value after name, if present.
func fakeFlag(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func (f *fakeGitHub) repoCmd(args []string) ([]byte, bool, error) {
	if len(args) < 2 || args[1] != f.fullName() {
		return nil, false, errUnknown
	}
	r := &f.s.Repo
	switch args[0] {
	case "view":
		fields, _ := fakeFlag(args, "--json")
		if jq, ok := fakeFlag(args, "--jq"); ok {
			if fields == "repositoryTopics" && jq == ".repositoryTopics // [] | .[].name" {
				return fakeLines(r.Topics), false, nil
			}
			return nil, false, errUnknown
		}
		if fields != "description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef" {
			return nil, false, errUnknown
		}
		topics := make([]map[string]string, 0, len(r.Topics))
		for _, t := range r.Topics {
			topics = append(topics, map[string]string{"name": t})
		}
		return mustJSON(map[string]any{
			"description":           r.Description,
			"homepageUrl":           r.Homepage,
			"visibility":            strings.ToUpper(r.Visibility),
			"isArchived":            r.Archived,
			"repositoryTopics":      topics,
			"hasIssuesEnabled":      r.HasIssues,
			"hasProjectsEnabled":    r.HasProjects,
			"hasWikiEnabled":        r.HasWiki,
			"hasDiscussionsEnabled": r.HasDiscussions,
			"mergeCommitAllowed":    r.AllowMergeCommit,
			"squashMergeAllowed":    r.AllowSquashMerge,
			"rebaseMergeAllowed":    r.AllowRebaseMerge,
			"deleteBranchOnMerge":   r.DeleteBranchOnMerge,
			"defaultBranchRef":      map[string]string{"name": r.DefaultBranch},
		}), false, nil
	case "create":
		if !f.missing {
			return nil, false, fakeStatus(gh.ErrValidation, "name already exists on this account")
		}
		visibility := ""
		for _, v := range []string{"private", "public", "internal"} {
			if slices.Contains(args, "--"+v) {
				visibility = v
			}
		}
		if visibility == "" {
			return nil, false, errUnknown
		}
		desc, _ := fakeFlag(args, "--description")
		f.s = fakeState{
			Repo: fakeRepo{
				Description: desc, Visibility: visibility,
				HasIssues: !slices.Contains(args, "--disable-issues"), HasProjects: true,
				HasWiki: !slices.Contains(args, "--disable-wiki"), HasPullRequests: true,
				AllowMergeCommit: true, AllowSquashMerge: true, AllowRebaseMerge: true,
				MergeCommitTitle: "MERGE_MESSAGE", MergeCommitMessage: "PR_TITLE",
				SquashMergeCommitTitle: "COMMIT_OR_PR_TITLE", SquashMergeCommitMessage: "COMMIT_MESSAGES",
				PullRequestCreationPolicy: "all",
			},
			VulnerabilityAlerts: false,
			Actions:             fakeActions{Enabled: true, AllowedActions: "all"},
			Workflow:            fakeWorkflow{DefaultWorkflowPermissions: "read"},
			Labels:              slices.Clone(f.defaultLabels),
			Org:                 f.s.Org, // the owner's, not the repository's
		}
		f.s = newFakeGitHub(f.owner, f.name, f.s).s
		// A new repository has no commit, so no branch yet.
		f.s.Repo.DefaultBranch = ""
		f.missing = false
		return nil, true, nil
	case "archive", "unarchive":
		if !slices.Contains(args, "--yes") {
			return nil, false, errUnknown
		}
		r.Archived = args[0] == "archive"
		return nil, true, nil
	case "edit":
		return f.repoEdit(args[2:])
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) repoEdit(args []string) ([]byte, bool, error) {
	r := &f.s.Repo
	if len(args) == 0 {
		return nil, false, errUnknown
	}
	switch args[0] {
	case "--description":
		r.Description = args[1]
		return nil, true, nil
	case "--homepage":
		r.Homepage = args[1]
		return nil, true, nil
	case "--visibility":
		// gh refuses a visibility change without this flag.
		if !slices.Contains(args, "--accept-visibility-change-consequences") {
			return nil, false, errUnknown
		}
		r.Visibility = args[1]
		return nil, true, nil
	case "--add-topic":
		if !slices.Contains(r.Topics, args[1]) {
			r.Topics = append(r.Topics, args[1])
		}
		return nil, true, nil
	case "--remove-topic":
		r.Topics = fakeRemove(r.Topics, args[1])
		return nil, true, nil
	}
	key, val, ok := strings.Cut(args[0], "=")
	if !ok {
		return nil, false, errUnknown
	}
	v, err := strconv.ParseBool(val)
	if err != nil {
		return nil, false, errUnknown
	}
	target := map[string]*bool{
		"--enable-issues":          &r.HasIssues,
		"--enable-projects":        &r.HasProjects,
		"--enable-wiki":            &r.HasWiki,
		"--enable-discussions":     &r.HasDiscussions,
		"--enable-merge-commit":    &r.AllowMergeCommit,
		"--enable-squash-merge":    &r.AllowSquashMerge,
		"--enable-rebase-merge":    &r.AllowRebaseMerge,
		"--delete-branch-on-merge": &r.DeleteBranchOnMerge,
	}[key]
	if target == nil {
		return nil, false, errUnknown
	}
	*target = v
	return nil, true, nil
}

func (f *fakeGitHub) api(body []byte, args []string) ([]byte, bool, error) {
	path := strings.TrimPrefix(args[0], "/")
	rest := args[1:]
	method, ok := fakeFlag(rest, "--method")
	if !ok {
		method, ok = fakeFlag(rest, "-X")
	}
	if !ok {
		method = "GET"
	}
	if slices.Contains(rest, "--input") {
		if v, _ := fakeFlag(rest, "--input"); v != "-" || body == nil {
			return nil, false, errUnknown
		}
	}

	if path == "search/issues" {
		if jq, _ := fakeFlag(rest, "--jq"); jq != `"\(.total_count) \(.items[0].updated_at // "")"` {
			return nil, false, errUnknown
		}
		return []byte("0 \n"), false, nil
	}

	if strings.HasPrefix(path, "apps/") || strings.HasPrefix(path, "orgs/") {
		return f.apiOrg(path, method)
	}

	prefix := "repos/" + f.fullName()
	if path != prefix && !strings.HasPrefix(path, prefix+"/") {
		return nil, false, errUnknown
	}
	sub := strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/")

	switch {
	case sub == "":
		return f.apiRepo(method, body, rest)
	case sub == "immutable-releases":
		return f.toggle(method, &f.s.ImmutableReleases, func() []byte {
			return mustJSON(map[string]any{"enabled": f.s.ImmutableReleases, "enforced_by_owner": false})
		})
	case sub == "vulnerability-alerts":
		if method == "GET" {
			// 204 when enabled, 404 when disabled.
			if !f.s.VulnerabilityAlerts {
				return nil, false, fakeStatus(gh.ErrNotFound, "vulnerability alerts are disabled")
			}
			return nil, false, nil
		}
		out, wrote, err := f.toggle(method, &f.s.VulnerabilityAlerts, nil)
		// Dependabot security updates need the alerts; GitHub turns them
		// off with the alerts.
		if err == nil && !f.s.VulnerabilityAlerts {
			f.s.AutomatedSecurityFixes = false
		}
		return out, wrote, err
	case sub == "automated-security-fixes":
		if method == "PUT" && !f.s.VulnerabilityAlerts {
			return nil, false, fakeStatus(gh.ErrValidation, "vulnerability alerts are disabled")
		}
		return f.toggle(method, &f.s.AutomatedSecurityFixes, func() []byte {
			return mustJSON(map[string]any{"enabled": f.s.AutomatedSecurityFixes, "paused": false})
		})
	case sub == "private-vulnerability-reporting":
		return f.toggle(method, &f.s.PrivateVulnerabilityReporting, func() []byte {
			return mustJSON(map[string]any{"enabled": f.s.PrivateVulnerabilityReporting})
		})
	case strings.HasPrefix(sub, "actions/permissions"):
		return f.apiActions(strings.TrimPrefix(sub, "actions/permissions"), method, body)
	case strings.HasPrefix(sub, "branches"):
		return f.apiBranches(sub, method, body, rest)
	case sub == "rulesets" || strings.HasPrefix(sub, "rulesets/"):
		return f.apiRulesets(sub, method, body)
	case sub == "labels" || strings.HasPrefix(sub, "labels/"):
		return f.apiLabel(strings.TrimPrefix(sub, "labels/"), method, body)
	case sub == "milestones?state=all&per_page=100" && method == "GET" && slices.Contains(rest, "--paginate"):
		return mustJSON(f.s.Milestones), false, nil
	case sub == "milestones" || strings.HasPrefix(sub, "milestones/"):
		return f.apiMilestone(strings.TrimPrefix(sub, "milestones"), method, body)
	}
	return nil, false, errUnknown
}

// toggle handles an endpoint that is enabled with PUT and disabled with
// DELETE; get renders its GET response.
func (f *fakeGitHub) toggle(method string, v *bool, get func() []byte) ([]byte, bool, error) {
	switch method {
	case "GET":
		if get == nil {
			return nil, false, errUnknown
		}
		return get(), false, nil
	case "PUT":
		*v = true
		return nil, true, nil
	case "DELETE":
		*v = false
		return nil, true, nil
	}
	return nil, false, errUnknown
}

// repoFields are the fields of PATCH repos/{o}/{r} the fake accepts.
var repoBoolFields = map[string]func(*fakeRepo) *bool{
	"has_projects":           func(r *fakeRepo) *bool { return &r.HasProjects },
	"has_discussions":        func(r *fakeRepo) *bool { return &r.HasDiscussions },
	"has_pull_requests":      func(r *fakeRepo) *bool { return &r.HasPullRequests },
	"allow_merge_commit":     func(r *fakeRepo) *bool { return &r.AllowMergeCommit },
	"allow_squash_merge":     func(r *fakeRepo) *bool { return &r.AllowSquashMerge },
	"allow_rebase_merge":     func(r *fakeRepo) *bool { return &r.AllowRebaseMerge },
	"allow_auto_merge":       func(r *fakeRepo) *bool { return &r.AllowAutoMerge },
	"delete_branch_on_merge": func(r *fakeRepo) *bool { return &r.DeleteBranchOnMerge },
}

var repoStringFields = map[string]func(*fakeRepo) *string{
	"homepage":                     func(r *fakeRepo) *string { return &r.Homepage },
	"pull_request_creation_policy": func(r *fakeRepo) *string { return &r.PullRequestCreationPolicy },
	"merge_commit_title":           func(r *fakeRepo) *string { return &r.MergeCommitTitle },
	"merge_commit_message":         func(r *fakeRepo) *string { return &r.MergeCommitMessage },
	"squash_merge_commit_title":    func(r *fakeRepo) *string { return &r.SquashMergeCommitTitle },
	"squash_merge_commit_message":  func(r *fakeRepo) *string { return &r.SquashMergeCommitMessage },
}

func (f *fakeGitHub) apiRepo(method string, body []byte, rest []string) ([]byte, bool, error) {
	r := &f.s.Repo
	switch method {
	case "GET":
		jq, _ := fakeFlag(rest, "--jq")
		all := mustJSONMap(*r)
		var keys []string
		switch jq {
		case "{squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}":
			keys = []string{"squash_merge_commit_title", "squash_merge_commit_message", "merge_commit_title", "merge_commit_message", "allow_auto_merge", "has_pull_requests", "pull_request_creation_policy"}
		case "{squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message}":
			keys = []string{"squash_merge_commit_title", "squash_merge_commit_message", "merge_commit_title", "merge_commit_message"}
		default:
			return nil, false, errUnknown
		}
		out := map[string]any{}
		for _, k := range keys {
			out[k] = all[k]
		}
		return mustJSON(out), false, nil
	case "PATCH":
		fields := map[string]any{}
		if body != nil {
			if err := json.Unmarshal(body, &fields); err != nil {
				return nil, false, errUnknown
			}
		}
		// gh api -f sends every value as a string.
		for i, a := range rest {
			if a == "-f" && i+1 < len(rest) {
				k, v, _ := strings.Cut(rest[i+1], "=")
				fields[k] = v
			}
		}
		if len(fields) == 0 {
			return nil, false, errUnknown
		}
		// GitHub rejects one half of a merge or squash commit title and
		// message pair without the other (fillMergeCommitCompanions).
		for _, pair := range mergeCommitPairs {
			_, t := fields[pair[0]]
			_, m := fields[pair[1]]
			if t != m {
				return nil, false, fakeStatus(gh.ErrValidation, pair[0]+" and "+pair[1]+" must be sent together")
			}
		}
		next := *r
		for k, v := range fields {
			if p, ok := repoBoolFields[k]; ok {
				b, ok := v.(bool)
				if s, isStr := v.(string); isStr {
					// Assumed: GitHub accepts "true"/"false" for a boolean
					// sent with -f.
					var err error
					b, err = strconv.ParseBool(s)
					ok = err == nil
				}
				if !ok {
					return nil, false, fakeStatus(gh.ErrValidation, k+" is not a boolean")
				}
				*p(&next) = b
				continue
			}
			if p, ok := repoStringFields[k]; ok {
				s, ok := v.(string)
				if !ok {
					return nil, false, fakeStatus(gh.ErrValidation, k+" is not a string")
				}
				*p(&next) = s
				continue
			}
			return nil, false, errUnknown
		}
		*r = next
		return mustJSON(*r), true, nil
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) apiActions(sub, method string, body []byte) ([]byte, bool, error) {
	private := f.s.Repo.Visibility == "private"
	switch sub {
	case "":
		switch method {
		case "GET":
			out := map[string]any{"enabled": f.s.Actions.Enabled}
			// GitHub does not report allowed_actions while Actions is disabled.
			if f.s.Actions.Enabled {
				out["allowed_actions"] = f.s.Actions.AllowedActions
				out["sha_pinning_required"] = f.s.Actions.SHAPinningRequired
			}
			return mustJSON(out), false, nil
		case "PUT":
			var p map[string]any
			if err := json.Unmarshal(body, &p); err != nil {
				return nil, false, errUnknown
			}
			enabled, ok := p["enabled"].(bool)
			if !ok {
				return nil, false, fakeStatus(gh.ErrValidation, "enabled is required")
			}
			// While Actions is disabled GitHub rejects allowed_actions
			// (applyActionsPermissions).
			if !enabled && (p["allowed_actions"] != nil || p["sha_pinning_required"] != nil) {
				return nil, false, fakeStatus(gh.ErrConflict, "actions are disabled")
			}
			f.s.Actions.Enabled = enabled
			if v, ok := p["allowed_actions"].(string); ok {
				f.s.Actions.AllowedActions = v
			}
			if v, ok := p["sha_pinning_required"].(bool); ok {
				f.s.Actions.SHAPinningRequired = v
			}
			return nil, true, nil
		}
	case "/workflow":
		switch method {
		case "GET":
			return mustJSON(f.s.Workflow), false, nil
		case "PUT":
			var p map[string]any
			if err := json.Unmarshal(body, &p); err != nil {
				return nil, false, errUnknown
			}
			if v, ok := p["default_workflow_permissions"].(string); ok {
				f.s.Workflow.DefaultWorkflowPermissions = v
			}
			if v, ok := p["can_approve_pull_request_reviews"].(bool); ok {
				f.s.Workflow.CanApprovePullRequestReviews = v
			}
			return nil, true, nil
		}
	case "/selected-actions":
		// Only while allowed_actions is "selected".
		if !f.s.Actions.Enabled || f.s.Actions.AllowedActions != "selected" {
			return nil, false, fakeStatus(gh.ErrConflict, "allowed_actions is not selected")
		}
		switch method {
		case "GET":
			return mustJSON(f.s.SelectedActions), false, nil
		case "PUT":
			// A PUT replaces the settings.
			var p fakeSelectedActions
			if err := json.Unmarshal(body, &p); err != nil {
				return nil, false, errUnknown
			}
			f.s.SelectedActions = p
			return nil, true, nil
		}
	case "/fork-pr-contributor-approval":
		// Not available on a private repository.
		if private {
			return nil, false, fakeStatus(gh.ErrValidation, "not available for private repositories")
		}
		switch method {
		case "GET":
			return mustJSON(map[string]string{"approval_policy": f.s.ForkPRApproval}), false, nil
		case "PUT":
			var p map[string]string
			if err := json.Unmarshal(body, &p); err != nil || p["approval_policy"] == "" {
				return nil, false, errUnknown
			}
			f.s.ForkPRApproval = p["approval_policy"]
			return nil, true, nil
		}
	}
	return nil, false, errUnknown
}

// findLabel finds a label by name, ignoring case as GitHub does.
func (f *fakeGitHub) findLabel(name string) int {
	for i, l := range f.s.Labels {
		if strings.EqualFold(l.Name, name) {
			return i
		}
	}
	return -1
}

func (f *fakeGitHub) apiLabel(escaped, method string, body []byte) ([]byte, bool, error) {
	if method != "PATCH" {
		return nil, false, errUnknown
	}
	name, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, false, errUnknown
	}
	i := f.findLabel(name)
	if i < 0 {
		return nil, false, fakeStatus(gh.ErrNotFound, "label "+name)
	}
	var p map[string]string
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, false, errUnknown
	}
	l := &f.s.Labels[i]
	if v, ok := p["new_name"]; ok {
		l.Name = v
	}
	if v, ok := p["color"]; ok {
		l.Color = v
	}
	if v, ok := p["description"]; ok {
		l.Description = v
	}
	return mustJSON(*l), true, nil
}

func (f *fakeGitHub) labelCmd(args []string) ([]byte, bool, error) {
	if repo, _ := fakeFlag(args, "--repo"); repo != f.fullName() {
		return nil, false, errUnknown
	}
	switch args[0] {
	case "list":
		if j, _ := fakeFlag(args, "--json"); j != "name,color,description" {
			return nil, false, errUnknown
		}
		return mustJSON(f.s.Labels), false, nil
	case "create":
		name := args[1]
		if f.findLabel(name) >= 0 {
			return nil, false, fakeStatus(gh.ErrValidation, "label "+name+" already exists")
		}
		color, _ := fakeFlag(args, "--color")
		desc, _ := fakeFlag(args, "--description")
		f.s.Labels = append(f.s.Labels, fakeLabel{Name: name, Color: color, Description: desc})
		return nil, true, nil
	case "delete":
		i := f.findLabel(args[1])
		if i < 0 || !slices.Contains(args, "--yes") {
			return nil, false, fakeStatus(gh.ErrNotFound, "label "+args[1])
		}
		f.s.Labels = append(f.s.Labels[:i], f.s.Labels[i+1:]...)
		return nil, true, nil
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) secretCmd(args []string) ([]byte, bool, error) {
	if repo, _ := fakeFlag(args, "--repo"); repo != f.fullName() {
		return nil, false, errUnknown
	}
	switch args[0] {
	case "list":
		if jq, _ := fakeFlag(args, "--jq"); jq != ".[].name" {
			return nil, false, errUnknown
		}
		names := make([]string, 0, len(f.s.Secrets))
		for n := range f.s.Secrets {
			names = append(names, n)
		}
		sort.Strings(names)
		return fakeLines(names), false, nil
	case "set":
		v, ok := fakeFlag(args, "--body")
		if !ok {
			return nil, false, errUnknown
		}
		f.s.Secrets[args[1]] = v
		return nil, true, nil
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) variableCmd(args []string) ([]byte, bool, error) {
	if repo, _ := fakeFlag(args, "--repo"); repo != f.fullName() {
		return nil, false, errUnknown
	}
	switch args[0] {
	case "list":
		if j, _ := fakeFlag(args, "--json"); j != "name,value" {
			return nil, false, errUnknown
		}
		var out []map[string]string
		for _, n := range fakeSortedKeys(f.s.Variables) {
			out = append(out, map[string]string{"name": n, "value": f.s.Variables[n]})
		}
		return mustJSON(out), false, nil
	case "set":
		v, ok := fakeFlag(args, "--body")
		if !ok {
			return nil, false, errUnknown
		}
		f.s.Variables[args[1]] = v
		return nil, true, nil
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) apiMilestone(sub, method string, body []byte) ([]byte, bool, error) {
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, false, errUnknown
	}
	apply := func(m *fakeMilestone) error {
		for k, v := range p {
			switch k {
			case "title", "description", "state":
				s, ok := v.(string)
				if !ok {
					return fakeStatus(gh.ErrValidation, k+" is not a string")
				}
				switch k {
				case "title":
					m.Title = s
				case "description":
					m.Description = s
				case "state":
					m.State = s
				}
			case "due_on":
				if v == nil {
					m.DueOn = nil // null clears
					continue
				}
				s, ok := v.(string)
				if !ok {
					return fakeStatus(gh.ErrValidation, "due_on is not a string")
				}
				m.DueOn = &s
			default:
				return errUnknown
			}
		}
		return nil
	}
	switch {
	case sub == "" && method == "POST":
		m := fakeMilestone{Number: len(f.s.Milestones) + 1, State: "open"}
		if err := apply(&m); err != nil {
			return nil, false, err
		}
		f.s.Milestones = append(f.s.Milestones, m)
		return mustJSON(m), true, nil
	case strings.HasPrefix(sub, "/") && method == "PATCH":
		n, err := strconv.Atoi(sub[1:])
		if err != nil {
			return nil, false, errUnknown
		}
		for i := range f.s.Milestones {
			if f.s.Milestones[i].Number == n {
				if err := apply(&f.s.Milestones[i]); err != nil {
					return nil, false, err
				}
				return mustJSON(f.s.Milestones[i]), true, nil
			}
		}
		return nil, false, fakeStatus(gh.ErrNotFound, "milestone "+sub[1:])
	}
	return nil, false, errUnknown
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func mustJSONMap(v any) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(mustJSON(v), &m); err != nil {
		panic(err)
	}
	return m
}

func fakeLines(ss []string) []byte {
	if len(ss) == 0 {
		return nil
	}
	return []byte(strings.Join(ss, "\n") + "\n")
}

func fakeRemove(ss []string, s string) []string {
	var out []string
	for _, x := range ss {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func fakeSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
