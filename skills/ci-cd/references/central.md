# Central Management

Use this when one repository manages many target repositories.

```text
github-config/
  repos/      Repository / RepositorySet manifests
  files/      FileSet manifests
  templates/  files read through source: / patches:
```

- workflows: [workflows.md](./workflows.md) as-is, with `./repos/ ./files/`
- token: fine-grained PAT or GitHub App token for all target repositories (see SKILL.md "Tokens"); the workflow's `GITHUB_TOKEN` cannot reach other repositories
- `--changed-since` replaces any script that maps changed templates to manifests (e.g. `grep "source: ./<path>"`): it also selects File/FileSet documents whose local `source:` files/directories or `patches:` files changed
- `github://` source changes are not detected by `--changed-since`; drift detection covers them
- many repositories: keep plan and apply on `--changed-since` and leave full plans to the schedule, to stay under the API rate limit
