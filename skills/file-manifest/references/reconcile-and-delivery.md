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
