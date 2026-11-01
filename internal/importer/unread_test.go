package importer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

// fullState is a repository with every setting import reads.
func fullState() *repository.CurrentState {
	s := rtState()
	withEnvironments(s)
	s.Variables = map[string]string{"ENV": "prod"}
	s.Milestones = map[string]*repository.CurrentMilestone{"v1": {Number: 1, Title: "v1", State: "closed", DueOn: "2025-12-31"}}
	s.BranchProtection = map[string]*repository.CurrentBranchProtection{"main": {Pattern: "main", RequiredReviews: 1}}
	s.Rulesets = map[string]*repository.CurrentRuleset{"main": {ID: 1, Name: "main", Target: "branch", Enforcement: "active",
		Conditions: &repository.CurrentRulesetConditions{RefName: &repository.CurrentRulesetRefCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}}}}}
	return s
}

// unreadCases leave one setting of fullState unread, as FetchRepository
// does: unset, and named in Unread.
var unreadCases = []struct {
	key   string
	unset func(*repository.CurrentState)
}{
	{"labels", func(s *repository.CurrentState) { s.Labels = nil }},
	{"variables", func(s *repository.CurrentState) { s.Variables = nil }},
	{"milestones", func(s *repository.CurrentState) { s.Milestones = nil }},
	{"rulesets", func(s *repository.CurrentState) { s.Rulesets = nil }},
	{"branch_protection", func(s *repository.CurrentState) { s.BranchProtection = nil }},
	{"environments", func(s *repository.CurrentState) { s.Environments = nil }},
	{"actions", func(s *repository.CurrentState) { s.Actions = repository.CurrentActions{} }},
	{"environments.production.variables", func(s *repository.CurrentState) { s.Environments["production"].Variables = nil }},
	{"environments.production.deployment_branch_policy", func(s *repository.CurrentState) { s.Environments["production"].BranchRules = nil }},
}

func hasWarning(ws []RepoWarning, field, message string) bool {
	for _, w := range ws {
		if w.Field == field && strings.Contains(w.Message, message) {
			return true
		}
	}
	return false
}

// import --into keeps the manifest's value of a setting that could not be
// read, and says so, rather than removing it as if GitHub had none.
func TestImportIntoRepositoryKeepsUnread(t *testing.T) {
	ctx := context.Background()
	resolver := manifest.NewResolver(rtRunner{})
	for _, tt := range unreadCases {
		t.Run(tt.key, func(t *testing.T) {
			path, data := writeYAML(t, repository.ToManifest(ctx, fullState(), resolver))
			parsed, err := manifest.ParseAll(path)
			if err != nil {
				t.Fatal(err)
			}
			current := fullState()
			tt.unset(current)
			unread := map[string]error{tt.key: fmt.Errorf("fetch: %w", gh.ErrNotFound)}
			current.Unread = unread
			imported := repository.ToManifest(ctx, current, resolver)

			rp, err := DiffRepository(DiffInput{Repos: parsed.RepositoryDocs, Imported: imported, ManifestBytes: map[string][]byte{path: data}, Unread: unread})
			if err != nil {
				t.Fatal(err)
			}
			if len(rp.Diffs) > 0 || len(rp.ManifestEdits) > 0 {
				t.Errorf("changed the manifest from what could not be read: %+v", rp.Diffs)
			}
			if !hasWarning(rp.Warnings, tt.key, "could not be read (HTTP 404); left as in the manifest") {
				t.Errorf("no warning for %s: %+v", tt.key, rp.Warnings)
			}

			// What GitHub does not have is still removed. Environments that
			// are nil were not read (#296), so none is an empty map.
			current.Unread = nil
			if tt.key == "environments" {
				current.Environments = map[string]*repository.CurrentEnvironment{}
			}
			none, err := DiffRepository(DiffInput{Repos: parsed.RepositoryDocs, Imported: repository.ToManifest(ctx, current, resolver), ManifestBytes: map[string][]byte{path: data}})
			if err != nil {
				t.Fatal(err)
			}
			if len(none.ManifestEdits) == 0 {
				t.Errorf("read as none, %s should change the manifest", tt.key)
			}
		})
	}
}

// In a RepositorySet entry, a setting that could not be read keeps the
// entry's override, and the defaults are not compared with it.
func TestImportIntoRepositorySetKeepsUnread(t *testing.T) {
	ctx := context.Background()
	resolver := manifest.NewResolver(rtRunner{})
	base := repository.ToManifest(ctx, fullState(), resolver)
	for _, tt := range []struct {
		name  string
		entry *manifest.RepositorySpec
	}{
		// `labels: []` opts the entry out of the defaults' labels.
		{name: "entry opts out of the defaults", entry: &manifest.RepositorySpec{LabelsSet: true}},
		{name: "entry inherits the defaults"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entry := map[string]any{"name": "r"}
			if tt.entry != nil {
				entry["spec"] = map[string]any{"labels": []any{}}
			}
			set := map[string]any{
				"apiVersion":   manifest.APIVersion,
				"kind":         manifest.KindRepositorySet,
				"metadata":     map[string]any{"owner": "o"},
				"defaults":     map[string]any{"spec": manifest.RepositorySpec{Labels: append([]manifest.Label{{Name: "extra", Color: "ffffff"}}, base.Spec.Labels...)}},
				"repositories": []map[string]any{entry},
			}
			path, data := writeYAML(t, set)
			parsed, err := manifest.ParseAll(path)
			if err != nil {
				t.Fatalf("%v\n%s", err, data)
			}
			current := fullState()
			current.Labels = nil
			unread := map[string]error{"labels": fmt.Errorf("fetch: %w", gh.ErrForbidden)}
			current.Unread = unread

			rp, err := DiffRepositorySet(DiffInput{Repos: parsed.RepositoryDocs, Imported: repository.ToManifest(ctx, current, resolver),
				ManifestBytes: map[string][]byte{path: data}, Unread: unread})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range rp.Diffs {
				if strings.HasPrefix(d.Field, "labels") {
					t.Errorf("changed the entry's labels from what could not be read: %+v", d)
				}
			}
			if !hasWarning(rp.Warnings, "labels", "could not be read (HTTP 403)") {
				t.Errorf("no warning for labels: %+v", rp.Warnings)
			}
			if hasWarning(rp.Warnings, "labels", "in defaults but not on GitHub") {
				t.Errorf("compared the defaults with labels that could not be read: %+v", rp.Warnings)
			}

			// Read as none, the labels change an entry that inherits the
			// defaults (an entry that opts out has none either way).
			if tt.entry != nil {
				return
			}
			current.Unread = nil
			none, err := DiffRepositorySet(DiffInput{Repos: parsed.RepositoryDocs, Imported: repository.ToManifest(ctx, current, resolver),
				ManifestBytes: map[string][]byte{path: data}})
			if err != nil {
				t.Fatal(err)
			}
			changed := false
			for _, d := range none.Diffs {
				changed = changed || strings.HasPrefix(d.Field, "labels")
			}
			if !changed && !hasWarning(none.Warnings, "labels", "in defaults but not on GitHub") {
				t.Errorf("read as none, the labels should change the entry or warn: %+v %+v", none.Diffs, none.Warnings)
			}
		})
	}
}

// An entry that adds to the defaults' list keeps its own items only: the
// list that could not be read is not rewritten as the merged one.
func TestImportIntoRepositorySetKeepsUnreadOverride(t *testing.T) {
	ctx := context.Background()
	resolver := manifest.NewResolver(rtRunner{})
	set := map[string]any{
		"apiVersion": manifest.APIVersion,
		"kind":       manifest.KindRepositorySet,
		"metadata":   map[string]any{"owner": "o"},
		"defaults": map[string]any{"spec": map[string]any{
			"variables":    []map[string]any{{"name": "GLOBAL", "value": "old"}},
			"environments": []map[string]any{{"name": "production", "variables": []map[string]any{{"name": "TIER", "value": "1"}}}},
		}},
		"repositories": []map[string]any{{"name": "r", "spec": map[string]any{
			"variables":    []map[string]any{{"name": "REGION", "value": "jp"}},
			"environments": []map[string]any{{"name": "production", "variables": []map[string]any{{"name": "REGION", "value": "eu"}}}},
		}}},
	}
	path, data := writeYAML(t, set)
	parsed, err := manifest.ParseAll(path)
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	for _, key := range []string{"variables", "environments.production.variables"} {
		t.Run(key, func(t *testing.T) {
			current := fullState()
			for _, tt := range unreadCases {
				if tt.key == key {
					tt.unset(current)
				}
			}
			unread := map[string]error{key: fmt.Errorf("fetch: %w", gh.ErrNotFound)}
			current.Unread = unread
			rp, err := DiffRepositorySet(DiffInput{Repos: parsed.RepositoryDocs, Imported: repository.ToManifest(ctx, current, resolver),
				ManifestBytes: map[string][]byte{path: data}, Unread: unread})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range rp.Diffs {
				if strings.HasPrefix(d.Field, key) {
					t.Errorf("changed the entry's %s, which could not be read: %+v", key, d)
				}
			}
		})
	}
}
