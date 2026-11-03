---
title: Delivery Method
sidebar:
  order: 5
---

`via` controls how file changes are delivered to the target repository.

## `push` (default)

Commits directly to the default branch. All file changes are bundled into a single atomic commit.

```yaml
spec:
  via: push
  commit_message: "ci: add the shared CI workflow"  # optional, made from the changed files if omitted
```

Use this for low-risk files (LICENSE, CODEOWNERS, security policies) or routine syncs of already-reviewed templates.

## `pull_request`

Creates a branch, commits all files, and opens a pull request for review before merging.

```yaml
spec:
  via: pull_request
  commit_message: "ci: add the shared CI workflow"  # optional, made from the changed files if omitted
  branch: gh-infra/sync-shared                       # optional, see below
  pr_title: "Add the shared CI workflow"             # optional, defaults to the commit headline
  pr_body: |                                         # optional, supports Markdown
    Run the same CI as the other repositories.

    <% range .Files -%>
    - <% .Action %> `<% .Path %>`: <% .Description %>
    <% end -%>
```

Use this when changes need review — for example, CI workflows or Dockerfiles that could break builds. See [Pull request description](#pull-request-description) for what the pull request says.

If a pull request is already open for the branch, `apply` resets the branch and commits again, so the pull request shows the new changes. Its title and body stay as they were: gh-infra sets them only when it opens the pull request.

### Pull request branch

Without `branch`, the branch is `gh-infra/sync-<owner>-<name>`: `<name>` is the repository for `File` and `metadata.name` for `FileSet`. An unnamed `FileSet` uses its sorted repository names joined by `+`, e.g. `gh-infra/sync-babarot-gomi+gh-infra`.

Each apply resets the branch to the default branch before committing, so two `pull_request` resources that would use the same branch on the same repository are rejected by `validate`, `plan`, and `apply`. This happens when both set the same `branch`, or neither sets one and they share a name: two `File` manifests for one repository, or unnamed `FileSet`s that list the same repositories. Set a distinct `branch`, or a distinct `metadata.name` on a `FileSet`.

## Related fields

| Field | Used by | Default | Description |
|---|---|---|---|
| `commit_message` | both | made from the changed files | Commit message for the sync commit |
| `branch` | `pull_request` | `gh-infra/sync-<owner>-<name>` | Branch name for the pull request (see [Pull request branch](#pull-request-branch)) |
| `pr_title` | `pull_request` | headline of `commit_message` | Pull request title |
| `pr_body` | `pull_request` | a line per changed file entry | Pull request body (supports Markdown) |
| `files[].description` | both | *(none)* | What the file is for, in `.Files` and the default pull request body (see [Pull request description](#pull-request-description)) |

## Pull request description

The pull request is read by people who look only at the target repository. Write it from that repository's point of view, saying what is added and why, rather than where the files come from. Give each file entry a `description` of what the file does for that repository:

```yaml
spec:
  files:
    - path: .regomark/policy
      source: ./templates/.regomark/policy/
      reconcile: authoritative
      description: Policies that check docs/adr against the ADR rules.

    - path: .github/workflows/regomark.yaml
      source: ./templates/.github/workflows/regomark.yaml
      description: Runs regomark check on every pull request.

  via: pull_request
  pr_title: Check docs/adr against the ADR rules
```

Without `pr_body`, the body has a line for each file entry the commit changes, in the order of `files`:

```markdown
- Add `.regomark/policy/` (11 files): Policies that check docs/adr against the ADR rules.
- Add `.github/workflows/regomark.yaml`: Runs regomark check on every pull request.
```

An entry is `Add` when all its changed files are created, `Remove` when all are deleted (by `reconcile: authoritative`), and `Update` otherwise. An entry with a directory source is one line with the number of its changed files. Entries with nothing to change are left out.

Without `commit_message`, the commit headline, and so the default pull request title, names the change: `Add .tagpr` for a single file, `Add 12 files` when every file has the same action, and `Update 12 files` otherwise. A count says little, so set `pr_title` for pull requests.

To write the body yourself, use `.Files` in `pr_body` (see [Templated messages](#templated-messages)). A `description` accepts the same `<% %>` expressions as the messages, except `.Files`. In a `FileSet`, an override can change the description for one repository, for example to explain why a file of its own is replaced (see [Per-repo Overrides](../../fileset/overrides/)).

The title and body are set when gh-infra opens the pull request. While it stays open, later applies update its branch but not its title or body, so a file that changes after the pull request was opened is not listed in its body.

## File modes

Set `executable` on a file entry to control its executable bit, for example for git hooks and scripts:

```yaml
spec:
  files:
    - path: .githooks/pre-commit
      source: ./templates/pre-commit
      executable: true
```

| Value | Result |
|---|---|
| `true` | The file is committed with mode `100755` |
| `false` | The file is committed with mode `100644`, clearing an existing executable bit |
| *(omitted)* | An existing file keeps its current mode; a new file is created as `100644` |

`plan` compares the mode as well as the content, so adding `executable: true` to a file whose content is already in sync shows a mode change (`mode 100644 → 100755`). With `reconcile: create_only`, the mode is applied only when the file is created.

:::caution
GitHub's `createCommitOnBranch` mutation cannot change file modes. When a commit has to change a mode, gh-infra creates it with the Git Data API instead, and that commit is not signed (not Verified). `plan` notes this for each affected repository, and pushes to branches that require signed commits are rejected. Commits that do not change a mode still use `createCommitOnBranch` and stay Verified. See [GraphQL Commit API vs Contents API](/internals/git-api/).
:::

Only regular files can be marked: a symlink or submodule at the path is reported as an error. Executable files cannot be created in an empty repository, because the Contents API used there cannot set a mode; create the first commit without them.

## Templated messages

`commit_message`, `pr_title`, and `pr_body` accept `<% %>` expressions, rendered per target repository at apply time. `plan` and `validate` also render them, so a template error is reported before anything is written:

| Variable | Value |
|---|---|
| `<% .Repo.Name %>`, `<% .Repo.Owner %>`, `<% .Repo.FullName %>` | Target repository, as in [Templating](/resources/file/templating/) |
| `<% .Source.URL %>` | Value of the `GH_INFRA_SOURCE_URL` environment variable, or empty if unset |
| `<% .Files %>` | The file entries the commit changes in the target repository, in the order of `files` (see below) |

Each element of `.Files` is a file entry with changes:

| Field | Value |
|---|---|
| `.Path` | The entry's `path`; the directory for a directory source |
| `.Paths` | The changed files of the entry |
| `.Count` | The number of changed files |
| `.Type` | `create`, `update` or `delete`, or `mixed` when the files differ |
| `.Action` | `Add`, `Update` or `Remove`; `Update` for `mixed` |
| `.Description` | The entry's `description`, rendered, without trailing newlines |

```yaml
spec:
  pr_body: |
    Add the rules for writing ADRs and a CI check that keeps docs/adr in line with them.

    <% range .Files -%>
    - <% .Action %> `<% .Path %>`<% if gt .Count 1 %> (<% .Count %> files)<% end %>: <% .Description %>
    <% end -%>
```

`-%>` drops the newline after the action, so the list has no blank lines between its items. `.Vars` is not available in messages. A multi-line `commit_message` is split Git-style: the first line becomes the commit headline and the rest becomes the body. When `pr_title` is omitted, only the headline is used as the title.

Link each sync commit back to the change that triggered it by setting `GH_INFRA_SOURCE_URL` in CI, for example to `${{ github.event.head_commit.url }}` in a workflow that applies on push (see [Apply on merge](../../../patterns/ci-cd/#apply-on-merge)). `plan` and `validate` render the messages and descriptions with an empty URL to check them. `plan` renders them against the files it plans to change; `validate`, which plans nothing, against every file entry as if each were added. Wrap the line in `if` so nothing is emitted when the variable is unset:

```yaml
spec:
  commit_message: |-
    ci: sync CI workflow for <% .Repo.Name %>

    <% if .Source.URL %>Source: <% .Source.URL %><% end %>
```

## Empty repositories

For repositories with no commits yet, gh-infra falls back to the Contents API. Each file becomes a separate commit, named `Add <path>` without `commit_message`, or `<headline>: <path>` with it. With `via: pull_request` there is no branch to open a pull request from, so the files are pushed to the default branch; `plan` shows `via push` for that repository, including one the same `apply` creates. See [GraphQL Commit API vs Contents API](/internals/git-api/) for details.
