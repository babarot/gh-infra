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

If a PR is already open for the branch, apply resets the branch and commits again; the PR's title and body are NOT updated (they are set only when gh-infra opens the PR).

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

## PR Description

Write the PR from the target repo's point of view (what is added and why), not "synced from X". Put the why on each file entry as `description`:

```yaml
spec:
  files:
    - path: .regomark/policy
      source: ./templates/.regomark/policy/
      description: Policies that check docs/adr against the ADR rules.
  via: pull_request
  pr_title: Check docs/adr against the ADR rules   # set it: the default title is only a count
```

- Default `pr_body`: one line per file entry with changes, in `files` order: ``- Add `.regomark/policy/` (11 files): <description>``. A directory source is one line. Unchanged entries are left out. No gh-infra or FileSet name appears.
- Action per entry: `Add` (all created), `Remove` (all deleted by `authoritative`), `Update` otherwise
- Default commit headline (and so default `pr_title`): `Add .tagpr` for one file, `Add 12 files` when all have the same action, `Update 12 files` otherwise. Empty-repo fallback (one commit per file): `Add <path>` per commit; with `commit_message`, `<headline>: <path>`
- `description` accepts `<% %>` with `.Repo.*` and `.Source.URL` (no `.Vars`, no `.Files`); trailing newlines are trimmed
- Per-repo description: an override with only `path` and `description` (see fileset reference)

## Templated Messages

`commit_message`, `pr_title`, `pr_body` support `<% %>` with `.Repo.*`, `.Source.URL` (from env `GH_INFRA_SOURCE_URL`; empty if unset) and `.Files`. `.Vars` is not available.

`.Files`: the file entries this commit changes in the target repo, in `files` order. Fields: `.Path` (entry path; the directory for a directory source), `.Paths`, `.Count`, `.Type` (`create`/`update`/`delete`/`mixed`), `.Action` (`Add`/`Update`/`Remove`), `.Description`.

```yaml
spec:
  pr_body: |
    Add the rules for writing ADRs and a CI check for them.

    <% range .Files -%>
    - <% .Action %> `<% .Path %>`<% if gt .Count 1 %> (<% .Count %> files)<% end %>: <% .Description %>
    <% end -%>
```

- Use `-%>` on `range`/`end`, or the list gets blank lines between items

```yaml
spec:
  commit_message: |-
    ci: sync CI workflow

    <% if .Source.URL %>Source: <% .Source.URL %><% end %>
```

- First line = commit headline, rest = body; empty body is dropped. Default `pr_title` uses only the headline.
- Guard `.Source.URL` with `if`, otherwise an unset env var leaves a dangling `Source: ` line.
- In GitHub Actions, set it on the apply job: `env: GH_INFRA_SOURCE_URL: ${{ github.event.head_commit.url }}` (the commit that triggered the apply; empty on `workflow_dispatch`, which the `if` guard handles).
- Messages are rendered for the commit at apply time and are not shown in `plan`; `plan` and `validate` render them (and descriptions) for each target repository to catch template errors before anything is written. `plan` uses the files it plans to change; `validate` uses every file entry as if added. So `index .Files 1` can pass `validate` and fail `plan` when only one entry changes.
