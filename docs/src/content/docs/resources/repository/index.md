---
title: Repository
sidebar:
  label: Overview
  order: 0
---

`Repository` manages a **single** GitHub repository — its description, visibility, topics, labels, features, merge strategy, branch protection rules, rulesets, secrets, variables, and Actions settings.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project
  owner: babarot

reconcile:
  rulesets: authoritative
  labels: authoritative

spec:
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public
  topics: [go, cli]

  labels:
    - name: kind/bug
      color: d73a4a
      description: "A bug; unintended behavior"
    - name: kind/feature
      color: "425df5"
      description: "A feature request"

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
    sha_pinning_required: true
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
| `labels` | Repository issue/PR labels — see [Labels](./labels/) |
| `label_sync` | Deprecated label sync mode: use `reconcile.labels` instead — see [Labels](./labels/#reconcile-mode) |
| `features` | Toggle issues, projects, wiki, discussions, pull requests — see [General Settings](./general/) |
| `merge_strategy` | Merge commit, squash, rebase options — see [General Settings](./general/) |
| `branch_protection` | Classic branch protection rules — see [Branch Protection](./branch-protection/) |
| `rulesets` | Modern rulesets with enforcement modes and bypass actors — see [Rulesets](./rulesets/) |
| `secrets` | GitHub Actions secrets (via `${ENV_*}` references) — see [Secrets & Variables](./secrets-variables/) |
| `variables` | Repository variables — see [Secrets & Variables](./secrets-variables/) |
| `actions` | GitHub Actions permissions, SHA pinning, workflow defaults, and fork PR policy — see [Actions](./actions/) |

All fields are optional — declare only what you want to manage. Fields not present in the YAML are left unchanged on GitHub.
The one exception is `spec.actions.enabled`: when managing any other Actions setting, GitHub requires `enabled` to be sent too. See [Actions](./actions/).

## Reconcile

By default, collection fields are managed additively: gh-infra creates or updates entries declared in YAML and leaves undeclared GitHub resources untouched.

Use top-level `reconcile` to make selected collections match YAML exactly:

```yaml
reconcile:
  rulesets: authoritative
  labels: additive
  branch_protection: additive

spec:
  rulesets:
    - name: protect-main
      target: branch
      rules:
        non_fast_forward: true
```

| Field | Modes | Description |
|---|---|---|
| `reconcile.labels` | `additive`, `authoritative` | Controls how `spec.labels` is compared with existing GitHub labels |
| `reconcile.rulesets` | `additive`, `authoritative` | Controls how `spec.rulesets` is compared with existing GitHub rulesets |
| `reconcile.branch_protection` | `additive`, `authoritative` | Controls how `spec.branch_protection` is compared with existing classic branch protection rules |

`additive` is the default. `authoritative` deletes remote entries that are not declared in YAML, but only when the corresponding `spec` collection is present. If `spec.rulesets`, `spec.branch_protection`, or `spec.labels` is omitted, that collection is unmanaged even when `reconcile` has a policy for it.

To delete all labels, rulesets, or branch protection rules, use `authoritative` with an empty list:

```yaml
reconcile:
  rulesets: authoritative
spec:
  rulesets: []
```

`labels: null`, `labels:`, `rulesets: null`, `rulesets:`, `branch_protection: null`, and `branch_protection:` are invalid. Use an empty list when you mean an empty managed collection.

## When to Use

Use `Repository` when you want to manage one repo's settings in a dedicated YAML file. This is the simplest resource kind and the starting point for most users.

It works best when:

- **Each repo has its own distinct settings** — different branch protection rules, different topics, different merge strategies.
- **You want per-repo change tracking** — each file maps to one repo, so `git blame` tells you exactly who changed what and when.
- **You're managing a small number of repos** — for 1–5 repos, separate files are easy to maintain.

If you find yourself copying the same settings across many files, consider [RepositorySet](../repository-set/) instead.

## Creating Repositories

If a YAML manifest references a repository that doesn't exist, `plan` shows it as a new resource, with what the same `apply` adds to it:

```
$ gh infra plan ./repos/

  # babarot/new-project will be created
  + babarot/new-project
      + repository      babarot/new-project
      + ruleset "protect-main"
          + name        protect-main
      + TOKEN           (new)
      + labels
          + bug         #ff0000
      a label named like one of GitHub's default labels is updated instead of created
      branch protection for main is applied by a later plan, once the branch exists
```

`apply` creates it with `gh repo create`, applying the settings, features, merge strategy, topics, security and Actions settings, and then the rulesets, secrets, variables, labels and milestones the plan shows, in the same run. `archived: true` is applied last.

A new repository is empty, so these are planned against nothing, with two exceptions:

- **Default labels**: GitHub creates default labels in a new repository (`bug`, `documentation`, and others; an organization can change them). A label with the name of one of them is updated instead of created, so what the manifest leaves out of it, such as its description, stays as GitHub made it. With `reconcile.labels: authoritative`, the default labels the manifest does not declare are deleted by the next `plan` and `apply`.
- **Branch protection**: a new repository has no branch to protect. The next `plan` after the branch exists shows the protection, and `apply` adds it. Rulesets do not need a branch and are applied at once.

The `-o json` and `-o markdown` plans list these notes as warnings for the repository. FileSets for the new repository are applied after it is created. The repository has no commits then, so a FileSet with `via: pull_request` is planned and applied as a push for it (see [Empty repositories](../file/delivery/#empty-repositories)).

## Deleting Repositories

gh-infra does **not** support repository deletion. Without a state file, there is no way to distinguish "removed from YAML" from "never managed" — and repository deletion is irreversible.

To delete a repository, use `gh repo delete` directly.
