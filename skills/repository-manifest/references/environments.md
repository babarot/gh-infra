# Environments

```yaml
reconcile:
  deployment_branch_policies: authoritative   # default additive
spec:
  environments:
    - name: production            # matched ignoring case; write it as GitHub has it
      wait_timer: 0               # minutes, 0-43200; 0 removes the timer
      prevent_self_review: true   # needs at least one reviewer
      can_admins_bypass: false    # GitHub default true
      reviewers:                  # up to 6; exactly one of user / team each; [] removes all
        - user: babarot
        - team: deployers         # team slug
      deployment_branch_policy:
        custom_branch_policies:
          - name: main            # type defaults to branch
          - name: "v*"
            type: tag
```

`deployment_branch_policy`:

| YAML | Meaning |
|---|---|
| left out | unmanaged |
| `{}` | all branches and tags |
| `protected_branches: true` | protected branches only |
| `custom_branch_policies: [...]` | name patterns (`[]` = none) |

Rules and gotchas:

- environments are never deleted (GitHub creates `github-pages`, `copilot`, and ones a workflow names); `environments: []` only drops RepositorySet defaults
- patterns not declared are deleted only with `reconcile.deployment_branch_policies: authoritative`, and only in environments that set `custom_branch_policies`
- `deployment_branch_policy:` (null) is invalid; `protected_branches` with `custom_branch_policies` is invalid; `protected_branches: false` alone is invalid (use `{}`)
- leaving `custom_branch_policies` makes GitHub delete the patterns; plan shows them as `(removed by GitHub)` in either reconcile mode
- plan fails when a reviewer it adds cannot read the repository (GitHub would silently drop them): add collaborators / team access first
- plan fails for `wait_timer` or reviewers on a private repo unless the owner's plan is Enterprise; it reads the plan (org owner token, or the user's own token with the `user` scope), else accepts only if an environment of the repo already has that kind of rule
- private repos on Free have no environments; Pro/Team private repos allow branch policies and `can_admins_bypass` only
- in a new repository only the owner is added as reviewer by the creating apply; other reviewers (and `prevent_self_review` when none is left) come in a later plan
- plan notes changes that widen deployment: policy to `{}`, reviewers to `[]`, `can_admins_bypass` to true
- in a RepositorySet, environments merge by name (ignoring case) field by field; `reviewers` and `deployment_branch_policy` replace as a whole
