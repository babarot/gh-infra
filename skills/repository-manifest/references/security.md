# Security

Fields under this section correspond to GitHub's **Advanced Security** settings page.

All Advanced Security fields live under the `security` map.

```yaml
spec:
  security:
    vulnerability_alerts: true
    automated_security_fixes: true
    private_vulnerability_reporting: true
    secret_scanning: true
    secret_scanning_push_protection: true
```

| Field | Endpoint | Notes |
|---|---|---|
| `vulnerability_alerts` | `vulnerability-alerts` | 204 = enabled / 404 = disabled |
| `automated_security_fixes` | `automated-security-fixes` | Requires `vulnerability_alerts: true` (server-side check) |
| `private_vulnerability_reporting` | `private-vulnerability-reporting` | |
| `secret_scanning` | `security_and_analysis.secret_scanning` | Free on public repos; org-owned private/internal repos need GitHub Secret Protection (paid, GitHub Team or Enterprise Cloud); user-owned private repos only on Enterprise Cloud with Enterprise Managed Users |
| `secret_scanning_push_protection` | `security_and_analysis.secret_scanning_push_protection` | Repository-level push protection only (not push protection for users). Requires secret scanning |

The first three use PUT to enable, DELETE to disable on `/repos/{owner}/{repo}/<endpoint>`. The secret scanning fields are read from `GET /repos/{owner}/{repo}` and set with `PATCH /repos/{owner}/{repo}` (`{"security_and_analysis": {"<field>": {"status": "enabled" | "disabled"}}}`).

Validation traps:
- `automated_security_fixes` requires `vulnerability_alerts` to be effectively true. Checked at `plan` (NOT `validate` — the check needs current GitHub state). Effective state = manifest value if set, else current remote value (omission = unmanaged).
- `vulnerability_alerts: false` + `automated_security_fixes: true` → plan fails with explicit error.
- `automated_security_fixes: true` alone (alerts omitted) → plan succeeds only if remote alerts already true; otherwise fails with same error.
- Apply order: when both are set true in one manifest, `vulnerability_alerts` is sent before `automated_security_fixes` (guaranteed by `diffSecurity` child order and `applyAllSettings` for new repos).
- `secret_scanning_push_protection` requires `secret_scanning` to be effectively true. GitHub answers 200 either way: it does not save push protection while scanning is off, and disabling scanning also disables push protection. So:
  - `secret_scanning_push_protection: true` + `secret_scanning: false` → `validate` fails.
  - `secret_scanning_push_protection: true` alone → plan succeeds only if remote scanning is on (new repository: fails, as nothing is read yet).
  - `secret_scanning: false` alone while remote push protection is on → plan fails; add `secret_scanning_push_protection: false`.
  - Apply enables `secret_scanning` first and disables `secret_scanning_push_protection` first (`diffSecurity` child order). A new repository gets them after the Actions settings (`applyAllSettings`): a public one every value set, `false` included, as GitHub creates it with both on; one that is not public only the `true` values, as it is taken to have both off.
- Both fields are read from one `GET /repos/{owner}/{repo}` (`security_and_analysis` and `.permissions.admin`) whenever either is managed. GitHub returns `security_and_analysis` only to repo admins, org owners and security managers, and on a repo that is not public leaves out what it does not offer:
  - Admin, repo not public, field or whole key missing → not offered = off. `plan`: `false` is no change, `true` fails (`ValidateDependencies`; when the same plan makes the repo public, the error says to apply the visibility change first). `import` leaves the fields out without a warning; `import --into` keeps `false`, removes `true`, and in a RepositorySet entry whose defaults enable them writes `false`.
  - Not admin, or public repo, field or key missing → unreadable. `plan` fails the repository (cannot read); `import` leaves the fields out (unread) and `import --into` keeps the manifest's values with a warning.
  - A status other than `enabled`/`disabled` → unreadable, as above.
  - New non-public repo with `secret_scanning: true` → plan note (`CreateNotes`): apply fails if GitHub does not offer it there. With either field `false` → plan note: the disable is not sent, so an owner's security configuration that turns scanning on shows in the next plan.
- Visibility changes: public → private/internal with either field `true` → plan fails (`ValidateDependencies`): whether GitHub offers scanning afterwards depends on Secret Protection, which plan cannot see. Leave both fields out for the visibility change, then set them again. Disabling them in the same plan works: the security changes are planned and applied before the visibility change (`Diff`), while the repo is still public.
- `private_vulnerability_reporting` is independent — no dependency.
