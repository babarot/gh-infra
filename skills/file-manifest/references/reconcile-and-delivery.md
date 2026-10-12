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

- Default `branch`: `gh-infra/sync-<owner>-<name>`; `<name>` is the repo for `File`, `metadata.name` for `FileSet`, or the sorted repo names joined by `+` for an unnamed `FileSet`
- Apply resets the PR branch to the default branch before committing, so two `pull_request` resources using the same branch on the same repo are rejected by `validate`/`plan`/`apply` (same explicit `branch`, or no `branch` and the same name: two `File`s for one repo, or unnamed `FileSet`s listing the same repos). Fix: distinct `branch`, or distinct `metadata.name` on a `FileSet`
- `push` resources may share a repo freely
- A repo with no commits (including one the same apply creates) has no branch for a PR: `pull_request` is planned and applied as `push` for that repo (plan shows `via push` with the reason; JSON `via: push` and a warning). If the repo had commits at plan but none at apply, apply fails it ("run plan again")

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
- In GitHub Actions, set it on the apply job: `env: GH_INFRA_SOURCE_URL: ${{ github.event.head_commit.url }}` (the commit that triggered the apply; empty on `workflow_dispatch`, which the `if` guard handles).
- Messages are rendered for the commit at apply time and are not shown in `plan`; `plan` and `validate` render them for each target repository to catch template errors before anything is written.
