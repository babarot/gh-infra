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
| `secret_scanning` | `PATCH /repos/{owner}/{repo}` | `security_and_analysis.secret_scanning.status`; read from the same object |
| `secret_scanning_push_protection` | `PATCH /repos/{owner}/{repo}` | `security_and_analysis.secret_scanning_push_protection.status`; requires `secret_scanning` |

The first three use PUT to enable, DELETE to disable on `/repos/{owner}/{repo}/<endpoint>`. The secret scanning pair is a repos PATCH of `security_and_analysis` (`enabled` / `disabled`), checked against the response.

Validation traps:
- `automated_security_fixes` requires `vulnerability_alerts` to be effectively true. Checked at `plan` (NOT `validate` — the check needs current GitHub state). Effective state = manifest value if set, else current remote value (omission = unmanaged).
- `vulnerability_alerts: false` + `automated_security_fixes: true` → plan fails with explicit error.
- `automated_security_fixes: true` alone (alerts omitted) → plan succeeds only if remote alerts already true; otherwise fails with same error.
- Apply order: when both are set true in one manifest, `vulnerability_alerts` is sent before `automated_security_fixes` (guaranteed by `diffSecurity` child order and `applyAllSettings` for new repos).
- `private_vulnerability_reporting` is independent — no dependency.
- `secret_scanning_push_protection: true` requires `secret_scanning` to be effectively true, checked at `plan` with the same effective-state rule as above. Apply sends `secret_scanning` before `secret_scanning_push_protection`.
- `security_and_analysis` is returned only to repository admins. When it is missing, plan fails for that repository (import leaves both fields out) rather than reading them as disabled.
- GitHub answers 200 and keeps the old status for what a repository cannot have (e.g. `secret_scanning: false` on a public repository, or any of it on a private repository without GitHub Secret Protection); apply reports that as failed.
