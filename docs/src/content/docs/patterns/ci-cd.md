---
title: CI/CD Integration
sidebar:
  order: 2
---

gh-infra is designed to run in CI. The recommended setup is three workflows, which work with either [central](../central/) or [self-managed](../self-managed/) manifests:

| Workflow | Runs on | Does |
|---|---|---|
| [Plan on pull request](#plan-on-pull-request) | `pull_request` | Plans only what the pull request changed and posts the plan as a comment |
| [Apply on merge](#apply-on-merge) | `push` to `main` | Applies only what the push changed |
| [Drift detection](#drift-detection) | `schedule` | Plans everything and opens an issue when GitHub differs from the manifests |

The first two use [`--changed-since`](#only-what-changed) to touch only the repositories a change affects, which keeps API usage low with many repositories. The third plans everything, so it also catches changes made outside gh-infra and anything an earlier apply missed.

All examples assume the manifests are in `./repos/` and `./files/`, and that the token is in a secret named `GH_INFRA_TOKEN` (see [Authentication](#authentication)). Pass every manifest directory to one command (`gh infra plan ./repos/ ./files/`): there is one exit status, and checks that span manifests, such as two FileSets opening a pull request from the same branch, cover all of them.

## Plan on Pull Request

```yaml
# .github/workflows/infra-plan.yaml
on:
  pull_request:
permissions:
  contents: read
  pull-requests: write
jobs:
  plan:
    # PRs from forks get no secrets and a read-only GITHUB_TOKEN; skip them.
    if: github.event.pull_request.head.repo.full_name == github.repository
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # --changed-since needs the history of the base branch
      - run: gh extension install babarot/gh-infra
      - id: plan
        run: |
          gh infra plan ./repos/ ./files/ \
            --changed-since origin/${{ github.base_ref }} \
            --detailed-exitcode -o markdown > plan.md || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: ${{ !cancelled() }}
        env:
          GH_TOKEN: ${{ github.token }}   # comment as github-actions[bot]
        run: |
          [ -s plan.md ] || exit 0   # the plan failed before writing anything
          gh pr comment ${{ github.event.pull_request.number }} \
            --body-file plan.md --edit-last --create-if-none
      - if: steps.plan.outputs.code == '1'
        run: exit 1   # the plan itself failed
```

- The comment is the [Markdown output](../../commands/markdown-output/) of `plan`. `--edit-last --create-if-none` keeps one comment per pull request and updates it on every push. It edits the last comment that github-actions[bot] wrote on the pull request, so another workflow commenting with the same token would be overwritten.
- The check fails only when the plan fails (exit `1`). Changes (exit `2`) are shown in the comment, not as a failed check. See [Exit status](#exit-status).
- When the pull request changes no manifest, the comment says `Plan: no manifests changed since origin/main` and no API calls are made. To skip the workflow for such pull requests, add `paths:` with the manifest directories and every directory the manifests read through `source:` or `patches:` (for example `templates/**`). A workflow filtered by `paths:` cannot be a required check, since it does not run on other pull requests.
- The job does not run for pull requests from forks, so it cannot be a required check for them either. Do not switch to `pull_request_target` to get around this: it would run the fork's manifests with your token.
- `plan` only reads from GitHub, but reading secrets, rulesets and Actions settings still needs the permissions listed in [Authentication](#authentication).

## Apply on Merge

```yaml
# .github/workflows/infra-apply.yaml
on:
  push:
    branches: [main]
  workflow_dispatch:
jobs:
  apply:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
      GH_INFRA_SOURCE_URL: ${{ github.event.head_commit.url }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: gh extension install babarot/gh-infra
      - env:
          BEFORE: ${{ github.event.before }}
        run: |
          # A manual run, or the first push of a branch, has no previous
          # commit to compare with: apply everything.
          if [ -z "$BEFORE" ] || [ "$BEFORE" = "0000000000000000000000000000000000000000" ]; then
            gh infra apply ./repos/ ./files/ --auto-approve
          else
            gh infra apply ./repos/ ./files/ --changed-since "$BEFORE" --auto-approve
          fi
```

- `--auto-approve` skips the confirmation prompt, which cannot be shown in CI.
- `GH_INFRA_SOURCE_URL` is optional: it is the value of `<% .Source.URL %>` in FileSet [commit messages](../../resources/file/delivery/#templated-messages), here the commit that triggered the apply.
- If you add `paths:` to the trigger, include the directories the manifests read through `source:` or `patches:`, not only the manifest directories. Otherwise a change to a template alone does not start the workflow. Without `paths:`, a push that changes no manifest makes no API calls anyway.
- If a run fails, the next push does not include its changes, so they are not retried. [Drift detection](#drift-detection) reports them, and `workflow_dispatch` applies everything.

## Drift Detection

```yaml
# .github/workflows/infra-drift.yaml
on:
  schedule:
    - cron: "0 9 * * 1"   # every Monday at 9am UTC
  workflow_dispatch:
permissions:
  contents: read
  issues: write
jobs:
  drift:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - id: plan
        run: gh infra plan ./repos/ ./files/ --detailed-exitcode -o markdown > plan.md || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: steps.plan.outputs.code == '2'
        env:
          GH_TOKEN: ${{ github.token }}
        run: gh issue create --title "Drift detected by gh-infra" --body-file plan.md
      - if: steps.plan.outputs.code == '1'
        run: exit 1
```

- It plans every manifest, without `--changed-since`, so it finds changes made through the GitHub UI and changes a failed apply left behind.
- Exit `2` (drift) opens an issue with the plan; replace that step with your own notification (Slack, email, and so on). Exit `1` (the plan failed) fails the workflow.

## Importing Changes Made on GitHub

Drift detection reports changes made outside gh-infra. To pull them into the manifests instead, run `import --into` with `--auto-approve` and open a pull request with the result:

```yaml
# .github/workflows/infra-import.yaml
on:
  schedule:
    - cron: "0 9 * * 1"
  workflow_dispatch:
permissions:
  contents: write
  pull-requests: write
jobs:
  import:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: gh infra import --into=./repos/ --auto-approve
      - run: |
          [ -n "$(git status --porcelain)" ] || exit 0   # nothing to import
          branch="gh-infra/import-$(date +%Y%m%d%H%M)"
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git checkout -b "$branch"
          git add -A
          git commit -m "Import GitHub state"
          git push -u origin "$branch"
          gh pr create --head "$branch" --title "Import GitHub state" --body "Changes made on GitHub, pulled into the manifests by gh infra import --into."
```

- `import --into` only edits the checkout. `--auto-approve` writes each file with its [default action](../../commands/import/#without-a-terminal); review the pull request before merging.
- `--into` takes one path; run it once per manifest directory (for example also `--into=./files/`).
- The pull request is opened with `GH_INFRA_TOKEN`, so it runs the [plan workflow](#plan-on-pull-request). One opened with the workflow's `GITHUB_TOKEN` would not trigger other workflows.

## Exit Status

With `--detailed-exitcode`, `plan` exits with:

| Code | Meaning |
|---|---|
| `0` | No changes, or `--changed-since` found nothing that changed |
| `1` | The plan failed: an error, a repository that could not be fetched (for example a rate limit), a bad ref, or paths with no manifests |
| `2` | Changes were found |

An error wins over changes. A step that runs `plan --detailed-exitcode` fails on both `1` and `2`, so capture the code to treat them differently:

```yaml
      - id: plan
        run: gh infra plan ./repos/ ./files/ --detailed-exitcode || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: steps.plan.outputs.code == '1'
        run: exit 1
```

`--ci` is deprecated: it exits with `1` for both changes and errors. It also disabled colors; set `NO_COLOR=1` for that.

## Only What Changed

With `--changed-since <ref>`, `plan` and `apply` handle only:

- the manifest files changed since `<ref>` (all documents in them), and
- the `File` and `FileSet` documents that read a changed local file: a `source:` file, a file added to, changed in or removed from a `source:` directory, or a `patches:` file

"Changed" means changed between the merge base of `<ref>` and the working tree, committed or not, plus untracked files. When nothing changed, the command says so and makes no API calls; with `-o json` the document's `changed_since.selected` is `0`, and with `-o markdown` the comment reads `Plan: no manifests changed since <ref>`.

- The ref must be in the checkout. With the default `fetch-depth: 1` it is not, and the command fails.
- Changes to a `github://` source cannot be seen in git; documents using one are handled only when their manifest or local files change.
- Validation still covers all manifests, so a conflict with an unchanged manifest is still reported.
- Every path argument must be inside the git repository. Requires git 2.30 or later.

## Authentication

The token needs access to every repository the manifests manage, and only the permissions for what they manage: `plan` and `apply` read only the settings a manifest sets. Use one of:

- **Fine-grained personal access token**: select the target repositories and grant the permissions below.
- **GitHub App installation token**: recommended for organizations. Create an app with the same permissions, install it on the target repositories, and generate an installation token in the workflow.

Store it as a secret (for example `GH_INFRA_TOKEN`) and pass it as `GH_TOKEN`.

| Manifest manages | `plan` reads with | `apply` writes with |
|---|---|---|
| description, homepage, visibility, archived, topics, features, merge strategy | Metadata (Contents: read for merge commit messages, auto-merge and pull requests) | Administration: write |
| branch protection, Actions settings, release immutability, vulnerability alerts, automated security fixes | Administration: read | Administration: write |
| rulesets, private vulnerability reporting | Metadata | Administration: write |
| labels, milestones | Issues: read | Issues: write |
| secrets | Secrets: read | Secrets: write |
| variables | Variables: read | Variables: write |
| `File` / `FileSet` | Contents: read | Contents: write (and Pull requests: write for `via: pull_request`) |

If the token cannot read a setting the manifest manages, `plan` fails for that repository and says what it could not read, for example `cannot read secrets of org/repo (the token needs the Secrets permission)`, instead of comparing against nothing. `import` reads everything and leaves out what it cannot read.

The default `GITHUB_TOKEN` of a workflow cannot have the Administration permission and cannot access other repositories. So for the repository running the workflow it can plan and apply labels, milestones (`issues: write`) and `File` / `FileSet` manifests (`contents: write`, plus `pull-requests: write` for `via: pull_request`), but not repository settings. A token without access to a repository at all makes it look like it does not exist, so `plan` shows it as a create.

:::caution
Never use a classic personal access token with broad scopes. Use a fine-grained token or GitHub App with the minimum permissions required.
:::
