# gh-infra

[![Tests](https://github.com/babarot/gh-infra/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)
[![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/babarot/c8e4b1de0846824230d86cb2d86f38cf/raw/gh-infra-coverage.json)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)

Declarative GitHub infrastructure management via YAML. Like Terraform, but for GitHub — no state file required.

<a href="https://babarot.github.io/gh-infra/">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="./docs/public/demo-light.gif" />
    <img src="./docs/public/demo.gif" alt="gh-infra demo showing plan and apply workflow" />
  </picture>
</a>

```
gh infra plan    # Show what would change
gh infra apply   # Apply the changes
```
📖 **[babarot.github.io/gh-infra](https://babarot.github.io/gh-infra/introduction/getting-started/)** — Full YAML reference, usage patterns, and guides.

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
gh extension install babarot/gh-infra
```

### Agent Skills (optional)

Install gh-infra skills for your coding agent (Claude Code, Codex, Cursor, etc.):

```bash
npx skills add babarot/gh-infra
```

This gives your agent knowledge of gh-infra commands, YAML schema, and CI/CD patterns so it can write and manage manifests for you.

## Quick Start

### 1. Import an existing repository

```bash
gh infra import babarot/my-project > my-project.yaml
```

### 2. Edit the YAML to your desired state

```diff
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
-     projects: false
+     projects: true
      wiki: false
      discussions: true
```

### 3. Plan and apply

```bash
gh infra plan
gh infra apply
```

## Commands

| Command | Description |
|---------|-------------|
| `plan [path]` | Show diff between YAML and current GitHub state |
| `apply [path]` | Apply changes (with confirmation prompt) |
| `import <owner/repo>` | Export existing repo settings as YAML |
| `validate [path]` | Check YAML syntax and schema |

## Path Resolution

`plan`, `apply`, and `validate` accept an optional `[path]` argument:

| Argument | Example | Behavior |
|----------|---------|----------|
| *(none)* or `.` | `gh infra plan` | All `*.yaml` / `*.yml` in the current directory |
| File | `gh infra plan repos/gomi.yaml` | That file only |
| Directory | `gh infra plan repos/` | All `*.yaml` / `*.yml` directly under it (subdirectories are ignored) |

YAML files that are not gh-infra manifests (e.g., GitHub Actions workflows, docker-compose) are silently skipped. Use `--fail-on-unknown` to treat them as errors instead.

## Documentation

For full documentation — YAML reference, usage patterns, and guides — visit **[babarot.github.io/gh-infra](https://babarot.github.io/gh-infra/introduction/getting-started/)**.

## License

MIT
