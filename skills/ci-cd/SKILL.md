---
name: ci-cd
description: >
  CI/CD integration for gh-infra in GitHub Actions: plan each pull request and
  post the plan as a comment, apply on merge, scheduled drift detection with
  notification, --changed-since and --detailed-exitcode, self-managed vs
  central-management layouts, and which token each needs.
---

# CI/CD Integration

Use this skill when the user wants GitHub Actions workflows around gh-infra.

## Recommended Setup

Three workflows; full YAML in [references/workflows.md](./references/workflows.md). Copy them and change only the paths and the secret name.

| Workflow | Trigger | Command |
|---|---|---|
| Plan on pull request | `pull_request` | `gh infra plan <paths> --changed-since origin/${{ github.base_ref }} --detailed-exitcode -o markdown > plan.md`, then `gh pr comment --body-file plan.md --edit-last --create-if-none` |
| Apply on merge | `push` to main, `workflow_dispatch` | `gh infra apply <paths> --changed-since "$BEFORE" --auto-approve`; without `--changed-since` when `github.event.before` is empty or all zeros |
| Drift detection | `schedule`, `workflow_dispatch` | `gh infra plan <paths> --detailed-exitcode -o markdown > plan.md` (no `--changed-since`), notify on exit 2 |

Optional fourth: pull changes made on GitHub into the manifests on a schedule with `gh infra import --into=<path> --auto-approve` and open a pull request (also in workflows.md).

Use the three unless the user wants less. Drift detection is also the safety net for apply runs that failed: a failed `apply --changed-since` is not retried by the next push.

## Rules For Every Workflow

- install the extension: `- run: gh extension install babarot/gh-infra` after `actions/checkout`, with `GH_TOKEN` set at job level
- pass every manifest directory to one command: `gh infra plan ./repos/ ./files/`. One exit status, and cross-manifest validation (shared PR branch) covers all of them. Do not run one command per directory: with `--detailed-exitcode` the first exits 2 and the second never runs
- `--changed-since` needs the ref in the checkout: `actions/checkout` with `fetch-depth: 0`
- `--auto-approve` on `apply` (and `import --into`) in CI; the prompt needs a TTY
- capture exit codes instead of letting the step fail: `... --detailed-exitcode ... || echo "code=$?" >> "$GITHUB_OUTPUT"`, then `if: steps.plan.outputs.code == '1'` -> `exit 1`. Exit 0 no changes (also when `--changed-since` selected nothing), 1 error (wins over changes; also bad ref, no manifests), 2 changes
- `--ci` is deprecated (exits 1 for changes and errors alike); use `--detailed-exitcode`, and `NO_COLOR=1` for plain output
- pull request workflow: `permissions: pull-requests: write`; skip PRs from forks (`if: github.event.pull_request.head.repo.full_name == github.repository`) since they get no secrets and a read-only token; never suggest `pull_request_target` for this (it runs the fork's manifests with your token); guard the comment with `[ -s plan.md ]` (empty when the plan failed early)
- `paths:` filters must include directories read through `source:` / `patches:` (e.g. `templates/**`), or a template-only change does not trigger the workflow. Without `paths:`, `--changed-since` already makes unrelated runs free. A `paths:`-filtered workflow cannot be a required check
- FileSet commit messages can link the trigger: `env: GH_INFRA_SOURCE_URL: ${{ github.event.head_commit.url }}` on the apply job, used as `<% if .Source.URL %>Source: <% .Source.URL %><% end %>`

## Tokens

- `Repository` / `RepositorySet` manifests need a fine-grained PAT or GitHub App token with `Administration: Read and write` (gh-infra reads Actions settings of every planned repository, which needs Administration, #221), `Contents: Read and write`, plus `Secrets`, `Variables`, `Issues` (labels, milestones), `Pull requests` (FileSet `via: pull_request`) as used. Store it as a secret (e.g. `GH_INFRA_TOKEN`) and pass it as `GH_TOKEN`
- the workflow's `GITHUB_TOKEN` works only for `File` / `FileSet` manifests targeting the same repository (`contents: write`, `pull-requests: write` for `via: pull_request`). It has no `administration` permission, so any `Repository` manifest fails with `Resource not accessible by integration`, even in a self-managed repo. It cannot reach other repositories
- comments and issues created by the workflow itself can use `GH_TOKEN: ${{ github.token }}` on that step
- never use a broad classic PAT

## Layouts

- Self-managed (`.github/infra.yaml` in each repo): [references/self-managed.md](./references/self-managed.md)
- Central (one config repo with `repos/`, `files/`): [references/central.md](./references/central.md)

gh-infra reads only top-level `*.yaml` / `*.yml` in each directory argument.
