---
paths:
  - "internal/repository/state.go"
  - "internal/repository/state_types.go"
  - "internal/manifest/repository_types.go"
---

# Keep mock-gh and VHS setup scripts in sync with API changes

When adding, removing, or renaming fields fetched from the GitHub API (GraphQL via `gh repo view --json` or REST via `gh api`), you **must** update the following files to match:

1. **`docs/tapes/mock-gh`** — the generic mock script. Update the default JSON fallbacks for both `view.json` (GraphQL fields) and `commit-settings.json` (REST API fields), plus any field-matching patterns.
2. **`docs/tapes/setup*.sh`** — every setup script that generates `view.json` or `commit-settings.json`. Each heredoc must include the new/changed field.
3. **`internal/repository/state_test.go`** — mock response map keys contain the full `--json` / `--jq` field list; these must match the production query exactly.

## Important: GraphQL vs REST API fields

Not all GitHub repository fields are available via GraphQL (`gh repo view --json`). Some fields (e.g. `allow_auto_merge`, commit message settings) are only available via the REST API (`gh api repos/{owner}/{repo}`). Before adding a new field:

1. Check `gh repo view --json` available fields by running `gh repo view --json` with no value
2. If the field is not in GraphQL, fetch it via the REST API path (`fetchCommitMessageSettings` or a new fetcher)
3. Place mock data in the correct mock response (`view.json` for GraphQL, `commit-settings.json` for REST)

## Checklist

- [ ] `mock-gh` default JSON fallbacks include the field in the correct response (GraphQL or REST)
- [ ] Every `setup*.sh` heredoc for the relevant mock file includes the field
- [ ] `state_test.go` mock keys match the updated query strings
- [ ] `state_test.go` mock JSON responses include the field with a sensible value
