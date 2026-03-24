---
title: import
sidebar:
  order: 0
---

Export existing repository settings as YAML. Useful for bootstrapping gh-infra configuration from an existing repository.

```bash
gh infra import <owner/repo>
```

## Examples

```bash
# Import and save to a file
gh infra import babarot/my-project > repos/my-project.yaml

# Import and review
gh infra import babarot/my-project
```

The output is a complete `Repository` YAML manifest reflecting the current state of the repository on GitHub.
