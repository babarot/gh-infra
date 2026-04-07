#!/usr/bin/env bash
# Setup for diff viewer demo: FileSet with file changes across 2 repos.
set -euo pipefail

cp /data/.gh-infra /usr/local/bin/gh-infra
chmod +x /usr/local/bin/gh-infra

cat > /usr/local/bin/gh << 'WRAPPER'
#!/usr/bin/env bash
if [[ "$1" == "infra" ]]; then
  shift
  exec /usr/local/bin/gh-infra "$@"
fi
exec /data/mock-gh "$@"
WRAPPER
chmod +x /usr/local/bin/gh

# Prepare mock data: 2 repos with old file contents.
# YAML wants updated content → plan shows file diffs.
export MOCK_DIR=/tmp/mock-data

for repo in app-api app-web; do
  dir="$MOCK_DIR/babarot/${repo}"
  mkdir -p "$dir" "$dir/contents/.github/workflows"

  # Repo settings (needed for plan to not error)
  cat > "$dir/view.json" << 'JSON'
{
  "description": "",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": false,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON

  # Old CODEOWNERS (differs from desired)
  echo -n '* @old-owner' > "$dir/contents/.github/CODEOWNERS"

  # Old CI workflow (differs from desired)
  cat > "$dir/contents/.github/workflows/ci.yml" << 'CI'
name: CI
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: make test
CI
done

mkdir -p /tmp/demo

cat > /tmp/demo/files.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot

spec:
  repositories:
    - app-api
    - app-web

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot @team-platform

    - path: .github/workflows/ci.yml
      content: |
        name: CI
        on: [push, pull_request]
        jobs:
          test:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4
              - run: make lint
              - run: make test

  via: push
YAML

export PS1='$ '
