---
title: FileSet
sidebar:
  label: Overview
  order: 0
---

`FileSet` distributes **files** to multiple repositories — CODEOWNERS, LICENSE, CI workflows, security policies, and any other files you want to keep consistent across repos.

:::tip[Example]
```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  name: common-files

spec:
  repositories:
    - babarot/gomi
    - babarot/enhancd
    - name: babarot/gh-infra
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @co-maintainer

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    - path: LICENSE
      source: ./templates/LICENSE

    - path: .github/workflows
      source: ./templates/workflows/

  on_drift: warn                          # warn | overwrite | skip
  strategy: direct                        # direct | pull_request
  commit_message: "ci: sync shared files"
```
:::

## When to Use

### The Problem

Teams often share common files across repositories: a standard CODEOWNERS, a LICENSE, CI workflow templates, or a security policy. Without automation, keeping these in sync means:

- Manually copying files whenever you update a template
- Discovering months later that some repos still have the old version
- No way to audit which repos are out of sync

### The Solution

`FileSet` lets you declare which files should exist in which repos. On `apply`, gh-infra creates a single atomic commit (via the Git Data API) containing all the files — no matter how many there are.

You can also pull files directly from another GitHub repository using `github://` sources, open pull requests instead of committing directly, and handle drift (manual edits) gracefully.

### Key Features

- **Atomic commits** — All files in a single commit, not one commit per file.
- **Multiple source types** — Inline content, local files, local directories, or remote GitHub repositories.
- **Per-repo overrides** — Customize specific files for specific repos while keeping the rest consistent.
- **Drift detection** — Know when someone has manually edited a managed file.
- **PR strategy** — Open a pull request for review instead of committing directly.
