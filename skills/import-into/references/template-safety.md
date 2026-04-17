# Template Safety

## Reverse Mapping

Template-backed files are not compared as raw template text.

gh-infra:

1. renders the local template for the target repository
2. compares rendered content with the GitHub file
3. attempts to reconstruct safe local source while preserving placeholders

## Importable Cases

Usually safe:

- literal version bumps near preserved placeholders
- punctuation or neighboring text changes around placeholder-backed values

## Hard-Skip Cases

Usually unsafe:

- placeholder-derived lines were removed entirely
- placeholder-derived content was rewritten ambiguously
- the template uses structure gh-infra cannot safely reverse

## Decision Rule

- if the remote file still clearly matches the local template shape, import may proceed
- if the remote file no longer resembles the template structure, expect a hard skip
