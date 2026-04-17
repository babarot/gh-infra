# Commands

## Global Flags

- `-V`, `--verbose`: shorthand for debug logging
- `--log-level <trace|debug|info|warn|error>`: explicit log level

## import

```bash
gh infra import <owner/repo> [owner/repo ...]
gh infra import <owner/repo> --into=<path>
```

Use plain `import` to bootstrap manifests.

Use `--into` to pull live GitHub state back into existing manifests and local file sources. This is not the normal day-to-day workflow; use it when local YAML drifted behind GitHub and you want to reconcile local artifacts.

## validate

```bash
gh infra validate [path...]
```

- Checks YAML syntax and schema only
- Does not contact GitHub
- Exits nonzero on validation failure
- Accepts multiple paths: `gh infra validate ./repos/ ./files/`

## plan

```bash
gh infra plan [path...]
```

Flags:

- `-r`, `--repo <owner/repo>`: limit to one repository
- `--ci`: exit 1 if changes are detected
- `--fail-on-unknown`: fail on non-gh-infra YAML kinds instead of skipping

## apply

```bash
gh infra apply [path...]
```

Flags:

- `-r`, `--repo <owner/repo>`: limit to one repository
- `--auto-approve`: skip confirmation prompt
- `--force-secrets`: re-send all declared secrets
- `--fail-on-unknown`: fail on non-gh-infra YAML kinds instead of skipping

Interactive diff viewer:

- `d`: open diff viewer from confirm prompt
- `j` / `k` or arrows: move selection
- `Tab`: toggle apply/skip for selected file
- `d` / `u`: scroll diff
- `q` / `Esc`: return to confirmation

Apply/skip choices are runtime-only. They do not rewrite the manifest.
