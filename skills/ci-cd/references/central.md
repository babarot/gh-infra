# Central Management

Use this when one repository manages many target repositories.

Typical layout:

```text
github-config/
  repos/
  files/
```

Auto-apply example:

```yaml
on:
  push:
    branches: [main]
    paths:
      - "repos/**"
      - "files/**"
jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: |
          gh infra apply ./repos/ --auto-approve
          gh infra apply ./files/ --auto-approve
        env:
          GITHUB_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
```

Drift detection example:

```yaml
on:
  schedule:
    - cron: "0 9 * * 1"
jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: |
          gh infra plan ./repos/ --ci
          gh infra plan ./files/ --ci
        env:
          GITHUB_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
```

Plan only what a pull request changed, to save API calls with many repositories:

```yaml
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # the ref must be in the checkout
      - run: gh infra plan ./repos/ --changed-since origin/${{ github.base_ref }} --ci
```

- also selects File/FileSet manifests whose local `source:`/`patches:` files changed; no grep over `source:` lines needed
- on push: `gh infra apply ./repos/ --changed-since ${{ github.event.before }} --auto-approve`; a failed run is not retried by the next push, so keep the scheduled full drift check
- `github://` source changes are not detected

Use a fine-grained PAT or GitHub App token. The default `GITHUB_TOKEN` cannot manage other repositories.
