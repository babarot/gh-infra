# Self-Managed

Use this when each repository owns its own gh-infra manifest.

```text
my-project/
  .github/
    infra.yaml
    workflows/
      infra-plan.yaml
      infra-apply.yaml
```

- workflows: [workflows.md](./workflows.md) with `.github/infra.yaml` instead of `./repos/ ./files/`; `paths: [".github/infra.yaml"]` is a fine trigger filter here (plus any directory the manifest reads through `source:`)
- token: `.github/infra.yaml` is usually a `Repository` manifest, which needs a fine-grained PAT or GitHub App token for this repository with `Administration: Read and write`. The workflow's own `GITHUB_TOKEN` (no Administration) is enough only if the manifest manages nothing but labels, milestones (`issues: write`) and `File` / `FileSet` documents for this repository

Minimal apply-on-merge:

```yaml
on:
  push:
    branches: [main]
    paths: [".github/infra.yaml"]
jobs:
  apply:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: gh infra apply .github/infra.yaml --auto-approve
```
