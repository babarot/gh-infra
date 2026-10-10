---
title: Defaults & Overrides
---

The `defaults` block defines shared settings applied to all repositories in the set. Individual repositories can override any default value.

```yaml
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

repositories:
  - name: my-repo
    spec:
      features:
        wiki: true    # overrides the default (false → true)
```

## Merge Behavior

How overrides are applied depends on the field type:

### Scalars — replaced

Strings, booleans, and numbers are simply replaced by the per-repo value.

```yaml
defaults:
  spec:
    visibility: private       # default

repositories:
  - name: public-repo
    spec:
      visibility: public      # overrides → public
  - name: private-repo
    spec:
      description: "Internal"  # visibility stays private (not specified)
```

### Lists — replaced entirely

Lists like `topics` are **not merged** — the per-repo list replaces the default list entirely.

```yaml
defaults:
  spec:
    topics: [go, cli]

repositories:
  - name: web-app
    spec:
      topics: [typescript, react]   # replaces → [typescript, react], NOT [go, cli, typescript, react]
  - name: cli-tool
    spec:
      description: "A CLI tool"     # topics stays [go, cli] (not specified)
  - name: docs
    spec:
      topics: []                    # replaces → no topics
```

### Collections — merged by key

Collections with a natural key field are merged. Entries with the same key are merged or overridden; new entries are appended. Omitting the collection entirely inherits the full default. Writing it as `[]` means the repository has none of it: the defaults are not inherited.

| Collection | Key field | Same-key behavior |
|---|---|---|
| `labels` | `name` | Entry replaced |
| `branch_protection` | `pattern` | Fields merged (unspecified fields inherit default) |
| `rulesets` | `name` | Entry replaced |
| `secrets` | `name` | Entry replaced |
| `variables` | `name` | Entry replaced |

`milestones` is not merged: a non-empty list replaces the defaults' list, and `[]` means none.

```yaml
repositories:
  - name: no-deploy
    spec:
      secrets: []        # none of the default secrets
```

With `reconcile.labels`, `reconcile.rulesets` or `reconcile.branch_protection` set to `authoritative`, `[]` plans the deletion of every label, ruleset or branch protection rule on that repository, as it does in a single `Repository`. `plan` warns when an entry's `[]` drops the defaults this way.

:::caution[Changed in v0.16.0]
Before v0.16.0, an entry's `[]` inherited the defaults. If you wrote `[]` to inherit them, remove the line.
:::

#### Labels

```yaml
defaults:
  spec:
    labels:
      - name: kind/bug
        color: d73a4a
        description: A bug
      - name: kind/feature
        color: "425df5"

repositories:
  - name: my-repo
    spec:
      labels:
        - name: kind/bug
          color: "FF0000"       # overrides default kind/bug
        - name: custom
          color: "00FF00"       # appended
      # result: kind/bug (FF0000) + kind/feature (425df5) + custom (00FF00)
```

#### Branch protection

Same-pattern rules are merged at the field level — only specify the fields you want to override.

```yaml
defaults:
  spec:
    branch_protection:
      - pattern: main
        required_reviews: 1
        dismiss_stale_reviews: true

repositories:
  - name: my-repo
    spec:
      branch_protection:
        - pattern: main
          required_reviews: 2       # overrides → 2; dismiss_stale_reviews stays true
        - pattern: release/*
          required_reviews: 1       # appended
```

#### Rulesets

Same-name rulesets are replaced entirely.

```yaml
defaults:
  spec:
    rulesets:
      - name: protect-main
        target: branch
        enforcement: active

repositories:
  - name: my-repo
    spec:
      rulesets:
        - name: protect-main
          target: branch
          enforcement: evaluate   # replaces the entire default ruleset entry
```

#### Secrets

Same-name entries are replaced; new entries are appended; default entries not referenced are inherited. Variables follow the same rules.

```yaml
defaults:
  spec:
    secrets:
      - name: DEPLOY_TOKEN
        value: "${ENV_DEPLOY_TOKEN}"
      - name: SLACK_WEBHOOK
        value: "${ENV_SLACK_WEBHOOK}"

repositories:
  - name: my-repo
    spec:
      secrets:
        - name: DEPLOY_TOKEN
          value: "${ENV_CUSTOM_TOKEN}"   # overrides default DEPLOY_TOKEN
        - name: EXTRA_TOKEN
          value: "${ENV_EXTRA_TOKEN}"    # appended
      # result: DEPLOY_TOKEN (custom) + SLACK_WEBHOOK (from defaults) + EXTRA_TOKEN (new)
```

#### Variables

```yaml
defaults:
  spec:
    variables:
      - name: APP_ENV
        value: production
      - name: REGION
        value: us-east-1

repositories:
  - name: my-repo
    spec:
      variables:
        - name: REGION
          value: eu-west-1           # overrides default REGION
        - name: EXTRA_VAR
          value: custom-value        # appended
      # result: APP_ENV (from defaults) + REGION (custom) + EXTRA_VAR (new)
```

### Maps — merged by key

Maps like `features`, `merge_strategy`, and `actions` are merged. Only specified keys are overridden; unspecified keys retain the default value.

```yaml
defaults:
  spec:
    features:
      issues: true
      wiki: true
      projects: false

repositories:
  - name: my-repo
    spec:
      features:
        wiki: false          # overrides wiki only
        # issues: true       ← retained from defaults
        # projects: false    ← retained from defaults
```

The `pull_requests` field within `features` is also merged at sub-field level when using the object form:

```yaml
defaults:
  spec:
    features:
      pull_requests:
        creation: collaborators_only

repositories:
  - name: open-repo
    spec:
      features:
        pull_requests:
          creation: all      # overrides creation; PRs stay enabled
```

#### Reconcile

`reconcile` follows the same pattern — each collection key is independently overridable:

```yaml
defaults:
  reconcile:
    labels: authoritative
    rulesets: authoritative
    branch_protection: additive

repositories:
  - name: manual-repo
    reconcile:
      rulesets: additive    # overrides only rulesets; labels and branch_protection stay
```

If a reconcile policy is set but the corresponding collection is omitted after defaults and overrides are merged, that collection is unmanaged for that repository. Use `spec.rulesets: []` or `spec.labels: []` for an explicitly empty managed collection.
