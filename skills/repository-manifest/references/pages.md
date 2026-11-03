# Pages

`spec.pages` manages the repository's GitHub Pages site (Settings → Pages).

```yaml
spec:
  pages:
    build_type: workflow       # workflow | legacy
    source:                    # legacy only, and required with it
      branch: gh-pages
      path: /                  # / (default) | /docs
    cname: docs.example.com    # "" removes the domain
    https_enforced: true
```

| Field | API | Notes |
|---|---|---|
| `build_type` | POST/PUT `repos/{o}/{r}/pages` | Required to enable a site |
| `source` | POST/PUT `source{branch,path}` | Only with `legacy`; sent with every move to legacy |
| `cname` | PUT `cname` (`""` → `null`) | Lowercase domain only (validate), no scheme, path or trailing dot |
| `https_enforced` | PUT | Waits for the certificate (see below) |

Behavior:
- No `pages` key → the site is never read (`GET .../pages` is skipped) or changed.
- No site on GitHub (GET 404) → plan shows `+ pages` (create); apply POSTs `build_type` (+ `source`), then PUTs `cname` / `https_enforced`.
- gh-infra never disables a site (no DELETE). Removing `pages` from the manifest leaves the site alone.
- A workflow site still reports its last `source`; gh-infra ignores it and import does not write it.
- A repository created by the same apply: Pages is enabled by the next plan (create note).

Validation traps:
- validate: `legacy` without `source`, or `source` without `build_type: legacy` → error. `cname` must be lowercase (GitHub may store another form, which would never converge).
- plan (fails the whole plan): enabling without `build_type`; a `legacy` source branch that does not exist when the site is enabled, its source changes, or it moves to legacy; enabling on a private/internal repo when the owner's plan is Free or cannot be read (same plan lookup as environments; an existing site skips it).
- `https_enforced: true` on a site with a custom domain (declared or current) while GitHub has not issued its certificate (`https_certificate.state != approved`), or together with a new `cname`, or on a new site: plan shows `https_enforced  false → true (once GitHub has issued the certificate)` with a note, apply sends nothing for it, and every plan shows it until the certificate is issued. Set `cname` first; HTTPS follows once DNS points to GitHub Pages. Without a custom domain it is sent at once (GitHub may report no certificate for such a site).
- A site that cannot be read (403) fails the repository at plan; import leaves `pages` out with an unread warning. Token permission: Pages read for plan; Pages and Administration write for apply (classic: `repo` scope).
