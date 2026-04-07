---
name: ci-cd
description: >
  CI/CD integration patterns for gh-infra: auto-apply on merge, scheduled drift
  detection, self-managed vs central-management layouts, and authentication
  setup for GitHub Actions workflows.
---

# CI/CD Integration

Use this skill when the user wants GitHub Actions workflows around gh-infra.

## Choose A Pattern

- Self-managed: each repository owns its own `.github/infra.yaml`
- Central management: one config repo manages many target repos

Read:

- Self-managed workflow: [references/self-managed.md](./references/self-managed.md)
- Central management workflow: [references/central.md](./references/central.md)

## Common Building Blocks

- `gh infra apply ... --auto-approve` on merge to `main`
- `gh infra plan ... --ci` on a schedule for drift detection
- use `GITHUB_TOKEN` only for self-managed single-repo workflows
- use a fine-grained PAT or GitHub App token for cross-repo central management

## Authentication

### Self-managed

Use the default workflow token:

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Central management

The default workflow token cannot manage other repositories. Use:

- a fine-grained PAT with the required per-repo permissions
- or a GitHub App installation token

Pass it as:

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
```

Never use a broad classic PAT.

## Important Constraints

- gh-infra reads only top-level `*.yaml` / `*.yml` in the target directory
- if manifests are split across `repos/` and `files/`, run once per directory
- `--auto-approve` is required in CI
- `--ci` makes `plan` exit 1 when drift exists

## Typical Flows

- PR review flow: run `plan` on pull requests touching manifests
- Auto-apply flow: run `apply --auto-approve` on merge
- Drift detection flow: run `plan --ci` on a schedule

Use both auto-apply and drift detection unless the user explicitly wants review-only behavior.
