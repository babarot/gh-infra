---
name: repository-manifest
description: >
  Complete YAML schema reference for Repository and RepositorySet resources.
  Use when writing or editing manifests for repo settings, labels, actions,
  branch protection, rulesets, secrets, variables, or repository defaults.
---

# Repository Manifest Reference

Use this skill when editing repository-side manifests. Keep the body small and load references only as needed.

## Core Rules

- All `spec` fields are optional. Omitted fields are left unchanged on GitHub.
- `Repository` manages one repository.
- `RepositorySet` manages many repositories with shared `defaults`.
- For new setups, prefer `rulesets` over classic `branch_protection`.
- Secret values must use `${ENV_*}` indirection, never literal secrets.

## Repository

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: my-org
  name: my-repo
reconcile:           # optional; controls collection reconciliation
  rulesets: additive # additive (default) | authoritative
  labels: additive
spec:
  # declare only managed fields
```

Read these references as needed:

- General settings and lifecycle: [references/general.md](./references/general.md)
- Labels and reconcile mode: [references/labels.md](./references/labels.md)
- Actions settings and validation traps: [references/actions.md](./references/actions.md)
- Rulesets and branch protection: [references/protection.md](./references/protection.md)
- Secrets and variables: [references/secrets-variables.md](./references/secrets-variables.md)
- Security (Dependabot, vulnerability alerts): [references/security.md](./references/security.md)

## RepositorySet

Use `RepositorySet` when many repositories share defaults.

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org

defaults:
  reconcile:
    rulesets: authoritative
    labels: additive
  spec:
    visibility: public
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      allow_rebase_merge: false
      auto_delete_head_branches: true
    rulesets:
      - name: protect-main
        target: branch
        enforcement: active
        conditions:
          ref_name:
            include: ["refs/heads/main"]
        rules:
          pull_request:
            required_approving_review_count: 1
          non_fast_forward: true

repositories:
  - name: repo-a
    spec:
      description: "Service A"
      topics: [go, api]

  - name: repo-b
    spec:
      description: "Service B"
      topics: [python, cli]
      features:
        wiki: true
```

Override behavior matters:

- Scalars are replaced
- Simple lists are replaced entirely
- Keyed collections are merged by key
- Maps are merged by key

This means `topics` replace the default list, while `labels` and `reconcile.labels` are merged by key/collection.

Read [references/repository-set.md](./references/repository-set.md) for the exact merge rules.

## High-Value Gotchas

- `actions.enabled` is required when setting any other `actions.*` field
- `actions.selected_actions` is valid only with `allowed_actions: selected`
- `reconcile.labels: authoritative`, `reconcile.rulesets: authoritative`, and `reconcile.branch_protection: authoritative` delete undeclared remote entries; review `plan` carefully
- `reconcile` without a corresponding `spec` collection is a parse error
- `label_sync: mirror` is deprecated; use `reconcile.labels: authoritative`
- for `gh infra import --into`, use the dedicated `import-into` skill
- Repository deletion is not supported

## Verification

```bash
gh infra validate <path>
gh infra plan <path>
```
