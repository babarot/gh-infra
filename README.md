# gh-infra

[![Tests](https://github.com/babarot/gh-infra/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)
[![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/babarot/c8e4b1de0846824230d86cb2d86f38cf/raw/gh-infra-coverage.json)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)

Declarative GitHub infrastructure management via YAML. Like Terraform, but for GitHub — no state file required.

```
gh infra plan    # Show what would change
gh infra apply   # Apply the changes
```

## Why

The [Terraform GitHub Provider](https://registry.terraform.io/providers/integrations/github/latest/docs) covers most GitHub-as-Code use cases, but it's overkill for personal or small-team use — provider installation, HCL, state files, and state locking add real overhead before you can change a single setting.

gh-infra takes a different approach:

- **YAML instead of HCL.** Declare what your repos should look like in plain YAML.
- **No state file.** GitHub itself is the source of truth. Every `plan` fetches the live state and diffs directly — there's nothing to store, lock, or lose.
- **`plan` before `apply`.** See exactly what will change before it happens. Most alternatives (Probot Settings, GHaC) apply immediately with no preview.
- **One file, many repos.** A single `RepositorySet` can enforce consistent settings across dozens of repositories. No more clicking through the UI one repo at a time.
- **Just `gh` and a token.** No GitHub App, no server, no extra infrastructure. If you can run `gh`, you can run `gh infra`.

## Install

```bash
# As a gh extension
gh extension install babarot/gh-infra

# Or with Homebrew
brew install babarot/tap/gh-infra

# Or build from source
go install github.com/babarot/gh-infra/cmd/gh-infra@latest
```

## Quick Start

### 1. Import an existing repository

```bash
gh infra import babarot/my-project > repos/my-project.yaml
```

### 2. Edit the YAML to your desired state

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project
  owner: babarot

spec:
  description: "My awesome project"
  visibility: public
  topics:
    - go
    - cli
  features:
    issues: true
    projects: false
    wiki: false
    discussions: true
```

### 3. Plan and apply

```bash
gh infra plan ./repos/
gh infra apply ./repos/
```

## Commands

| Command | Description |
|---------|-------------|
| `plan [path]` | Show diff between YAML and current GitHub state |
| `apply [path]` | Apply changes (with confirmation prompt) |
| `import <owner/repo>` | Export existing repo settings as YAML |
| `validate [path]` | Check YAML syntax and schema |

### Flags

```
Global:
  -V, --verbose             Show gh command execution details (shorthand for --log-level=debug)
      --log-level <level>   Log level: trace, debug, info, warn, error

plan:
  -r, --repo <owner/repo>   Target a specific repository
      --ci                   Exit with code 1 if changes detected

apply:
  -r, --repo <owner/repo>   Target a specific repository
      --auto-approve         Skip confirmation prompt
      --force-secrets        Re-set all secrets (even existing ones)
```

## Documentation

For the full YAML DSL reference, usage patterns, and advanced topics, see the [documentation](./docs/).

## License

MIT
