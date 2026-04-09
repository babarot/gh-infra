---
title: Import Into Manifests
sidebar:
  order: 5
---

This page documents the internal behavior of `gh infra import --into`.

It focuses on how gh-infra:

- matches manifest resources
- computes file and repository diffs
- decides whether a file is writable, patchable, or skipped
- handles template-backed files safely

## Overview

`import --into` is the reverse direction of `plan` / `apply`.

Instead of taking local manifests as the source of truth and pushing them to GitHub, it:

1. reads local manifests
2. fetches current GitHub state
3. compares local vs remote
4. writes approved changes back into local files and manifests

This is intentionally conservative. The goal is not to reproduce GitHub content at any cost. The goal is to update local source-of-truth files **only when gh-infra can explain and preserve their structure safely**.

## High-Level Flow

At a high level, `import --into` does the following:

1. parse all manifests under the requested path
2. match each `<owner/repo>` target to:
   - `Repository`
   - `RepositorySet`
   - `File`
   - `FileSet`
3. fetch current GitHub state
4. build a combined diff result
5. show a terminal plan
6. optionally open the diff viewer for file changes
7. write the selected changes back to local files

Repository-level changes and file-level changes are planned separately, then combined into one import result.

## File Write-Back Modes

File import planning works with four concrete write-back modes:

- `WriteSource`
  - overwrite the local source file referenced by `source: ./...`
- `WriteInline`
  - update an inline `content: |` block inside the manifest
- `WritePatch`
  - store the drift as a patch under `patches:`
  - available for both source-backed and inline-backed entries
- `WriteSkip`
  - do not write anything

The interactive diff viewer shows these as three user-facing actions:

- `write`
- `patch`
- `skip`

`write` is a user-facing abstraction. Internally it resolves to:

- `WriteSource` for source-backed entries
- `WriteInline` for inline entries

## Default Skip vs Hard Skip

There are two very different reasons a file may appear as skipped.

### Default skip

The file is still importable, but gh-infra chooses `skip` as the safest default for this run.

Typical example:

- `reconcile: create_only`

In this case:

- the file appears in the plan
- the diff viewer can still show it
- the user can press `Tab` to switch to `write` or `patch`

### Hard skip

gh-infra could not construct a safe local write-back result.

In this case:

- the file appears only in the terminal plan
- it does not appear in the diff viewer
- it cannot be toggled

Typical examples:

- `github://` source
- directory-expanded patch target that cannot be reconstructed
- template-backed file whose remote content cannot be safely mapped back to the original template

## Template-Backed File Import

Template-backed files are the most subtle part of `import --into`.

### The problem

A local file may contain placeholders such as:

- `<% .Repo.Name %>`
- `<% .Repo.Owner %>`
- `<% .Repo.FullName %>`

but GitHub only stores the rendered result, for example:

- `my-project`
- `babarot`
- `babarot/my-project`

So import cannot simply copy the remote file back into the local template source. Doing that would destroy the placeholders.

### Current strategy

gh-infra currently uses a trace-aware render and reverse-mapping flow for simple `.Repo.*` placeholders.

It works like this:

1. render the local template for the target repository
2. keep trace information about which rendered segments came from:
   - literals
   - placeholders
3. compare the rendered local content with the remote GitHub content
4. reconstruct updated template source by:
   - preserving the original placeholder source
   - allowing safe literal drift around those placeholders

This is why files like `go.mod` can be imported safely:

- `module github.com/<% .Repo.FullName %>` stays templated
- nearby literal lines such as `go 1.26.1` can still be updated from GitHub

### What is considered safe

The current implementation is intentionally conservative.

It can usually import cases where:

- the placeholder-backed line still exists in recognizable form
- the placeholder value itself is still present in the remote content
- literal drift happens around the placeholder or in nearby non-template lines

Examples:

- `module github.com/<% .Repo.FullName %>`
- `/<% .Repo.Name %>`

### What is considered unsafe

gh-infra hard-skips template-backed files when the reverse mapping would be ambiguous or structurally risky.

Examples:

- the remote file removed the placeholder-backed line entirely
- the remote file rewrote the line into a different shape that no longer clearly corresponds to the template
- the template contains unsupported control flow such as:
  - `<% if %>`
  - `<% range %>`
  - `<% with %>`
- the remote change would require inventing a new placeholder or guessing how to rewrite the template

This is why a heavily diverged `Makefile` may still hard-skip even when a simpler `go.mod` no longer does.

## Why Some Template Files Import and Others Hard-Skip

The distinction is not “uses template syntax” vs “does not use template syntax”.

The real distinction is:

- **can gh-infra reconstruct template source safely?**

So:

- a template-backed file may still be importable
- another template-backed file may hard-skip

For example:

- a `go.mod` file often keeps its placeholder-backed line intact, so gh-infra can preserve the placeholder and update literal lines
- a `Makefile` may have changed structure so much on GitHub that the placeholder-backed lines are no longer recoverable as template source

## Why `.Vars.*` Is More Restrictive

Simple `.Repo.*` placeholders are derived from repository identity, so they are stable and deterministic.

`.Vars.*` is different because its values come from manifest-managed variables.

At the moment, gh-infra does **not** treat changed `.Vars.*` values as safely reversible during import. If the remote file implies a changed `.Vars.*` value, import prefers to hard-skip rather than guess how the manifest variable should be rewritten.

This is an intentional safety choice.

## Shared Sources and Patches

When a local source file is shared across multiple repositories, `import --into` prefers `patch` by default.

Why:

- writing the shared source directly would affect other repositories
- patching captures drift for just the current repository

So the planner distinguishes:

- a file that is directly writable
- a file that should default to `patch`
- a file that cannot be safely written at all

## Design Principle

The main design principle of `import --into` is:

> Preserve manifest intent, not just remote bytes.

That means:

- placeholders should stay placeholders
- shared sources should not be rewritten accidentally
- deprecated field names should not be normalized unnecessarily
- unrelated YAML formatting should not be churned

If gh-infra cannot preserve that intent confidently, it should hard-skip instead of guessing.
