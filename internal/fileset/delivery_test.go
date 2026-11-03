package fileset

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func TestDeliveryFor_RendersPerRepository(t *testing.T) {
	opts := ApplyOptions{FileSetID: "owner/sync", Via: manifest.ViaPullRequest, CommitMessage: "chore: sync <% .Repo.Name %>"}
	a, err := DeliveryFor(opts, "owner/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := DeliveryFor(opts, "owner/web", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.CommitMessage != "chore: sync api" || b.CommitMessage != "chore: sync web" {
		t.Errorf("commit messages = %q, %q; want them rendered per repository", a.CommitMessage, b.CommitMessage)
	}
	if a.PRTitle != "chore: sync api" || b.PRTitle != "chore: sync web" {
		t.Errorf("PR titles = %q, %q; want the rendered commit message", a.PRTitle, b.PRTitle)
	}
	if a.Branch != b.Branch || a.PRBody != b.PRBody {
		t.Errorf("branch and body should not depend on the repository: %+v, %+v", a, b)
	}
}

func TestDeliveryFor_PushHasNoPullRequest(t *testing.T) {
	d, err := DeliveryFor(ApplyOptions{FileSetID: "owner/sync", Via: manifest.ViaPush, PRTitle: "<% .Repo.Name %>"}, "owner/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Branch != "" || d.PRTitle != "" || d.PRBody != "" {
		t.Errorf("push should carry no pull request, got %+v", d)
	}
}

// TestDeliveryFor_MatchesApply checks DeliveryFor against the pull request
// apply opens, so the two cannot drift apart.
func TestDeliveryFor_MatchesApply(t *testing.T) {
	changes := []Change{
		{Target: "owner/repo", Path: "Justfile", Type: ChangeUpdate, Desired: "build:\n", Entry: "Justfile", Description: "Tasks for <% .Repo.Name %>"},
		{Target: "owner/repo", Path: ".github/a.yml", Type: ChangeCreate, Desired: "a\n", Entry: ".github", EntryIndex: 1},
	}
	tests := []struct {
		name string
		opts ApplyOptions
	}{
		{"templated", ApplyOptions{
			FileSetID: "owner/sync", Via: manifest.ViaPullRequest, Branch: "sync/<% .Repo.Name %>",
			PRTitle: "Sync <% .Repo.FullName %>", PRBody: "Files for <% .Repo.Name %>",
		}},
		{"default", ApplyOptions{FileSetID: "owner/sync", Via: manifest.ViaPullRequest}},
		{"files", ApplyOptions{
			FileSetID: "owner/sync", Via: manifest.ViaPullRequest,
			PRTitle: "<% (index .Files 0).Action %> <% (index .Files 0).Path %>",
			PRBody:  "<% range .Files %>- <% .Action %> <% .Path %>: <% .Description %>\n<% end %>",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newNoopGuardMock("new-tree")
			applyChanges(t, mock, changes, tt.opts)

			got := map[string]string{}
			for _, call := range mock.Called {
				if len(call) < 2 || call[0] != "pr" || call[1] != "create" {
					continue
				}
				for i := 2; i+1 < len(call); i++ {
					got[call[i]] = call[i+1]
				}
			}
			want, err := DeliveryFor(tt.opts, "owner/repo", changes)
			if err != nil {
				t.Fatal(err)
			}
			if got["--title"] != want.PRTitle || got["--body"] != want.PRBody || got["--head"] != want.Branch {
				t.Errorf("apply opened title %q, body %q, head %q; DeliveryFor says %q, %q, %q",
					got["--title"], got["--body"], got["--head"], want.PRTitle, want.PRBody, want.Branch)
			}
		})
	}
}
