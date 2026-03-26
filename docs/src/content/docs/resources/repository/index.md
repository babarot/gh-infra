---
title: Repository
sidebar:
  label: Overview
  order: 0
---

`Repository` manages a **single** GitHub repository — its description, visibility, topics, features, merge strategy, branch protection rules, rulesets, secrets, variables, and Actions settings.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project
  owner: babarot

spec:
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public
  topics: [go, cli]

  features:
    issues: true
    wiki: false

  merge_strategy:
    allow_squash_merge: true
    auto_delete_head_branches: true

  rulesets:
    - name: protect-main
      enforcement: active
      conditions:
        ref_name:
          include: ["refs/heads/main"]
      rules:
        pull_request:
          required_approving_review_count: 1
        non_fast_forward: true

  secrets:
    - name: DEPLOY_TOKEN
      value: "${ENV_DEPLOY_TOKEN}"

  variables:
    - name: APP_ENV
      value: production

  actions:
    enabled: true
    allowed_actions: selected
    workflow_permissions: read
    can_approve_pull_requests: false
    selected_actions:
      github_owned_allowed: true
      patterns_allowed: ["actions/*"]
```
:::

## Metadata

```yaml
metadata:
  owner: babarot        # GitHub owner or organization
  name: my-project      # Repository name
```

The combination of `owner` and `name` identifies the target repository (`babarot/my-project`).

## Spec

| Field | Description |
|---|---|
| `description` | Repository description |
| `homepage` | URL displayed on the repo page |
| `visibility` | `public`, `private`, or `internal` — see [General Settings](./general/) |
| `archived` | Archive (read-only) or unarchive — see [General Settings](./general/#archiving) |
| `topics` | GitHub topics for discoverability |
| `features` | Toggle issues, projects, wiki, discussions — see [General Settings](./general/) |
| `merge_strategy` | Merge commit, squash, rebase options — see [General Settings](./general/) |
| `branch_protection` | Classic branch protection rules — see [Branch Protection](./branch-protection/) |
| `rulesets` | Modern rulesets with enforcement modes and bypass actors — see [Rulesets](./rulesets/) |
| `secrets` | GitHub Actions secrets (via `${ENV_*}` references) — see [Secrets & Variables](./secrets-variables/) |
| `variables` | Repository variables — see [Secrets & Variables](./secrets-variables/) |
| `actions` | GitHub Actions permissions, workflow defaults, and fork PR policy — see [Actions](./actions/) |

All fields are optional — declare only what you want to manage. Fields not present in the YAML are left unchanged on GitHub.

## When to Use

Use `Repository` when you want to manage one repo's settings in a dedicated YAML file. This is the simplest resource kind and the starting point for most users.

It works best when:

- **Each repo has its own distinct settings** — different branch protection rules, different topics, different merge strategies.
- **You want per-repo change tracking** — each file maps to one repo, so `git blame` tells you exactly who changed what and when.
- **You're managing a small number of repos** — for 1–5 repos, separate files are easy to maintain.

If you find yourself copying the same settings across many files, consider [RepositorySet](../repository-set/) instead.

## Creating Repositories

If a YAML manifest references a repository that doesn't exist, `plan` shows it as a new resource:

```
$ gh infra plan ./repos/

Plan: 1 to create, 0 to update, 0 to destroy

  + babarot/new-project (new)
      + repository: babarot/new-project
```

`apply` creates it with `gh repo create`, then applies all settings (features, topics, branch protection, etc.) in a single pass.

## Deleting Repositories

gh-infra does **not** support repository deletion. Without a state file, there is no way to distinguish "removed from YAML" from "never managed" — and repository deletion is irreversible.

To delete a repository, use `gh repo delete` directly.
