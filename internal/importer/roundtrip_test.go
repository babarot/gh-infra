package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

// The round trip behind `import --into`: importing a repository into a
// manifest that already matches it changes nothing, and after importing into
// a RepositorySet entry, plan finds no changes.

// rtRunner answers the resolver's name lookups for owner o.
type rtRunner struct{}

func (rtRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	cmd := strings.Join(args, " ")
	for _, r := range [][2]string{
		{"orgs/o/teams/platform", `{"id":11}`},
		{"orgs/o/teams", `[{"id":11,"slug":"platform"}]`},
		{"apps/ci", `{"id":7}`},
		{"commits/HEAD/check-runs", `[{"app":{"id":7,"slug":"ci"}}]`},
	} {
		if strings.Contains(cmd, r[0]) {
			return []byte(r[1]), nil
		}
	}
	return nil, nil
}

func (r rtRunner) RunWithStdin(ctx context.Context, _ []byte, args ...string) ([]byte, error) {
	return r.Run(ctx, args...)
}

func rtState() *repository.CurrentState {
	return &repository.CurrentState{
		Owner: "o", Name: "r", Description: "a repository", Visibility: "public", Topics: []string{"go"},
		Features: repository.CurrentFeatures{Issues: true, Wiki: true, PullRequests: true, PullRequestCreation: manifest.PullRequestCreationAll},
		MergeStrategy: repository.CurrentMergeStrategy{AllowMergeCommit: true, AllowSquashMerge: true, AutoDeleteHeadBranches: true,
			MergeCommitTitle: "PR_TITLE", MergeCommitMessage: "PR_BODY", SquashMergeCommitTitle: "PR_TITLE", SquashMergeCommitMessage: "PR_BODY"},
		Security: repository.CurrentSecurity{VulnerabilityAlerts: true},
		Actions:  repository.CurrentActions{Enabled: true, AllowedActions: "all", WorkflowPermissions: "read"},
		Labels:   map[string]*repository.CurrentLabel{"bug": {Name: "bug", Color: "d73a4a", Description: "a bug"}},
	}
}

func writeYAML(t *testing.T, v any) (string, []byte) {
	t.Helper()
	data, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestRoundTrip_ImportIntoRepository(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*repository.CurrentState)
	}{
		{"base", func(*repository.CurrentState) {}},
		{"branch protection with no required checks", func(s *repository.CurrentState) {
			s.BranchProtection = map[string]*repository.CurrentBranchProtection{
				"main": {Pattern: "main", RequiredReviews: 1, RequireStatusChecks: &repository.CurrentStatusChecks{Strict: true, Contexts: []string{}}},
			}
		}},
		{"branch protection with nil required checks", func(s *repository.CurrentState) {
			s.BranchProtection = map[string]*repository.CurrentBranchProtection{
				"main": {Pattern: "main", RequireStatusChecks: &repository.CurrentStatusChecks{Strict: true}},
			}
		}},
		{"ruleset with no required checks", func(s *repository.CurrentState) {
			s.Rulesets = map[string]*repository.CurrentRuleset{"main": {ID: 1, Name: "main", Target: "branch", Enforcement: "active",
				Conditions: &repository.CurrentRulesetConditions{RefName: &repository.CurrentRulesetRefCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}}},
				Rules:      repository.CurrentRulesetRules{RequiredStatusChecks: &repository.CurrentRulesetStatusChecks{Contexts: []repository.CurrentRulesetStatusCheck{}}}}}
		}},
		{"pull requests disabled for collaborators", func(s *repository.CurrentState) {
			s.Features.PullRequests = false
			s.Features.PullRequestCreation = manifest.PullRequestCreationCollaboratorsOnly
		}},
		{"milestones and variables", func(s *repository.CurrentState) {
			s.Milestones = map[string]*repository.CurrentMilestone{"v1": {Number: 1, Title: "v1", State: "closed", DueOn: "2025-12-31"}}
			s.Variables = map[string]string{"ENV": "prod"}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current := rtState()
			tt.mutate(current)
			m := repository.ToManifest(context.Background(), current, manifest.NewResolver(rtRunner{}))
			path, data := writeYAML(t, m)
			parsed, err := manifest.ParseAll(path)
			if err != nil {
				t.Fatal(err)
			}
			rp, err := DiffRepository(DiffInput{Repos: parsed.RepositoryDocs, Imported: m, ManifestBytes: map[string][]byte{path: data}})
			if err != nil {
				t.Fatal(err)
			}
			if len(rp.Diffs) > 0 || len(rp.ManifestEdits) > 0 {
				t.Errorf("importing into a matching manifest reports %+v\n%s", rp.Diffs, data)
			}
		})
	}
}

func TestRoundTrip_ImportIntoRepositorySet(t *testing.T) {
	ctx := context.Background()
	resolver := manifest.NewResolver(rtRunner{})
	current := rtState()
	imported := repository.ToManifest(ctx, current, resolver)

	for _, tt := range []struct {
		name     string
		defaults *manifest.RepositorySpec // nil: a RepositorySet without defaults
		skip     string
		wantPlan []string // fields the plan still changes after import
		wantWarn string   // field a warning names
	}{
		{name: "defaults match GitHub", defaults: &manifest.RepositorySpec{
			Description: imported.Spec.Description, Features: imported.Spec.Features, MergeStrategy: imported.Spec.MergeStrategy, Labels: imported.Spec.Labels}},
		{name: "defaults differ in features and merge strategy", defaults: &manifest.RepositorySpec{
			Features: &manifest.Features{Wiki: manifest.Ptr(false), Discussions: manifest.Ptr(true)}, MergeStrategy: &manifest.MergeStrategy{AllowRebaseMerge: manifest.Ptr(true)}}},
		{name: "defaults have a label GitHub does not", defaults: &manifest.RepositorySpec{
			Labels: append([]manifest.Label{{Name: "extra", Color: "ffffff"}}, imported.Spec.Labels...)},
			wantPlan: []string{"extra"}, wantWarn: "labels"},
		{name: "defaults differ in security and release immutability", skip: "#235: RepositorySet import does not compare security, release_immutability or milestones",
			defaults: &manifest.RepositorySpec{Security: &manifest.Security{VulnerabilityAlerts: manifest.Ptr(false)}, ReleaseImmutability: manifest.Ptr(true)}},
		{name: "no defaults", skip: "#236: import --into writes nothing for RepositorySet entries without defaults"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			set := map[string]any{
				"apiVersion":   manifest.APIVersion,
				"kind":         manifest.KindRepositorySet,
				"metadata":     map[string]any{"owner": "o"},
				"repositories": []map[string]any{{"name": "r"}},
			}
			if tt.defaults != nil {
				set["defaults"] = map[string]any{"spec": tt.defaults}
			}
			path, data := writeYAML(t, set)
			parsed, err := manifest.ParseAll(path)
			if err != nil {
				t.Fatalf("%v\n%s", err, data)
			}

			rp, err := DiffRepositorySet(DiffInput{Repos: parsed.RepositoryDocs, Imported: imported, ManifestBytes: map[string][]byte{path: data}})
			if err != nil {
				t.Fatal(err)
			}
			// Guard against passing because nothing happened: either import
			// wrote the entry, or the defaults already match GitHub.
			if len(rp.ManifestEdits) == 0 && len(compareSpecs(*tt.defaults, imported.Spec)) > 0 {
				// compareSpecs also lists what the defaults leave unset, so
				// only a write proves the entry was handled.
				t.Fatalf("import wrote nothing although the defaults differ from GitHub")
			}
			if tt.wantWarn != "" {
				found := false
				for _, w := range rp.Warnings {
					found = found || w.Field == tt.wantWarn
				}
				if !found {
					t.Errorf("want a %s warning, got %+v", tt.wantWarn, rp.Warnings)
				}
			}
			if edited, ok := rp.ManifestEdits[path]; ok {
				data = edited
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			reparsed, err := manifest.ParseAll(path)
			if err != nil {
				t.Fatalf("%v\n%s", err, data)
			}
			var changed []string
			for _, c := range repository.Diff(ctx, reparsed.Repositories[0], current, repository.DiffOptions{Resolver: resolver.For("o")}) {
				changed = append(changed, c.Field)
			}
			if strings.Join(changed, ",") != strings.Join(tt.wantPlan, ",") {
				t.Errorf("plan after import changes %v, want %v\n%s", changed, tt.wantPlan, data)
			}

			again, err := DiffRepositorySet(DiffInput{Repos: reparsed.RepositoryDocs, Imported: imported, ManifestBytes: map[string][]byte{path: data}})
			if err != nil {
				t.Fatal(err)
			}
			if len(again.Diffs) > 0 {
				t.Errorf("a second import still reports %+v", again.Diffs)
			}
		})
	}
}
