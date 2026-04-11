# Changelog

## [v0.10.0](https://github.com/babarot/gh-infra/compare/v0.9.1...v0.10.0) - 2026-04-11
### New Features
- Add milestone management and improve plan display grouping by @babarot in https://github.com/babarot/gh-infra/pull/104
### Bug fixes
- Propagate unexpected errors from sub-resource fetching by @babarot in https://github.com/babarot/gh-infra/pull/109
- fix: resolve false-positive merge strategy diffs when GitHub returns null by @yagihash in https://github.com/babarot/gh-infra/pull/116
### Improvements
- Add clickable terminal hyperlinks for repository names by @babarot in https://github.com/babarot/gh-infra/pull/107
- Sort sub-resources for deterministic export output by @babarot in https://github.com/babarot/gh-infra/pull/114
- Suppress ANSI color codes in CI and NO_COLOR environments by @babarot in https://github.com/babarot/gh-infra/pull/115
### Refactorings
- Replace hardcoded indent magic numbers with IndentLevel system by @babarot in https://github.com/babarot/gh-infra/pull/106
- Drop huh dependency in favor of inline bubbletea confirm model by @babarot in https://github.com/babarot/gh-infra/pull/108
- Replace global DefaultResolver with explicit SourceResolver injection by @babarot in https://github.com/babarot/gh-infra/pull/110
- Replace long positional parameter lists with typed option structs by @babarot in https://github.com/babarot/gh-infra/pull/111

## [v0.9.1](https://github.com/babarot/gh-infra/compare/v0.9.0...v0.9.1) - 2026-04-08
### Bug fixes
- Replace DiffEntry.Skip bool with Action-based model by @babarot in https://github.com/babarot/gh-infra/pull/97
- Fix literal block scalar corruption in yamledit by @babarot in https://github.com/babarot/gh-infra/pull/100
### Improvements
- Support multiple paths in plan/apply/validate commands by @babarot in https://github.com/babarot/gh-infra/pull/103

## [v0.9.0](https://github.com/babarot/gh-infra/compare/v0.8.0...v0.9.0) - 2026-04-07
### New Features
- Add label management support for repositories by @babarot in https://github.com/babarot/gh-infra/pull/91
- Add label sync mode (additive/mirror) by @babarot in https://github.com/babarot/gh-infra/pull/93
### Improvements
- Import action selector: write / patch / skip per file by @babarot in https://github.com/babarot/gh-infra/pull/83
- Extract withTrackerCancelContext and propagate context through Diff by @babarot in https://github.com/babarot/gh-infra/pull/84
- Default create_only files to skip in import with soft/hard skip distinction by @babarot in https://github.com/babarot/gh-infra/pull/85
- Add per-file status callback to DiffFiles for granular progress reporting by @babarot in https://github.com/babarot/gh-infra/pull/86
- Reverse-map template placeholders during import by @babarot in https://github.com/babarot/gh-infra/pull/87
### Refactorings
- Introduce local interfaces to decouple fileset/repository from ui package by @babarot in https://github.com/babarot/gh-infra/pull/81
- Consolidate formatImportValue into shared FormatValue by @babarot in https://github.com/babarot/gh-infra/pull/90

## [v0.8.0](https://github.com/babarot/gh-infra/compare/v0.7.0...v0.8.0) - 2026-04-05
### New Features
- Add `import --into` command for pulling GitHub state into local manifests by @babarot in https://github.com/babarot/gh-infra/pull/68
### Bug fixes
- Fix FileSet identity collision causing duplicate apply by @babarot in https://github.com/babarot/gh-infra/pull/70
### Improvements
- Show step-level progress during apply and parallelize FileSets by @babarot in https://github.com/babarot/gh-infra/pull/71
- Truncate spinner errors to one line and show detailed summary after by @babarot in https://github.com/babarot/gh-infra/pull/72
- Auto-generate patches for shared templates during import by @babarot in https://github.com/babarot/gh-infra/pull/75
- Patch only changed fields in YAML instead of replacing entire spec by @babarot in https://github.com/babarot/gh-infra/pull/76
### Refactorings
- fix: batch repo settings into single PATCH to fix create validation error by @babarot in https://github.com/babarot/gh-infra/pull/73
- Refactor: simplify yamledit API with shorter names and shared pathContext by @babarot in https://github.com/babarot/gh-infra/pull/77
- Refactor: replace switch-case with descriptor-based field dispatch in patchRepositorySpec by @babarot in https://github.com/babarot/gh-infra/pull/78
- Introduce editOp abstraction for unified YAML edit operations by @babarot in https://github.com/babarot/gh-infra/pull/79

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
