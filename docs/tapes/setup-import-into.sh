#!/usr/bin/env bash
# Setup for import --into demo: pull GitHub state back into local manifests.
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

# Prepare mock data: repo was changed via GitHub GUI.
# Mock returns state that drifted from local YAML:
#   topics: added "rm"
#   discussions: false → true
# Everything else matches the YAML so only these 2 diffs appear.
export MOCK_DIR=/tmp/mock-data
mkdir -p "$MOCK_DIR/babarot/gomi"

cat > "$MOCK_DIR/babarot/gomi/view.json" << 'JSON'
{
  "description": "🗑️ Your UNIX rm command with a safety net!",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [{"name":"trash"},{"name":"go"},{"name":"cli"},{"name":"rm"}],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": false,
  "hasWikiEnabled": false,
  "hasDiscussionsEnabled": true,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON

# Local YAML: stale state (before GUI changes)
mkdir -p /tmp/demo

cat > /tmp/demo/gomi.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: gomi
  owner: babarot

spec:
  description: "🗑️ Your UNIX rm command with a safety net!"
  visibility: public
  topics:
    - trash
    - go
    - cli
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
    pull_requests: true
  merge_strategy:
    allow_merge_commit: true
    allow_squash_merge: true
    allow_rebase_merge: true
    auto_delete_head_branches: true
    squash_merge_commit_title: COMMIT_OR_PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES
    merge_commit_title: MERGE_MESSAGE
    merge_commit_message: PR_TITLE
  actions:
    enabled: true
    allowed_actions: all
    workflow_permissions: write
    can_approve_pull_requests: true
  security:
    vulnerability_alerts: true
YAML

export PS1='$ '
