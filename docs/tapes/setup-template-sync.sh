#!/usr/bin/env bash
# Setup for the template sync demo (demo-template-sync.tape): a github-config
# style directory with twelve repositories, six of which sync shared files
# from templates/ by pull request. Editing a template and applying opens a
# pull request on each of those six; one repository has also drifted on
# GitHub (its wiki was turned on).
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

export MOCK_DIR=/tmp/mock-data
export MOCK_DELAY=0.6
work=/tmp/demo/github-config
mkdir -p "$work/templates/common/.github/workflows"

cat > "$work/templates/common/.github/release.yml" << 'YAML'
changelog:
  exclude:
    labels:
      - tagpr
      - changelog/skip
  categories:
    - title: Breaking Changes
      labels:
        - kind/breaking-change
    - title: New Features
      labels:
        - kind/feature
    - title: Bug fixes
      labels:
        - kind/bug
    - title: Deprecated features
      labels:
        - kind/deprecation
    - title: Improvements
      labels:
        - kind/improvement
    - title: Refactorings
      labels:
        - kind/cleanup
    - title: Others
      labels:
        - "*"
YAML

cat > "$work/templates/common/.github/workflows/pr-labeler.yaml" << 'YAML'
name: PR Labeler

on:
  pull_request_target:
    types: [opened, synchronize]

permissions:
  contents: read
  pull-requests: write

jobs:
  label:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/labeler@bf12e9b00b37c5c0ca2b87b79b2daf7891dbda13 # v7.0.0
        with:
          configuration-path: .github/pr-labeler.yaml
          sync-labels: true
YAML

# repo <name> <description> <homepage> <topics, comma separated> [<pr number>]
# writes <name>.yaml and the mock's state for it, which matches the manifest
# unless wiki=true is set for the call (the wiki was turned on on GitHub).
# With a pull request number, the repository also syncs the templates; the
# mock holds them as they are now, before the demo edits them.
repo() {
  local name="$1" description="$2" homepage="$3" topics="$4" pr="${5:-}"
  local dir="$MOCK_DIR/babarot/$name" yaml="$work/$name.yaml" t
  mkdir -p "$dir"

  {
    echo "apiVersion: gh-infra/v1"
    echo "kind: Repository"
    echo "metadata:"
    echo "  name: $name"
    echo "  owner: babarot"
    echo "spec:"
    echo "  description: \"$description\""
    [[ -n "$homepage" ]] && echo "  homepage: $homepage"
    echo "  visibility: public"
    if [[ -n "$topics" ]]; then
      echo "  topics:"
      for t in ${topics//,/ }; do echo "  - $t"; done
    fi
    cat << 'YAML'
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
  merge_strategy:
    allow_merge_commit: true
    allow_squash_merge: true
    allow_rebase_merge: true
    auto_delete_head_branches: true
YAML
  } > "$yaml"

  local json_topics="" t
  for t in ${topics//,/ }; do
    json_topics+="${json_topics:+,}{\"name\":\"$t\"}"
  done
  cat > "$dir/view.json" << JSON
{
  "description": "$description",
  "homepageUrl": "$homepage",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [$json_topics],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": false,
  "hasWikiEnabled": ${wiki:-false},
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON

  [[ -z "$pr" ]] && return
  echo "$pr" > "$dir/pr-number"
  mkdir -p "$dir/contents/.github/workflows"
  cp "$work/templates/common/.github/release.yml" "$dir/contents/.github/release.yml"
  cp "$work/templates/common/.github/workflows/pr-labeler.yaml" "$dir/contents/.github/workflows/pr-labeler.yaml"
  cat >> "$yaml" << YAML
---
apiVersion: gh-infra/v1
kind: File
metadata:
  name: $name
  owner: babarot
spec:
  files:
    - path: .github/release.yml
      source: ./templates/common/.github/release.yml
    - path: .github/workflows/pr-labeler.yaml
      source: ./templates/common/.github/workflows/pr-labeler.yaml
  commit_message: "Sync shared files from github-config"
  via: pull_request
  pr_title: "Sync shared files from github-config"
YAML
}

repo afx "An alternative, fast package manager for plugins and executable commands" "https://babarot.me/afx/" "cli,package-manager,go" 87
repo claude-recall "Recall any past Claude Code session: ask Claude to look into it, or find it yourself and go back to it." "" "" 24
repo emoji-cli ":scream: Emoji completion on the command line" "" ""
repo enhancd ":rocket: A next-generation cd command with your interactive filter" "" "bash,zsh,shell,fuzzy-search,fzf,cd,cli" 231
repo gh-infra "Declarative GitHub infrastructure management via YAML" "https://babarot.github.io/gh-infra" "cli,go,github,gh-extension" 277
repo gh-tidy "Keep one owner's local clones up to date and clear of finished work, ready to hand to a coding agent" "" ""
repo gist ":octocat: A simple gist editor for CLI" "https://babarot.me/gist" "gist,go,cli,gist-editor" 112
repo gomi "🗑️ Your UNIX rm command with a safety net!" "https://gomi.dev" "trash,go,cli,rm,trash-cli,trashcan"
repo httpstat "It's like curl -v, with graphs and colors" "" "bash,zplug,httpstat"
repo syno "Find the Synology NAS on your network and check on it from the terminal" "" "cli,dsm,go,monitoring,nas,synology" 19
wiki=true repo tmux-git-prompt "Git status on Tmux status line" "" "tmux,tmux-plugin"
repo zsh-mini-prompt "A small zsh prompt with smart defaults, configured through zstyle." "" "zsh-theme,zsh-prompt,prompt,zsh"

cd "$work"
export GH_INFRA_OUTPUT="${GH_INFRA_OUTPUT:-}"
export PS1='$ '
