package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// scopeResponses answers the JSON settings endpoints, which fail on the
// mock's empty default output.
func scopeResponses(visibility string) map[string][]byte {
	return map[string][]byte{
		repoViewKey: []byte(`{"visibility":"` + visibility + `"}`),
		"api repos/myorg/myrepo --jq {squash_merge_commit_title,squash_merge_commit_message,merge_commit_title,merge_commit_message,allow_auto_merge,has_pull_requests,pull_request_creation_policy}": []byte(`{}`),
		"api repos/myorg/myrepo/immutable-releases":              []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/automated-security-fixes":        []byte(`{"enabled": false}`),
		"api repos/myorg/myrepo/private-vulnerability-reporting": []byte(`{"enabled": false}`),
	}
}

const repoViewKey = "repo view myorg/myrepo --json description,homepageUrl,visibility,isArchived,repositoryTopics,hasIssuesEnabled,hasProjectsEnabled,hasWikiEnabled,hasDiscussionsEnabled,mergeCommitAllowed,squashMergeAllowed,rebaseMergeAllowed,deleteBranchOnMerge,defaultBranchRef"

// fetchedGroups names the settings groups FetchRepository asked GitHub for.
func fetchedGroups(called [][]string) []string {
	var groups []string
	add := func(g string) {
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	for _, args := range called {
		c := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(c, "repo view"):
		case strings.Contains(c, "branches?protected"):
			add("branch_protection")
		case strings.Contains(c, "/rulesets"):
			add("rulesets")
		case strings.HasPrefix(c, "secret list"):
			add("secrets")
		case strings.HasPrefix(c, "variable list"):
			add("variables")
		case strings.HasPrefix(c, "label list"):
			add("labels")
		case strings.Contains(c, "/milestones"):
			add("milestones")
		case strings.Contains(c, "/actions/permissions"):
			add("actions")
		case strings.Contains(c, "allow_auto_merge"):
			add("commit_message_settings")
		case strings.Contains(c, "immutable-releases"):
			add("release_immutability")
		case strings.Contains(c, "vulnerability-alerts"):
			add("vulnerability_alerts")
		case strings.Contains(c, "automated-security-fixes"):
			add("automated_security_fixes")
		case strings.Contains(c, "private-vulnerability-reporting"):
			add("private_vulnerability_reporting")
		default:
			add("unknown: " + c)
		}
	}
	slices.Sort(groups)
	return groups
}

func TestFetchRepository_FetchesOnlyManagedSettings(t *testing.T) {
	yes := manifest.Ptr(true)
	for _, tt := range []struct {
		name string
		spec manifest.RepositorySpec
		want []string
	}{
		{"description only", manifest.RepositorySpec{Description: manifest.Ptr("x")}, nil},
		{"labels", manifest.RepositorySpec{Labels: []manifest.Label{{Name: "bug", Color: "d73a4a"}}}, []string{"labels"}},
		{"empty secrets only opt out", manifest.RepositorySpec{Secrets: []manifest.Secret{}, SecretsSet: true}, nil},
		{"secrets", manifest.RepositorySpec{Secrets: []manifest.Secret{{Name: "S", Value: "v"}}}, []string{"secrets"}},
		{"rulesets []", manifest.RepositorySpec{RulesetsSet: true}, []string{"rulesets"}},
		{"actions", manifest.RepositorySpec{Actions: &manifest.Actions{Enabled: yes}}, []string{"actions"}},
		{"merge commits", manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{AllowMergeCommit: yes}}, nil},
		{"auto merge", manifest.RepositorySpec{MergeStrategy: &manifest.MergeStrategy{AllowAutoMerge: yes}}, []string{"commit_message_settings"}},
		{"pull requests", manifest.RepositorySpec{Features: &manifest.Features{PullRequests: &manifest.PullRequests{Enabled: yes}}}, []string{"commit_message_settings"}},
		{"wiki", manifest.RepositorySpec{Features: &manifest.Features{Wiki: yes}}, nil},
		{"automated security fixes need alerts", manifest.RepositorySpec{Security: &manifest.Security{AutomatedSecurityFixes: yes}},
			[]string{"automated_security_fixes", "vulnerability_alerts"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{Responses: scopeResponses("PUBLIC")}
			p := NewProcessor(mock, nil)
			spec := tt.spec
			if _, err := p.FetchRepository(context.Background(), "myorg", "myrepo", &spec, nil); err != nil {
				t.Fatal(err)
			}
			if got := fetchedGroups(mock.Called); !slices.Equal(got, tt.want) {
				t.Errorf("fetched %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("import fetches everything", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: scopeResponses("PUBLIC")}
		if _, err := NewProcessor(mock, nil).FetchRepository(context.Background(), "myorg", "myrepo", nil, nil); err != nil {
			t.Fatal(err)
		}
		if got := fetchedGroups(mock.Called); len(got) != 12 {
			t.Errorf("fetched %v, want all 12 groups", got)
		}
	})
}

func forbidden(msg string) error {
	return fmt.Errorf("%w: %s", gh.ErrForbidden, msg)
}

func TestFetchRepository_ForbiddenManagedSettingIsAnError(t *testing.T) {
	secretKey := "secret list --repo myorg/myrepo --json name --jq .[].name"
	rulesetKey := "api repos/myorg/myrepo/rulesets --paginate"
	protectedKey := "api repos/myorg/myrepo/branches?protected=true&per_page=100 --paginate --jq .[].name"
	ruleKey := "api repos/myorg/myrepo/branches/main/protection"
	errs := map[string]error{
		secretKey:  forbidden("Resource not accessible by personal access token"),
		rulesetKey: forbidden("Upgrade to GitHub Pro or make this repository public to enable this feature."),
		ruleKey:    forbidden("Resource not accessible by personal access token"),
	}
	responses := scopeResponses("PRIVATE")
	responses[protectedKey] = []byte("main\n")

	for _, tt := range []struct {
		name string
		spec manifest.RepositorySpec
		want string
	}{
		{"secrets", manifest.RepositorySpec{Secrets: []manifest.Secret{{Name: "S", Value: "v"}}}, "cannot read secrets of myorg/myrepo (the token needs the Secrets permission)"},
		{"rulesets on a Free private repository", manifest.RepositorySpec{RulesetsSet: true}, "Upgrade to GitHub Pro"},
		{"branch protection rule", manifest.RepositorySpec{BranchProtectionSet: true}, "cannot read branch protection of myorg/myrepo (the token needs the Administration permission)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock := &gh.MockRunner{Responses: responses, Errors: errs}
			spec := tt.spec
			_, err := NewProcessor(mock, nil).FetchRepository(context.Background(), "myorg", "myrepo", &spec, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}

	t.Run("import ignores the same 403s", func(t *testing.T) {
		mock := &gh.MockRunner{Responses: responses, Errors: errs}
		state, err := NewProcessor(mock, nil).FetchRepository(context.Background(), "myorg", "myrepo", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Secrets) != 0 || len(state.Rulesets) != 0 || len(state.BranchProtection) != 0 {
			t.Errorf("unreadable settings should be empty for import: %+v", state)
		}
	})

	if !errors.Is(forbidden("x"), gh.ErrForbidden) {
		t.Fatal("helper")
	}
}
