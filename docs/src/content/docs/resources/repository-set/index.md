---
title: RepositorySet
sidebar:
  label: Overview
  order: 0
---

`RepositorySet` manages **multiple** repositories with shared defaults. Use this when you have many repos with identical settings and want to avoid repeating the same configuration in every file. Each repository in the set inherits from `defaults` and can override any value.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot

defaults:
  spec:
    visibility: public
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true
    branch_protection:
      - pattern: main
        required_reviews: 1

repositories:
  - name: gomi
    spec:
      description: "Trash CLI: a safe alternative to rm"
      topics: [go, cli, trash]

  - name: enhancd
    spec:
      description: "A next-generation cd command with an interactive filter"
      topics: [zsh, shell, cd, fzf]

  - name: oksskolten
    spec:
      description: "The AI-native RSS reader"
      topics: [rss, self-hosted, ai, typescript]
```
:::

## Metadata

```yaml
metadata:
  owner: babarot    # GitHub owner or organization
```

All repositories in the set belong to this owner. Individual repo names are listed in `repositories`.

## Shared Features

All settings available in [Repository](../repository/) — general settings, branch protection, rulesets, secrets, and variables — work identically in `RepositorySet`. See the Repository documentation for field references:

- [General Settings](../repository/general/) — Description, visibility, topics, features, merge strategy
- [Branch Protection](../repository/branch-protection/) — Classic branch protection rules
- [Rulesets](../repository/rulesets/) — Modern rulesets with enforcement modes and bypass actors
- [Secrets & Variables](../repository/secrets-variables/) — GitHub Actions secrets and repository variables
- [Actions](../repository/actions/) — GitHub Actions permissions, workflow defaults, allowed actions, and fork PR approval

## When to Use RepositorySet

### The Problem

Suppose you manage 20 repositories that all share the same merge strategy, branch protection, and feature settings. With individual `Repository` files, you'd repeat those settings in every file:

```
repos/
├── gomi.yaml          # merge_strategy, branch_protection, features...
├── enhancd.yaml       # same merge_strategy, branch_protection, features...
├── oksskolten.yaml    # same merge_strategy, branch_protection, features...
└── ... (17 more files with the same boilerplate)
```

When you need to change a shared setting — say, bump `required_reviews` from 1 to 2 — you have to edit all 20 files. Miss one, and your repos drift out of sync.

### The Solution

`RepositorySet` solves this by extracting shared settings into a `defaults` block. Each repository only declares what's unique to it (description, topics, etc.). A single file replaces 20:

```yaml
defaults:
  spec:
    # Change once, applies to all 20 repos
    branch_protection:
      - pattern: main
        required_reviews: 2

repositories:
  - name: gomi
    spec:
      description: "Trash CLI"
  - name: enhancd
    spec:
      description: "A next-generation cd command"
  # ...
```

### When Not to Use It

`RepositorySet` isn't always the right choice. Use separate `Repository` files when:

- **Each repo has mostly unique settings** — the `defaults` block would be nearly empty, so there's no benefit.
- **You need clean per-repo git blame** — with `RepositorySet`, all repos share one file, so `git blame` shows who changed the file, not which repo was affected.
- **Different teams own different repos** — separate files let each team manage their own config independently.

### Comparison

| | RepositorySet | Separate Repository files |
|---|---|---|
| Shared settings | Write once in `defaults` | Repeated in every file |
| Adding a repo | Add 3-5 lines | Create a new file with full spec |
| Changing a shared setting | Edit one place | Edit every file |
| Per-repo git blame | All changes in one file | Clean, one file per repo |
| Team ownership | Single file, shared ownership | Each team owns their file |
