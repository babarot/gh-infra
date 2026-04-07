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
#   topics: added "docker"
#   discussions: false → true
# Everything else matches the YAML so only these 2 diffs appear.
export MOCK_DIR=/tmp/mock-data
mkdir -p "$MOCK_DIR/babarot/my-project"

cat > "$MOCK_DIR/babarot/my-project/view.json" << 'JSON'
{
  "description": "My project",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [{"name":"go"},{"name":"cli"},{"name":"docker"}],
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

cat > /tmp/demo/my-project.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project
  owner: babarot

spec:
  description: "My project"
  visibility: public
  topics:
    - go
    - cli
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
  merge_strategy:
    allow_squash_merge: true
    auto_delete_head_branches: true
YAML

export PS1='$ '
