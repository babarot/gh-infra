# Changelog

## [v0.7.0](https://github.com/babarot/gh-infra/compare/v0.6.2...v0.7.0) - 2026-04-03
### New Features
- feat: add release_immutability support for repositories by @yagihash in https://github.com/babarot/gh-infra/pull/63
### Improvements
- fix: harden type assertions, fetch error handling, and add release_immutability docs by @babarot in https://github.com/babarot/gh-infra/pull/64

## [v0.7.0](https://github.com/babarot/gh-infra/compare/v0.6.2...v0.7.0) - 2026-04-03
### New Features
- feat: add release_immutability support for repositories by @yagihash in https://github.com/babarot/gh-infra/pull/63
### Improvements
- fix: harden type assertions, fetch error handling, and add release_immutability docs by @babarot in https://github.com/babarot/gh-infra/pull/64

## [v0.6.2](https://github.com/babarot/gh-infra/compare/v0.6.1...v0.6.2) - 2026-03-30
### Bug fixes
- fix: replace --body with --input - for gh api calls by @yagihash in https://github.com/babarot/gh-infra/pull/61
### Improvements
- Separate column width calculation per section for independent alignment by @babarot in https://github.com/babarot/gh-infra/pull/56
- Add Ctrl+C cancellation support with graceful context propagation by @babarot in https://github.com/babarot/gh-infra/pull/57
- Unify spinner display with Docker-like live status per repository by @babarot in https://github.com/babarot/gh-infra/pull/58
- Group diff viewer file list by repository by @babarot in https://github.com/babarot/gh-infra/pull/59
### Refactorings
- Refactor: clean architecture, DRY, and scalable design by @babarot in https://github.com/babarot/gh-infra/pull/53

## [v0.6.1](https://github.com/babarot/gh-infra/compare/v0.6.0...v0.6.1) - 2026-03-27
### Improvements
- Improve gh CLI error parsing with HTTP status codes and stdout fallback by @babarot in https://github.com/babarot/gh-infra/pull/52

## [v0.6.0](https://github.com/babarot/gh-infra/compare/v0.5.1...v0.6.0) - 2026-03-26
### New Features
- Add `patches` support for applying unified diffs to file content by @babarot in https://github.com/babarot/gh-infra/pull/48
### Bug fixes
- Fix phantom diff for private GitHub App bypass actors by @babarot in https://github.com/babarot/gh-infra/pull/43
- Add ErrValidation sentinel for HTTP 422 errors by @babarot in https://github.com/babarot/gh-infra/pull/45
- Handle HTTP 403 gracefully when fetching rulesets on Free plan by @babarot in https://github.com/babarot/gh-infra/pull/49
### Improvements
- Error on importing non-existent repository by @babarot in https://github.com/babarot/gh-infra/pull/38
- Support multi-document YAML files in manifest parser by @babarot in https://github.com/babarot/gh-infra/pull/39
- Disable spinners when logging is active by @babarot in https://github.com/babarot/gh-infra/pull/46

## [v0.5.1](https://github.com/babarot/gh-infra/compare/v0.5.0...v0.5.1) - 2026-03-26
### Improvements
- Add sha_pinning_required, fix validation, and improve docs/tests by @babarot in https://github.com/babarot/gh-infra/pull/35

## [v0.5.0](https://github.com/babarot/gh-infra/compare/v0.4.0...v0.5.0) - 2026-03-26
### New Features
- Add GitHub Actions settings management by @babarot in https://github.com/babarot/gh-infra/pull/34
### Refactorings
- Add custom struct tag validator for manifest types by @babarot in https://github.com/babarot/gh-infra/pull/32

## [v0.4.0](https://github.com/babarot/gh-infra/compare/v0.3.0...v0.4.0) - 2026-03-25
### Bug fixes
- fix: support exempt bypass mode in rulesets validation by @yagihash in https://github.com/babarot/gh-infra/pull/29
### Deprecated features
- Rename sync_mode to reconcile and commit_strategy to on_apply by @babarot in https://github.com/babarot/gh-infra/pull/24
- Deprecate on_drift and simplify file change handling by @babarot in https://github.com/babarot/gh-infra/pull/25
- Rename on_apply to via and improve docs by @babarot in https://github.com/babarot/gh-infra/pull/26
### Improvements
- Show diff stat in file change plan output by @babarot in https://github.com/babarot/gh-infra/pull/27
- Highlight delivery method in plan output by @babarot in https://github.com/babarot/gh-infra/pull/30

## [v0.3.0](https://github.com/babarot/gh-infra/compare/v0.2.0...v0.3.0) - 2026-03-25
### New Features
- Add interactive diff viewer with on_drift toggle to apply flow by @babarot in https://github.com/babarot/gh-infra/pull/17
### Bug fixes
- Fix invalid YAML output from gh infra import by @babarot in https://github.com/babarot/gh-infra/pull/21
### Improvements
- FileSet improvements, error handling fixes, and bubbletea escape leak workaround by @babarot in https://github.com/babarot/gh-infra/pull/16
- Add sync_mode: create_only for seed files by @babarot in https://github.com/babarot/gh-infra/pull/18
- Improve resource type labels and show commit strategy in output by @babarot in https://github.com/babarot/gh-infra/pull/19
### Refactorings
- Refactor concurrency with generics and adopt Go 1.26 features by @babarot in https://github.com/babarot/gh-infra/pull/14

## [v0.2.0](https://github.com/babarot/gh-infra/compare/v0.1.3...v0.2.0) - 2026-03-24
### New Features
- Add sync_mode: mirror for directory-level file sync by @babarot in https://github.com/babarot/gh-infra/pull/11
### Improvements
- Silently skip unknown YAML kinds by default by @babarot in https://github.com/babarot/gh-infra/pull/5
- Unify spinner tracker across repository and fileset processing by @babarot in https://github.com/babarot/gh-infra/pull/7
- Unified plan output for repository and fileset changes by @babarot in https://github.com/babarot/gh-infra/pull/8
- Unified apply results and spinner label improvements by @babarot in https://github.com/babarot/gh-infra/pull/12
- Terraform-style plan output and ProgressReporter interface by @babarot in https://github.com/babarot/gh-infra/pull/13
### Refactorings
- Fix separation of concerns across packages by @babarot in https://github.com/babarot/gh-infra/pull/9
- Rename strategy to commit_strategy and direct to push by @babarot in https://github.com/babarot/gh-infra/pull/10

## [v0.1.3](https://github.com/babarot/gh-infra/compare/v0.1.2...v0.1.3) - 2026-03-24

## [v0.1.2](https://github.com/babarot/gh-infra/compare/v0.1.1...v0.1.2) - 2026-03-24

## [v0.1.1](https://github.com/babarot/gh-infra/compare/v0.1.0...v0.1.1) - 2026-03-24

## [v0.1.0](https://github.com/babarot/gh-infra/commits/v0.1.0) - 2026-03-24
