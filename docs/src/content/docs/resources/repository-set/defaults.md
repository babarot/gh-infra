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

Lists like `topics`, `labels`, and `branch_protection` are **not merged** — the per-repo list replaces the default list entirely.

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

### Maps — merged by key

Maps like `features` and `merge_strategy` are merged. Only specified keys are overridden; unspecified keys retain the default value.

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
