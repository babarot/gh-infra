---
title: JSON output
sidebar:
  order: 4
---

`plan` and `apply` can print their result as JSON for CI jobs and AI agents:

```bash
gh infra plan ./repos/ --output json
gh infra apply ./repos/ --output json --auto-approve
```

With `--output json`, stdout has only the JSON document; the progress, the plan table and the summary are not printed. `apply` needs `--auto-approve`, since the confirmation prompt cannot be shown. Exit status is the same as with text output.

## Document

```json
{
  "format_version": 1,
  "command": "plan",
  "summary": { "create": 1, "update": 2, "delete": 0 },
  "changes": [
    {
      "target": "babarot/my-cli",
      "resource": "Repository",
      "action": "update",
      "field": "description",
      "before": "old",
      "after": "new"
    },
    {
      "target": "babarot/my-cli",
      "resource": "Repository",
      "action": "update",
      "field": "features",
      "details": [
        { "field": "wiki", "action": "update", "before": true, "after": false }
      ]
    },
    {
      "target": "babarot/my-cli",
      "resource": "Ruleset",
      "action": "create",
      "name": "main-protection",
      "after": "main-protection",
      "details": [
        { "field": "enforcement", "action": "create", "after": "active" }
      ]
    },
    {
      "target": "babarot/my-cli",
      "resource": "File",
      "action": "update",
      "path": ".github/workflows/ci.yaml",
      "fileset": "babarot",
      "via": "push",
      "lines": { "added": 3, "removed": 1 }
    }
  ],
  "warnings": [],
  "errors": []
}
```

| Field | Description |
|---|---|
| `format_version` | `1`. Raised only when a field is removed or changes meaning; new fields keep it |
| `command` | `plan` or `apply` |
| `summary` | Number of changes by action. `apply` adds `applied`, `failed` and `skipped` |
| `changes` | One entry per change, in the order `plan` prints them. The counts in `summary` match these entries |
| `warnings` | Deprecations found while parsing, and notes such as a commit that will not be signed. `target` is set when it is about one repository |
| `errors` | Repositories that could not be planned, with the error. They are left out of `changes` |

### Change entries

| Field | Description |
|---|---|
| `target` | Repository (`owner/repo`) |
| `resource` | `Repository`, `Actions`, `BranchProtection`, `Ruleset`, `Label`, `Milestone`, `Secret`, `Variable`, or `File` |
| `action` | `create`, `update`, or `delete` |
| `name` | Name of a named resource: the ruleset, branch protection pattern, label, milestone, secret, or variable |
| `field` | Setting name for repository settings, such as `description` or `features` |
| `path` | File path, for `File` |
| `before`, `after` | The values `plan` shows. They are display values, not API payloads (for example `"3 actors"` for bypass actors). Secret values never appear |
| `details` | Changes inside a grouped setting, with the same `field`, `action`, `before` and `after` |
| `lines` | Added and removed lines, for `File` |
| `mode` | `before` and `after` file mode, for `File` when the mode changes |
| `diff` | Unified diff, for `File` with `plan --diff` (up to 500 lines) |
| `fileset`, `via` | FileSet owner and delivery method (`push` or `pull_request`), for `File` |
| `status` | `apply` only: `applied`, `failed`, or `skipped` (no content change on GitHub) |
| `error` | `apply` only: the error when `status` is `failed` |
| `pr_url` | `apply` only: the pull request for a `File` delivered via `pull_request` |

## Examples

Fail a workflow when a plan would delete something:

```bash
gh infra plan ./repos/ -o json | jq -e '.summary.delete == 0'
```

List every change on one line each, with grouped settings flattened into one line per setting:

```bash
gh infra plan ./repos/ -o json | jq -r '
  .changes[] | . as $c | (.details // [{}])[]
  | [$c.target, $c.resource, ($c.name // $c.path // ""), ([$c.field, .field] | map(select(.)) | join(".")), (.action // $c.action)]
  | @tsv'
```

Show the changes that failed in an apply:

```bash
gh infra apply ./repos/ -o json --auto-approve | jq '.changes[] | select(.status == "failed") | {target, resource, name, field, error}'
```

Errors that stop the command before a plan is made, such as an invalid manifest, are printed to stderr as usual, with no JSON on stdout.
