---
title: Repository
sidebar:
  label: Overview
  order: 0
---

`Repository` manages a **single** GitHub repository — its description, visibility, topics, features, merge strategy, branch protection rules, rulesets, secrets, and variables.

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
```
:::

## When to Use

Use `Repository` when you want to manage one repo's settings in a dedicated YAML file. This is the simplest resource kind and the starting point for most users.

It works best when:

- **Each repo has its own distinct settings** — different branch protection rules, different topics, different merge strategies.
- **You want per-repo change tracking** — each file maps to one repo, so `git blame` tells you exactly who changed what and when.
- **You're managing a small number of repos** — for 1–5 repos, separate files are easy to maintain.

If you find yourself copying the same settings across many files, consider [RepositorySet](../../repository-set/) instead.

