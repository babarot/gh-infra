# Reconcile And Delivery

## Reconcile

Modes:

- `additive`: create and update only
- `authoritative`: create, update, and delete orphans under the managed directory
- `create_only`: create if missing, never update on apply

Examples:

```yaml
files:
  - path: .github/CODEOWNERS
    content: "* @platform-team"
```

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
    reconcile: authoritative
```

```yaml
files:
  - path: VERSION
    content: "0.1.0"
    reconcile: create_only
```

## Delivery

`push`:

```yaml
spec:
  via: push
  commit_message: "ci: sync managed files"
```

`pull_request`:

```yaml
spec:
  via: pull_request
  branch: gh-infra/sync
  pr_title: "Sync shared files"
```

If the PR branch already exists, gh-infra updates that PR.

## File Modes

```yaml
spec:
  files:
    - path: .githooks/pre-commit
      source: ./templates/pre-commit
      executable: true   # 100755; false = 100644; omitted = keep current mode
```

- plan diffs the mode too: a content-identical file shows `mode 100644 → 100755`
- A commit that changes a mode uses the Git Data API and is NOT signed/Verified; it is rejected on branches requiring signed commits. Other commits stay on `createCommitOnBranch` (Verified)
- `create_only`: the mode is applied only on creation
- Errors: executable file in an empty repository; `executable` on a symlink or submodule path

## Templated Messages

`commit_message`, `pr_title`, `pr_body` support `<% %>` with `.Repo.*` and `.Source.URL` (from env `GH_INFRA_SOURCE_URL`; empty if unset). `.Vars` is not available.

```yaml
spec:
  commit_message: |-
    ci: sync CI workflow

    <% if .Source.URL %>Source: <% .Source.URL %><% end %>
```

- First line = commit headline, rest = body; empty body is dropped. Default `pr_title` uses only the headline.
- Guard `.Source.URL` with `if`, otherwise an unset env var leaves a dangling `Source: ` line.
- Messages render only at apply time; `plan` does not show or validate them, so template errors surface during `apply`.
