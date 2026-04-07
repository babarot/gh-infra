#!/usr/bin/env bash
# Setup for intro demo (import → edit → plan).
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

# Prepare mock data: two repos with different settings
export MOCK_DIR=/tmp/mock-data
mkdir -p "$MOCK_DIR/babarot/my-project" "$MOCK_DIR/babarot/my-service"

cat > "$MOCK_DIR/babarot/my-project/view.json" << 'JSON'
{
  "description": "My project",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": true,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": false,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": false,
  "defaultBranchRef": { "name": "main" }
}
JSON

cat > "$MOCK_DIR/babarot/my-service/view.json" << 'JSON'
{
  "description": "My service",
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

mkdir -p /tmp/demo
export GH_INFRA_OUTPUT="${GH_INFRA_OUTPUT:-}"
export PS1='$ '
