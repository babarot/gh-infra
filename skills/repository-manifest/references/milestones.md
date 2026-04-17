# Milestones

## Basic Shape

```yaml
spec:
  milestones:
    - title: "v1.0"
      description: "First stable release"
      state: open
      due_on: "2026-06-01"
```

- `title` must be unique in the list
- `state`: `open` (default when omitted) or `closed`
- `due_on`: `YYYY-MM-DD` format; converted to `T00:00:00Z` for the API

## Behavior

- **Additive only**: create and update; never deletes milestones
- No `milestone_sync` or mirror mode — milestones have issues attached so auto-delete is dangerous
- To retire a milestone, set `state: closed`
- Removing a milestone from the manifest leaves it unchanged on GitHub

## API Details

- Fetched via `GET /repos/{owner}/{repo}/milestones?state=all`
- Created via `POST /repos/{owner}/{repo}/milestones`
- Updated via `PATCH /repos/{owner}/{repo}/milestones/{number}`
- Title is the human identifier; API uses `number` (resolved internally)

## Gotchas

- `due_on` from the API is RFC3339 (`2026-06-01T00:00:00Z`); normalized to `YYYY-MM-DD` during fetch to prevent perpetual diff loops
- `state` defaults to `open` when nil/omitted — an existing `open` milestone with no `state` in the manifest produces no diff
- Milestones are fetched in parallel with other sub-resources; fetch errors are non-fatal (returns nil)

## RepositorySet Merge

Per-repo `milestones` fully replaces defaults (same as labels — not a deep merge).
