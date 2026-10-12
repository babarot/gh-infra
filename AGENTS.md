# AGENTS.md

## Design Principles

`plan` is the only review in the workflow gh-infra is built for (plan on a pull request, apply on merge), so every change must keep these three rules. The user-facing version is `docs/src/content/docs/introduction/principles.md`; keep both in sync.

1. `apply` does what `plan` shows. Every change `apply` makes is in the plan, and nothing else. What GitHub would reject is caught by `validate` or `plan` (for example in `repository.ValidateDependencies`), not found at `apply`; a setting GitHub silently does not save is caught after the PATCH (`verifyPatchResponse`) and reported as failed. A setting or file that cannot be read is reported and its repository left out, never compared as absent or at its default. When order matters (create before FileSets, archive last), `apply` keeps it and the plan lists changes in that order. `apply` does not read state back and re-plan. Known gap: a `via: pull_request` FileSet on a repository with no commits is pushed, while the plan shows `pull_request` (#262).
2. What a manifest leaves out is left alone. A field not in the manifest is not compared and not sent. Collections are additive; labels, rulesets and branch protection delete undeclared entries only with `reconcile: authoritative` and the collection present. Secrets, variables and milestones never delete. For settings the API replaces as a whole (branch protection, rulesets), `apply` reads the current object and sends it back with only the manifest's fields changed, including fields gh-infra does not model.
3. After `apply`, `plan` shows no changes. One apply converges. Anything a run cannot apply is said in the plan (for a new repository: `repository.CreateNotes`), never left for a later surprise. A `via: pull_request` FileSet keeps showing its changes until the pull request is merged, by design. Known gap: #259.

### Keeping them

- The property tests run plan, apply and plan again against a fake GitHub that holds state in the API's JSON shape and fails on commands it does not know: `internal/repository/apply_property_test.go`, `apply_property_rules_test.go`, `apply_property_new_test.go`, with the fake in `fakegithub_test.go` and `fakegithub_rules_test.go`. A new field, setting or resource gets a case there, and the fake learns any GitHub behavior it depends on (what GitHub rejects or fills in).
- Check that a new test catches what it covers: break the code on purpose, see the test fail, then restore it.
- `go test` has no terminal, so the spinner runs in its plain fallback; test the spinner model (`internal/ui/refresh.go`) directly for anything about its progress or ending.
- Rules for specific areas are in `.claude/rules/*.md` (command output, skills and docs sync, mocks, GitHub Actions, ADRs, renames). They apply to every agent, not only Claude.

## Package Responsibilities

| Package | Role |
|---|---|
| `cmd` | Cobra CLI command definitions and flag handling |
| `internal/infra` | Orchestrator — drives plan, apply, import workflows |
| `internal/fileset` | File-set resolution, templating, patching |
| `internal/importer` | Import logic: match existing repos, reverse-template, write YAML |
| `internal/repository` | Per-repository state, diff, plan, apply, export |
| `internal/manifest` | YAML manifest parsing, validation, type definitions, source expansion, and GitHub identity resolution |
| `internal/gh` | GitHub CLI (`gh`) runner abstraction |
| `internal/ui` | Terminal UI: printer, diff viewer, progress, confirm |
| `internal/parallel` | Generic concurrent map utility |
| `internal/logger` | slog-based structured logger |
| `internal/yamledit` | In-place YAML node editor (preserves formatting) |

## Package Dependency Rules

The internal packages form a clean DAG (no circular dependencies). Respect the following layering:

```
Layer 0 (leaves):  logger, parallel, yamledit   — no internal imports allowed
Layer 1:           gh (→ logger), ui (→ logger)
Layer 2:           manifest (→ gh)
Layer 3:           fileset (→ manifest, gh, parallel), repository (→ manifest, gh, logger, parallel)
Layer 4:           importer (→ fileset, repository, manifest, gh, yamledit)
Layer 5:           infra (→ orchestrator, may import all above + ui)
Layer 6:           cmd (→ infra, ui, manifest, gh, logger, fileset)
```

Key constraints:
- `internal/ui` must NOT depend on domain packages (manifest, repository, infra, etc.) — it is a pure presentation layer
- **Domain packages (repository, importer, fileset, manifest) must NOT depend on `internal/ui` in production code** — the dependency DAG is strictly top-down. When domain packages need progress/diagnostic callbacks, they define their own interfaces and receive implementations from `infra` (Layer 5). Do NOT "unify" these interfaces into `ui` — that would leak presentation concerns into the domain layer. (Test files (`*_test.go`) may import `ui` for test helpers such as formatting utilities; this does not affect the production dependency graph.)
- `internal/manifest` must NOT depend on upper layers — it is the manifest/schema layer and also owns manifest-adjacent resolution logic (for example source expansion and GitHub identity resolution)
- `internal/infra` is the orchestrator; other internal packages must NOT depend on it (except cmd)
- Leaf packages (logger, parallel, yamledit) must NOT import any internal package
- `cmd` should access workflow/domain logic through `infra` where practical. Small direct imports of lower layers are acceptable for thin CLI concerns such as validation/path resolution or shared CLI-only types (for example `manifest` in `validate`, `fileset` for diff-entry helpers).
