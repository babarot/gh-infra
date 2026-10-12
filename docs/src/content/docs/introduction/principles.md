---
title: Design Principles
sidebar:
  order: 2
---

gh-infra is built for a workflow where `plan` is the only review: a pull request shows the plan, and merging it runs `apply`. Nobody watches the apply. So what the plan shows has to be what happens, and three rules keep it that way.

## `apply` does what `plan` shows

Every change `apply` makes is in the plan, and `apply` makes no other change.

- What GitHub would reject is caught by `validate` or `plan`, not found at `apply`. For example, `automated_security_fixes: true` needs vulnerability alerts, and `actions.fork_pr_approval` is not supported for a private repository.
- When a setting the manifest manages cannot be read (for example, the token lacks the permission), the repository is reported and left out of the plan. An unreadable setting is never compared as if it were absent or at its default, and a file that cannot be read is never planned as missing.
- When the order of changes matters, `apply` keeps it, and the plan lists them in that order. For example, a repository is created before its FileSets write to it, and archiving comes after every other change to the repository.

## What a manifest leaves out is left alone

A field that is not in the manifest is not compared and not sent.

- Omit a field to leave it as it is on GitHub. Write an empty value, such as `topics: []`, to clear it.
- Collections (labels, rulesets, branch protection, ...) are additive by default: declared entries are created or updated, and others are left alone. `reconcile: authoritative` deletes undeclared entries, and only when the collection is in the manifest. See [Reconcile](../../resources/repository/#reconcile).
- Some settings, like branch protection and rulesets, are replaced as a whole by the GitHub API. `apply` reads the current settings and sends them back with only what the manifest sets changed, including settings gh-infra does not model.

## After `apply`, `plan` shows no changes

One `apply` reaches what the manifest declares, so the next `plan` is empty. When a run cannot apply something, the plan says so instead of leaving it for a surprise later.

- A repository that does not exist is created with its rulesets, secrets, variables, labels and milestones in the same run. Its branch protection, which needs a branch, and with authoritative label reconcile the deletion of GitHub's default labels, are left to the next plan, and the plan notes them. See [Creating Repositories](../../resources/repository/#creating-repositories).
- A repository that is already archived is read-only, so changes to it cannot be applied. `plan` does not catch this yet ([#259](https://github.com/babarot/gh-infra/issues/259)).

## How the rules are checked

Tests run `plan`, `apply` and `plan` again against a fake GitHub that keeps its state the way the GitHub API returns it, including settings gh-infra does not model. For each kind of setting they check that every planned change applies, that only what the plan shows changes, and that the next plan is empty. The fake rejects what GitHub rejects, such as writes to an archived repository, so a change in the wrong order fails the test.
