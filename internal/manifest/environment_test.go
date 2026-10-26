package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func parseEnvironmentsYAML(t *testing.T, spec string) ([]*Repository, error) {
	t.Helper()
	content := "apiVersion: gh-infra/v1\nkind: Repository\nmetadata:\n  owner: org\n  name: repo\nspec:\n" + spec
	path := filepath.Join(t.TempDir(), "repo.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return ParsePath(path)
}

func TestEnvironment_DeploymentBranchPolicyModes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		policy   string
		wantNil  bool
		wantMode string
	}{
		{name: "left out", policy: "", wantNil: true},
		{name: "all branches", policy: "      deployment_branch_policy: {}\n", wantMode: DeploymentBranchesAll},
		{name: "protected branches", policy: "      deployment_branch_policy:\n        protected_branches: true\n", wantMode: DeploymentBranchesProtected},
		{name: "patterns", policy: "      deployment_branch_policy:\n        custom_branch_policies:\n          - name: main\n", wantMode: DeploymentBranchesCustom},
		{name: "no pattern", policy: "      deployment_branch_policy:\n        custom_branch_policies: []\n", wantMode: DeploymentBranchesCustom},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repos, err := parseEnvironmentsYAML(t, "  environments:\n    - name: production\n"+tt.policy)
			if err != nil {
				t.Fatal(err)
			}
			p := repos[0].Spec.Environments[0].DeploymentBranchPolicy
			if tt.wantNil {
				if p != nil {
					t.Fatalf("policy = %+v, want nil", p)
				}
				return
			}
			if p == nil {
				t.Fatal("policy is nil")
			}
			if got := p.Mode(); got != tt.wantMode {
				t.Errorf("Mode() = %q, want %q", got, tt.wantMode)
			}
		})
	}
}

func TestEnvironment_Invalid(t *testing.T) {
	for _, tt := range []struct {
		name    string
		env     string
		wantErr string
	}{
		{name: "null policy", env: "      deployment_branch_policy:\n", wantErr: "use {} to allow all branches"},
		{name: "null patterns", env: "      deployment_branch_policy:\n        custom_branch_policies:\n", wantErr: "use [] to allow no branch"},
		{name: "null reviewers", env: "      reviewers:\n", wantErr: "use [] to remove all reviewers"},
		{name: "both policies", env: "      deployment_branch_policy:\n        protected_branches: true\n        custom_branch_policies: []\n", wantErr: "cannot have both"},
		{name: "protected false", env: "      deployment_branch_policy:\n        protected_branches: false\n", wantErr: "deployment_branch_policy: {}"},
		{name: "pattern type", env: "      deployment_branch_policy:\n        custom_branch_policies:\n          - name: main\n            type: ref\n", wantErr: "type"},
		{name: "duplicate pattern", env: "      deployment_branch_policy:\n        custom_branch_policies:\n          - name: main\n          - name: main\n            type: branch\n", wantErr: `duplicate branch pattern "main"`},
		{name: "wait timer too long", env: "      wait_timer: 43201\n", wantErr: "between 0 and 43200"},
		{name: "reviewer with both", env: "      reviewers:\n        - user: a\n          team: b\n", wantErr: "exactly one of: user or team"},
		{name: "reviewer with neither", env: "      reviewers:\n        - {}\n", wantErr: "exactly one of: user or team"},
		{name: "reviewer twice", env: "      reviewers:\n        - user: Alice\n        - user: alice\n", wantErr: "listed twice"},
		{name: "too many reviewers", env: "      reviewers:\n        - user: a\n        - user: b\n        - user: c\n        - user: d\n        - user: e\n        - user: f\n        - user: g\n", wantErr: "at most 6"},
		{name: "null variables", env: "      variables:\n", wantErr: "variables must be a sequence"},
		{name: "duplicate secret", env: "      secrets:\n        - name: A\n          value: x\n        - name: A\n          value: y\n", wantErr: "duplicate name"},
		{name: "secret without a name", env: "      secrets:\n        - value: x\n", wantErr: "name is required"},
		{name: "self review without reviewers", env: "      reviewers: []\n      prevent_self_review: true\n", wantErr: "needs at least one reviewer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEnvironmentsYAML(t, "  environments:\n    - name: production\n"+tt.env)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestEnvironment_NamesAreCaseInsensitive(t *testing.T) {
	_, err := parseEnvironmentsYAML(t, "  environments:\n    - name: production\n    - name: Production\n")
	if err == nil || !strings.Contains(err.Error(), "the same environment") {
		t.Fatalf("err = %v, want a duplicate environment error", err)
	}
}

func TestEnvironment_NullEnvironments(t *testing.T) {
	_, err := parseEnvironmentsYAML(t, "  environments:\n")
	if err == nil || !strings.Contains(err.Error(), "environments must be a sequence") {
		t.Fatalf("err = %v", err)
	}
}

// An explicit [] is kept when written back: custom_branch_policies: [] allows
// no branch, but without it {} allows every branch.
func TestEnvironment_MarshalKeepsEmptyLists(t *testing.T) {
	repos, err := parseEnvironmentsYAML(t, "  environments:\n    - name: production\n      reviewers: []\n      deployment_branch_policy:\n        custom_branch_policies: []\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(repos[0].Spec.Environments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "custom_branch_policies: []") || !strings.Contains(string(out), "reviewers: []") {
		t.Fatalf("marshaled:\n%s", out)
	}
	var back []Environment
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if got := back[0].DeploymentBranchPolicy.Mode(); got != DeploymentBranchesCustom {
		t.Errorf("mode after round trip = %q, want custom", got)
	}
	if !back[0].ReviewersSet {
		t.Error("reviewers: [] lost in round trip")
	}
}

func TestRepositorySet_EnvironmentsMerge(t *testing.T) {
	dir := t.TempDir()
	content := `
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: org
defaults:
  reconcile:
    deployment_branch_policies: authoritative
  spec:
    environments:
      - name: production
        wait_timer: 5
        reviewers:
          - team: deployers
        deployment_branch_policy:
          custom_branch_policies:
            - name: main
            - name: release/*
        variables:
          - name: REGION
            value: us
        secrets:
          - name: TOKEN
            value: default
      - name: staging
repositories:
  - name: inherits
  - name: overrides
    spec:
      environments:
        - name: Production
          can_admins_bypass: false
          reviewers: []
          deployment_branch_policy: {}
          variables:
            - name: REGION
              value: eu
            - name: TIER
              value: prod
          secrets: []
        - name: preview
  - name: none
    spec:
      environments: []
`
	path := filepath.Join(dir, "set.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	repos, err := ParsePath(path)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*Repository{}
	for _, r := range repos {
		byName[r.Metadata.Name] = r
	}

	if got := byName["inherits"].Spec.Environments; len(got) != 2 || len(got[0].DeploymentBranchPolicy.CustomBranchPolicies) != 2 {
		t.Errorf("inherits: environments = %+v", got)
	}
	if got := DeploymentBranchPoliciesReconcileMode(byName["inherits"].Reconcile); got != CollectionReconcileAuthoritative {
		t.Errorf("reconcile = %q", got)
	}

	envs := byName["overrides"].Spec.Environments
	if len(envs) != 3 {
		t.Fatalf("overrides: %d environments, want 3: %+v", len(envs), envs)
	}
	prod := envs[0]
	if prod.Name != "Production" || prod.WaitTimer == nil || *prod.WaitTimer != 5 || prod.CanAdminsBypass == nil || *prod.CanAdminsBypass {
		t.Errorf("production not merged field by field: %+v", prod)
	}
	if !prod.HasReviewers() || len(prod.Reviewers) != 0 {
		t.Errorf("reviewers: [] did not replace the defaults: %+v", prod.Reviewers)
	}
	if prod.DeploymentBranchPolicy.Mode() != DeploymentBranchesAll {
		t.Errorf("deployment_branch_policy: {} did not replace the defaults' patterns: %+v", prod.DeploymentBranchPolicy)
	}
	if len(prod.Variables) != 2 || prod.Variables[0].Value != "eu" || prod.Variables[1].Name != "TIER" {
		t.Errorf("variables not merged by name: %+v", prod.Variables)
	}
	if !prod.SecretsSet || len(prod.Secrets) != 0 {
		t.Errorf("secrets: [] inherited the defaults: %+v", prod.Secrets)
	}
	if got := byName["inherits"].Spec.Environments[0].Secrets; len(got) != 1 || got[0].Value != "default" {
		t.Errorf("inherits: secrets = %+v", got)
	}
	if envs[2].Name != "preview" {
		t.Errorf("new environment not appended: %+v", envs[2])
	}

	if got := byName["none"].Spec.Environments; len(got) != 0 || byName["none"].Spec.ManagesEnvironments() {
		t.Errorf("none: environments = %+v", got)
	}
}

func TestResolveSecrets_Environments(t *testing.T) {
	t.Setenv("ENV_PROD_TOKEN", "p")
	repos := []*Repository{{
		Metadata: RepositoryMetadata{Name: "repo", Owner: "org"},
		Spec: RepositorySpec{Environments: []Environment{{
			Name:    "production",
			Secrets: []Secret{{Name: "TOKEN", Value: "${ENV_PROD_TOKEN}"}, {Name: "LEAK", Value: "${GH_TOKEN}"}},
		}}},
	}}
	err := ResolveSecrets(repos)
	if err == nil || !strings.Contains(err.Error(), `environment "production": secret "LEAK"`) {
		t.Fatalf("err = %v, want the environment's secret named", err)
	}
	if got := repos[0].Spec.Environments[0].Secrets[0].Value; got != "p" {
		t.Errorf("TOKEN = %q, want it expanded", got)
	}
}
