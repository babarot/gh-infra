---
name: ci-cd
description: >
  CI/CD integration patterns for gh-infra: auto-apply on merge, scheduled drift
  detection, and authentication setup for GitHub Actions workflows.
---

# CI/CD Integration

gh-infra is designed to run in CI. The two main patterns are **auto-apply on merge** and **scheduled drift detection**.

## Auto-Apply on Merge

Automatically apply infrastructure changes when YAML files are merged to `main`:

```yaml
# .github/workflows/infra-apply.yaml
name: Apply Infrastructure
on:
  push:
    branches: [main]
    paths:
      - "repos/**"
      - "files/**"

jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          gh infra apply ./repos/ --auto-approve
          gh infra apply ./files/ --auto-approve
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The `--auto-approve` flag skips the interactive confirmation prompt — required in non-interactive environments like CI.

**Important:** gh-infra only reads YAML files directly under the specified directory (not subdirectories). If your manifests are split across directories (e.g., `repos/` for Repository resources and `files/` for File resources), run `apply` on each directory. Adjust the `paths` filter and `apply` commands to match your directory structure.

## Drift Detection

Run `plan` on a schedule to detect when GitHub state has drifted from YAML — for example, when someone changes a setting through the GitHub UI:

```yaml
# .github/workflows/infra-drift.yaml
name: Drift Detection
on:
  schedule:
    - cron: "0 9 * * 1"   # Every Monday at 9am UTC

jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          gh infra plan ./repos/ --ci
          gh infra plan ./files/ --ci
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The `--ci` flag makes `plan` exit with code 1 if any changes are detected. Wire this up to notifications (Slack, email, etc.) or use as a required status check.

## Combining Both

Use both workflows together for full coverage:

- **Auto-apply** keeps live state in sync after intentional changes
- **Drift detection** catches unintentional changes made outside of gh-infra

## Authentication

### Self-Managed (single repo)

The default `GITHUB_TOKEN` provided by GitHub Actions is sufficient — no extra setup:

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Central Management (multiple repos)

The default `GITHUB_TOKEN` **cannot access other repositories**. Use one of:

**Fine-grained personal access token:**

- Select the target repositories
- Grant permissions: `Administration: Read and write`, `Contents: Read and write`, and others as needed (e.g., `Secrets: Read and write`)

**GitHub App installation token (recommended for organizations):**

- Create a GitHub App with the required permissions
- Install on the target repos
- Generate an installation token in the workflow

Store as a repository secret and pass to the workflow:

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
```

Never use a classic personal access token with broad scopes. Use fine-grained tokens or GitHub Apps with minimum required permissions.

## PR-based Review Flow

For teams that want to review infrastructure changes before applying:

```yaml
# .github/workflows/infra-plan.yaml
name: Plan Infrastructure
on:
  pull_request:
    paths:
      - "repos/**"
      - "files/**"

jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh infra plan ./repos/
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

This runs `plan` on PRs that modify manifests, giving reviewers visibility into what would change before merging triggers `apply`.
