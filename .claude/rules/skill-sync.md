---
paths:
  - "docs/src/content/docs/**"
  - "skills/**"
---

# Agent Skill ↔ Documentation Sync

The Astro documentation (`docs/src/content/docs/`) is the **source of truth** for user-facing feature descriptions. The agent skills (`skills/`) are an **agent-optimized view** of the same information, tailored for LLM consumption. (`.agents/skills` is a symlink to `skills/` for local agent resolution.)

When either side changes, check whether the other side needs updating.

## Mapping

| Documentation | Skill | Reference files |
|---|---|---|
| `resources/repository/general.md` | `repository-manifest` | `references/general.md` |
| `resources/repository/labels.md` | `repository-manifest` | `references/labels.md` |
| `resources/repository/milestones.md` | `repository-manifest` | `references/milestones.md` |
| `resources/repository/actions.md` | `repository-manifest` | `references/actions.md` |
| `resources/repository/rulesets.md`, `resources/repository/branch-protection.md` | `repository-manifest` | `references/protection.md` |
| `resources/repository/secrets-variables.md` | `repository-manifest` | `references/secrets-variables.md` |
| `resources/repository/security.md` | `repository-manifest` | `references/security.md` |
| `resources/repository-set/defaults.md`, `resources/repository-set/index.md` | `repository-manifest` | `references/repository-set.md` |
| `resources/file/sources.md`, `resources/file/templating.md` | `file-manifest` | `references/sources-and-templating.md` |
| `resources/file/delivery.md`, `resources/file/index.md` | `file-manifest` | `references/reconcile-and-delivery.md` |
| `resources/fileset/index.md`, `resources/fileset/overrides.md` | `file-manifest` | `references/fileset.md` |
| `commands/import.md`, `internals/import-into.md` | `import-into` | `references/behavior.md`, `references/write-modes.md`, `references/template-safety.md` |
| `commands/plan.md`, `commands/apply.md`, `commands/validate.md`, `commands/import.md` | `gh-infra` | `references/commands.md` |
| `patterns/ci-cd.md` | `ci-cd` | `references/self-managed.md`, `references/central.md` |

## Rules

1. **Doc updated → check skill**: When a doc file changes, read the mapped skill reference and verify it still reflects the doc content. Update the skill reference if it has become stale. Do not copy verbatim; skill references are concise, schema-focused, and include agent-specific gotchas that docs may not have.
2. **Skill updated → check doc**: When a skill reference changes (e.g., adding a new gotcha or correcting a field), verify the corresponding doc is consistent. If the doc is missing information that the skill added, flag it to the user.
3. **New feature added**: When a new spec field, command flag, or resource kind is added to the Go code, ensure both the doc and the corresponding skill reference are updated.
4. **Do not strip agent-specific content from skills**: Skill references may contain validation traps, gotchas, and merge-behavior notes that are not in the docs. These are intentional and should be preserved.
