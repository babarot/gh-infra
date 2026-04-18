---
title: Defaults & Overrides
---

The `defaults` block defines shared settings applied to all repositories in the set. Individual repositories can override any default value.

```yaml
defaults:
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

Strings, booleans, and numbers (including `label_sync`) are simply replaced by the per-repo value.

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
```

### Collections — merged by key

Collections with a natural key field are merged. Entries with the same key are merged or overridden; new entries are appended. Omitting the collection entirely inherits the full default.

| Collection | Key field | Same-key behavior |
|---|---|---|
| `labels` | `name` | Entry replaced |
| `branch_protection` | `pattern` | Fields merged (unspecified fields inherit default) |
| `rulesets` | `name` | Entry replaced |

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
