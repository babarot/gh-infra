---
title: Why gh-infra
sidebar:
  order: 0
---

## The Problem

You have 10, 20, maybe 50 GitHub repositories. You want consistent settings across all of them — the same merge strategy, branch protection rules, topics, and shared files like CODEOWNERS and LICENSE.

The obvious answer is **Infrastructure as Code**. And for GitHub, that usually means Terraform.

### Why not Terraform?

The [Terraform GitHub Provider](https://registry.terraform.io/providers/integrations/github/latest/docs) works. But for personal or small-team use, it comes with real overhead:

- **State file management** — You need a remote backend (S3, GCS, Terraform Cloud) to store and lock state. Without it, you risk state corruption or conflicts.
- **HCL learning curve** — Another language to learn and maintain. For simple repo settings, HCL feels heavy.
- **Provider setup** — Installing the provider, configuring authentication, pinning versions.
- **CI/CD pipeline** — You need a pipeline to run `terraform plan` and `terraform apply`, with secrets for the backend and GitHub token.
- **State drift** — If someone changes a setting through the GitHub UI, your state file is now out of sync. You need `terraform import` or `terraform refresh` to fix it.

All of this makes sense for managing cloud infrastructure at scale. But for managing **GitHub repository settings**, it's too much.

### Other alternatives

Tools like [Probot Settings](https://github.com/probot/settings) exist, but they apply changes **immediately on push** with no preview step. You can't see what will change before it happens.

## gh-infra's Approach

gh-infra takes a different path:

- **YAML instead of HCL** — Declare what your repos should look like in plain YAML. No new language to learn.
- **No state file** — GitHub itself is the source of truth. Every `plan` fetches the live state and diffs directly. There's nothing to store, lock, or lose.
- **`plan` before `apply`** — See exactly what will change before it happens. Review the diff, then apply with confidence.
- **Just `gh` and a token** — No provider installation, no remote backend, no GitHub App. If you can run `gh`, you can run `gh infra`.

### Comparison

| | Terraform | gh-infra |
|---|---|---|
| Language | HCL | YAML |
| State file | Required (remote backend) | None |
| Preview changes | `terraform plan` | `gh infra plan` |
| Authentication | Provider config + token | `gh auth` (already set up) |
| State drift | Manual import/refresh | Automatic (always fetches live state) |
| Setup time | 30+ minutes | 1 command |

## Who is it for?

- **Individual developers** managing personal repositories
- **Small teams** that want consistent repo settings without Terraform overhead
- **Organizations** that need to distribute shared files (LICENSE, CODEOWNERS, CI workflows) across many repos
- Anyone who thinks "I just want to change a repo setting in code" shouldn't require a state backend

Ready to try it? Head to [Getting Started](/introduction/getting-started/).
