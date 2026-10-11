---
title: Markdown output
sidebar:
  order: 5
---

`plan --output markdown` prints the plan as a comment to post on a pull request, so the plan can be reviewed before merging:

```bash
gh infra plan ./repos/ --output markdown > plan.md
```

Like JSON output, stdout has only the comment, and the exit status is the same as with text output. It is built from the same data as [JSON output](../json-output/).

The comment starts with the summary, then has one collapsed block per repository with a table of its changes. Grouped settings are shown one row per setting (`features.wiki`). With `--diff`, each file change gets its own collapsed block with the diff. Warnings and errors come last; if some repositories could not be planned, the comment says the plan is incomplete.

```markdown
### Plan: 1 to create, 2 to update, 0 to destroy

<details><summary><code>babarot/my-cli</code>: 1 to create, 2 to update</summary>

| | Resource | Name | Change | Before | After |
|---|---|---|---|---|---|
| ~ | Repository |  | <code>description</code> | <code>old</code> | <code>new</code> |
| ~ | Repository |  | <code>features.wiki</code> | <code>false</code> | <code>true</code> |
| + | Label | <code>bug</code> |  |  | <code>#d73a4a</code> |

</details>
```

With [`--changed-since`](../../patterns/ci-cd/#only-what-changed), the summary is followed by `Planned N of M resources changed since <ref>.`, and when nothing changed the heading is `### Plan: no manifests changed since <ref>` instead of `### Plan: no changes`, so the comment shows whether anything was planned.

Values are cut at 200 characters, and diffs at 500 lines per file. The comment is not otherwise limited in size, so a very large plan can go over GitHub's limit of 65,536 characters per comment.

## Posting it from GitHub Actions

The [Plan on pull request](../../patterns/ci-cd/#plan-on-pull-request) workflow posts this comment on every pull request: it plans only what the pull request changed, keeps one comment per pull request with `gh pr comment --edit-last --create-if-none`, and fails the check only when the plan itself fails. The job needs `pull-requests: write`.

With `--detailed-exitcode`, `plan` exits with `2` when there are changes, which fails the step; capture the exit status as that workflow does, so the comment is still posted.
