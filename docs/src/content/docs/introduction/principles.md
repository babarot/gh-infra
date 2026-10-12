---
title: Design Principles
sidebar:
  order: 2
---

gh-infra is built for a workflow where `plan` is the only review: a pull request shows the plan, and merging it runs `apply`. Nobody watches the apply. So what the plan shows has to be what happens, and three rules keep it that way.

## `apply` does what `plan` shows

Every change `apply` makes is in the plan, and `apply` makes no other change.

- What GitHub would reject is caught by `validate` or `plan`, not found at `apply`. For example, `automated_security_fixes: true` needs vulnerability alerts, and `actions.fork_pr_approval` is not supported for a private repository. Some settings GitHub accepts but silently does not save, such as `allow_auto_merge` on a private repository without branch protection; `plan` cannot know that, so `apply` compares what GitHub returns with what it sent and reports the setting as failed.
- When a setting the manifest manages cannot be read (for example, the token lacks the permission), the repository is reported and left out of the plan. An unreadable setting is never compared as if it were absent or at its default, and a file that cannot be read is never planned as missing.
- When the order of changes matters, `apply` keeps it, and the plan lists them in that order. For example, a repository is created before its FileSets write to it, and archiving comes after every other change to the repository.
- A repository with no commits yet has no branch to open a pull request from, so a FileSet with `via: pull_request` pushes its files to it directly. The plan still shows `via pull_request`; the apply result shows `push` ([#262](https://github.com/babarot/gh-infra/issues/262)). See [Git API](../../internals/git-api/).

## What a manifest leaves out is left alone

A field that is not in the manifest is not compared and not sent.

- Omit a field to leave it as it is on GitHub. Write an empty value, such as `topics: []`, to clear it.
- Collections are additive by default: declared entries are created or updated, and others are left alone. For labels, rulesets and branch protection, `reconcile: authoritative` deletes undeclared entries, and only when the collection is in the manifest. Secrets, variables and milestones are always additive. See [Reconcile](../../resources/repository/#reconcile).
- Some settings, like branch protection and rulesets, are replaced as a whole by the GitHub API. `apply` reads the current settings and sends them back with only what the manifest sets changed, including settings gh-infra does not model.

## After `apply`, `plan` shows no changes

One `apply` reaches what the manifest declares, so the next `plan` is empty. When a run cannot apply something, the plan says so instead of leaving it for a surprise later.

- A repository that does not exist is created with its rulesets, secrets, variables, labels and milestones in the same run. Its branch protection, which needs a branch, and with authoritative label reconcile the deletion of GitHub's default labels, are left to the next plan, and the plan notes them. See [Creating Repositories](../../resources/repository/#creating-repositories).
- A FileSet with `via: pull_request` changes nothing until its pull request is merged, so the next plan shows the same file changes until then. This is by design: the pull request is the review.
- gh-infra does not change a repository that is already archived, except to unarchive it. GitHub makes an archived repository read-only; a few of its APIs still accept writes, but gh-infra does not rely on them. Changes to an archived repository without `archived: false` make `plan` report the repository as an error and leave it out, so the other repositories are still planned and applied. FileSets for an archived repository are not caught at plan yet, and fail at `apply`.

## How the rules are checked

Tests run `plan`, `apply` and `plan` again against a fake GitHub that keeps its state the way the GitHub API returns it, including settings gh-infra does not model. For each kind of setting they check that every planned change applies, that only what the plan shows changes, and that the next plan is empty. The fake rejects what GitHub rejects, such as writes to an archived repository, so a change in the wrong order fails the test.
