# ADR-007: Merging repositories in the plan body

## Status

Accepted

## Context

`plan` prints a block per repository with changes. When a RepositorySet or a
FileSet makes the same change in many repositories, the body repeats the same
block for each of them, so a rollout to 30 repositories takes 30 blocks.

`plan` is the only review in the workflow gh-infra is built for (plan on a pull
request, apply on merge), and its text output is read in a terminal's
scrollback and in CI logs. Shortening it must not hide anything a reviewer
needs, and must not take over the screen: a pager or a full-screen viewer
would keep the output out of the scrollback and the logs.

The body does not show everything `apply` does. Without `--diff`, a file shows
only its line counts (`+1 -1`); a ruleset's bypass actors and rules are shown
as `1 actors` and `enabled`; a secret's value is never shown; and the branch,
commit message and pull request a FileSet delivers with can be rendered per
repository by templates. Two repositories can therefore read the same while
`apply` does different things to them.

## Decision

Repositories whose blocks print the same are printed once:

```
  ~ 12 repositories                                         2 to update each
    acme/svc-auth  acme/svc-billing  acme/svc-catalog  ...
    ! differs per repository: file contents
      ~ FileSet: 2 files, via pull_request
          ~ .github/workflows/ci.yaml  +2 -1
          ~ README.md                  +1 -1
```

1. Merge on what is printed. Two blocks merge when their bodies and their
   header counts print the same, compared without a terminal so no value is
   cut at the terminal width. No separate equality of changes is defined; a
   merged block is then, by construction, what each of its repositories would
   print alone. Created and destroyed repositories never merge.
2. Note what is not printed. What `apply` does beyond the body is kept per
   part (settings the plan summarizes, file contents, delivery) and compared
   among the merged repositories. A part that differs gets a `!` line saying
   which; repositories are not kept apart for it. Only what `apply` would do
   differently counts: fields a change does not touch and identifiers that
   differ per repository by nature (a deleted ruleset's ID) are left out, so
   the note is not raised for them. The values compared are read the way
   `apply` reads them (`repository.ApplyInputs`, `fileset.DeliveryFor`), so
   the two cannot drift.
3. List every name. A merged block lists all its repositories, in plan
   order, so text output stays reviewable in CI.
4. Rewrite blocks in passes. The body is built as blocks
   (`ui.PlanBlock`), rewritten by a list of passes, and printed by one
   renderer. Each pass is a function from blocks to blocks with a single
   condition (putting a lone child on its parent's line, merging), tested on
   blocks alone, while golden files (`internal/infra/testdata/plan`) pin the
   whole body and property tests check what merging must keep.

JSON and Markdown output are not affected; they list every repository.

## Consequences

- A rollout across many repositories reads as one block, and the summary and
  counts stay those of each repository (`2 to update each`).
- Merging and the `!` line were weighed against two alternatives in a mock.
  Merging only when the unseen parts match too would leave the most common
  case, a template that renders the repository name into a file, as long as
  before, with identical-looking blocks side by side and no reason given.
  Merging without a note would let `each` claim the same change for
  repositories that get different ones. The note says only which part
  differs, not how; showing the difference (with `--diff`, or a key to open
  it) would lengthen the body again, so it is left to future work.
- A new value that the plan summarizes or leaves out must be added to
  `ApplyInputs` (or to what the FileSet delivery compares); otherwise
  repositories that differ in it merge without a note. The property test in
  `internal/infra/plan_merge_property_test.go` derives its expected notes
  from the generated values, not from the code under test, so it can catch a
  part that stops being compared.
- A further rewrite of the body (for the apply results or the Markdown
  output) can be added as another pass without touching the conversion or
  the renderer.
