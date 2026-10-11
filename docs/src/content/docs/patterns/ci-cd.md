---
title: CI/CD Integration
sidebar:
  order: 2
---

gh-infra is designed to run in CI. The two most common patterns are **auto-apply on merge** and **scheduled drift detection**. Both work with either [central](../central/) or [self-managed](../self-managed/) setups.

## Auto-Apply on Merge

Automatically apply infrastructure changes when YAML files are merged to `main`. This ensures the live GitHub state always matches the declared state.

```yaml
# .github/workflows/infra-apply.yaml
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
      - run: gh infra apply ./repos/ --auto-approve
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The `--auto-approve` flag skips the interactive confirmation prompt, which is required in non-interactive environments like CI.

## Authentication

The token you use depends on which pattern you follow.

### Self-Managed (single repo)

The default `GITHUB_TOKEN` provided by GitHub Actions is sufficient. It has permissions to modify the repository where the workflow runs — no extra setup needed.

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Central Management (multiple repos)

The default `GITHUB_TOKEN` **cannot access other repositories**. You need a token with cross-repo permissions:

- **Fine-grained personal access token** — Select the target repositories and grant `Administration: Read and write`, `Contents: Read and write`, and any other permissions you need (e.g., `Secrets: Read and write` for managing secrets).
- **GitHub App installation token** — Recommended for organizations. Create a GitHub App with the required permissions, install it on the target repos, and generate an installation token in your workflow.

Store the token as a repository secret (e.g., `GH_INFRA_TOKEN`) and pass it to the workflow:

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
```

:::caution
Never use a classic personal access token with broad scopes. Use a fine-grained token or GitHub App with the minimum permissions required.
:::

## Drift Detection

Run `plan` on a schedule to detect when the live GitHub state has drifted from your YAML — for example, when someone changes a setting through the GitHub UI.

```yaml
# .github/workflows/infra-drift.yaml
on:
  schedule:
    - cron: "0 9 * * 1"   # Every Monday at 9am UTC
jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh infra plan ./repos/ --detailed-exitcode
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

With `--detailed-exitcode`, `plan` exits with `2` when changes are detected and `1` when the plan itself fails (for example, a fetch hits the rate limit), so a workflow can tell drift from a broken run. Either fails the step, which you can wire up to notifications (Slack, email, etc.) or use as a required status check. To react to drift only, check the code:

```yaml
      - id: plan
        run: gh infra plan ./repos/ --detailed-exitcode || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: steps.plan.outputs.code == '1'
        run: exit 1   # the plan failed
      - if: steps.plan.outputs.code == '2'
        run: echo "drift detected"
```

## Planning Only What Changed

Planning every manifest on every pull request costs API calls for every repository. With `--changed-since <ref>`, `plan` and `apply` handle only:

- the manifest files changed since `<ref>` (all documents in them), and
- the `File` and `FileSet` documents that read a changed local file: a `source:` file, a file added to, changed in or removed from a `source:` directory, or a `patches:` file

"Changed" means changed between the merge base of `<ref>` and the working tree, committed or not, plus untracked files. When nothing changed, the command says so and makes no API calls; with `-o json` the document's `changed_since.selected` is `0`, and with `-o markdown` the comment reads `Plan: no manifests changed since <ref>`.

```yaml
# .github/workflows/infra-plan.yaml
on:
  pull_request:
jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # --changed-since needs the history of the ref
      - run: gh infra plan ./repos/ --changed-since origin/${{ github.base_ref }} --detailed-exitcode
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

On merge, apply what the push changed:

```yaml
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: gh infra apply ./repos/ --changed-since ${{ github.event.before }} --auto-approve
```

Things to know:

- The ref must be in the checkout. With the default `fetch-depth: 1` it is not, and the command fails.
- If an `apply --changed-since` run fails, the next push does not include its changes, so they are not retried. Keep a scheduled full `plan --detailed-exitcode` (see [Drift Detection](#drift-detection)) or a periodic full `apply`.
- Changes to a `github://` source cannot be seen in git; documents using one are handled only when their manifest or local files change.
- Validation still covers all manifests, so a conflict with an unchanged manifest (for example two FileSets opening a pull request from the same branch) is still reported.
- Every path argument must be inside the git repository. Requires git 2.30 or later.

## Combining Both

For a complete setup, use both workflows together:

- **Auto-apply** keeps the live state in sync after intentional changes
- **Drift detection** catches unintentional changes made outside of gh-infra

This gives you confidence that the YAML files are the true source of truth — not just a snapshot that may have drifted.
