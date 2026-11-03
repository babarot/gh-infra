---
title: Pages
sidebar:
  order: 10
---

The `pages` key manages the repository's GitHub Pages site: how it is built and its custom domain. It corresponds to **Settings → Pages** on GitHub.

```yaml
spec:
  pages:
    build_type: workflow   # workflow | legacy
    cname: docs.example.com
    https_enforced: true
```

| Field | Type | Description |
|-------|------|-------------|
| `build_type` | string | `workflow` (built and deployed by a GitHub Actions workflow) or `legacy` (built from a branch) |
| `source.branch` | string | The branch a `legacy` site is built from |
| `source.path` | string | `/` (default) or `/docs`: the folder of `source.branch` the site is built from |
| `cname` | string | The custom domain, a lowercase domain name such as `docs.example.com`. `""` removes it |
| `https_enforced` | bool | Redirect HTTP to HTTPS |

As everywhere in gh-infra, a repository without `pages` is left alone: its site is not read or changed. Within `pages`, a field left out is not compared or sent.

## Enabling a site

When the repository has no site, `plan` shows one to create, and `apply` enables it with the declared `build_type` (and `source`), then sets `cname` and `https_enforced`. `build_type` is required to enable a site.

```
  ~ babarot/my-project
      + pages
          + build_type  workflow
          + cname       docs.example.com
```

gh-infra does not disable a site. Removing `pages` from the manifest leaves the site as it is; disable it in the repository settings.

A repository the same `apply` creates has no commits to build a site from, so its site is enabled by the next `plan` and `apply` (the plan says so).

GitHub Free has Pages only on public repositories. Enabling a site on a private or internal repository needs the owner's plan to be read, as for [environment protection rules](./environments/#what-plan-checks): `plan` fails when the plan is Free, or when it cannot be read.

## Building from a branch

A `legacy` site is built from `source`, which is required with `build_type: legacy` and not allowed with `workflow`:

```yaml
spec:
  pages:
    build_type: legacy
    source:
      branch: gh-pages
      path: /
```

`plan` fails when the branch does not exist and the site needs it: when it is enabled, its source changes, or it moves to `legacy`.

A `workflow` site has no source. GitHub keeps reporting the last one it had, which gh-infra ignores, and `import` does not write it.

## HTTPS and the certificate

A site without a custom domain is served from GitHub's domain (or from the domain of the owner's user site), and `https_enforced` is set at once. With a custom domain, GitHub refuses `https_enforced: true` until it has issued a certificate for it, which it does in its own time after the domain is set (DNS has to point to GitHub Pages). When the certificate is not issued yet, or the same `apply` sets the domain, `plan` shows the change as waiting, and `apply` does not send it:

```
  ~ babarot/my-project
      ~ pages
          ~ cname           (none) → docs.example.com
          ~ https_enforced  false → true (once GitHub has issued the certificate)
      pages: https_enforced is applied by a later plan, once GitHub has issued the certificate for the site's domain
```

Every `plan` shows it until the certificate is issued; then `apply` enforces HTTPS.

## Permissions

Reading the site needs the **Pages** repository permission (read) of a fine-grained token; enabling and changing it (`apply`) needs **Pages** and **Administration** (write). A classic token needs the `repo` scope, and the user must be an administrator or maintainer of the repository, or have the "manage GitHub Pages settings" permission. A site that cannot be read fails the repository at `plan`, and `import` leaves `pages` out with a warning.
