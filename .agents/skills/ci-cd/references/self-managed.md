# Self-Managed

Use this when each repository owns its own gh-infra manifest.

Typical layout:

```text
my-project/
  .github/
    infra.yaml
    workflows/
      infra.yaml
```

Example workflow:

```yaml
on:
  push:
    branches: [main]
    paths: [".github/infra.yaml"]
jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: gh infra apply .github/infra.yaml --auto-approve
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

This is the simplest pattern because the default workflow token can manage the same repository.
