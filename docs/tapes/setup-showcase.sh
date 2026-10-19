#!/usr/bin/env bash
# Setup for the showcase demo (showcase.tape): forty of babarot's public
# repositories in a github-config style directory, with a bit of every kind
# of change in one apply: labels that drifted, a shared template synced by
# pull request, Actions variables, branch protection, a description edited
# on GitHub, and a repository to create.
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
export MOCK_DELAY=0.3
work=/tmp/demo/github-config
mkdir -p "$work/templates/common/.github"

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
    - title: Bug Fixes
      labels:
        - kind/bug
    - title: Others
      labels:
        - "*"
YAML

# name|color|description
labels=(
  "changelog/skip|ededed|Used in case of no need to update changelog"
  "kind/bug|425df5|A bug; unintended behavior"
  "kind/feature|425df5|A feature request; new or enhanced behavior"
  "kind/improvement|425df5|An improvement to existing functionality"
  "tagpr|ededed|Used by tagpr for managing the release"
)

# repo <name> <description>
# writes <name>.yaml and the mock's state for it. GitHub matches the
# manifest unless these are set for the call:
#   gh_description  the description on GitHub
#   missing         labels GitHub does not have, space separated
#   recolor         <label>=<color>: GitHub has another color for the label
#   extra           labels only GitHub has, name|color|description per line
#   protect=1       the manifest protects main; GitHub does not yet
#   vars            Actions variables the manifest adds, NAME=value space separated
#   sync=<number>   the repository syncs templates/ by pull request, and
#                   GitHub has the old release.yml; the number is the PR's
#   absent=1        the repository does not exist yet
repo() {
  local name="$1" description="$2"
  local dir="$MOCK_DIR/babarot/$name" yaml="$work/$name.yaml"
  local l lname lcolor ldesc v
  mkdir -p "$dir"

  {
    echo "apiVersion: gh-infra/v1"
    echo "kind: Repository"
    echo "metadata:"
    echo "  name: $name"
    echo "  owner: babarot"
    echo "reconcile:"
    echo "  labels: authoritative"
    echo "spec:"
    echo "  description: \"$description\""
    echo "  visibility: public"
    echo "  labels:"
    for l in "${labels[@]}"; do
      IFS='|' read -r lname lcolor ldesc <<< "$l"
      echo "  - name: $lname"
      echo "    description: \"$ldesc\""
      echo "    color: $lcolor"
    done
    if [[ -n "${vars:-}" ]]; then
      echo "  variables:"
      for v in $vars; do
        echo "  - name: ${v%%=*}"
        echo "    value: \"${v#*=}\""
      done
    fi
    if [[ -n "${protect:-}" ]]; then
      echo "  branch_protection:"
      echo "  - pattern: main"
      echo "    required_reviews: 1"
    fi
  } > "$yaml"

  if [[ -n "${absent:-}" ]]; then
    touch "$dir/absent"
    echo "[]" > "$dir/labels.json"
    return
  fi

  cat > "$dir/view.json" << JSON
{
  "description": "${gh_description-$description}",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": true,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": false,
  "defaultBranchRef": { "name": "main" }
}
JSON

  local json_labels=""
  while IFS='|' read -r lname lcolor ldesc; do
    if [[ -z "$lname" || " ${missing:-} " == *" $lname "* ]]; then continue; fi
    if [[ "${recolor:-}" == "$lname="* ]]; then lcolor="${recolor#*=}"; fi
    json_labels+="${json_labels:+,}{\"name\":\"$lname\",\"color\":\"$lcolor\",\"description\":\"$ldesc\"}"
  done < <(printf '%s\n' "${labels[@]}" "${extra:-}")
  echo "[$json_labels]" > "$dir/labels.json"

  if [[ -n "${sync:-}" ]]; then
    echo "$sync" > "$dir/pr-number"
    mkdir -p "$dir/contents/.github"
    sed 's/Bug Fixes/Bug fixes/' "$work/templates/common/.github/release.yml" > "$dir/contents/.github/release.yml"
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
  commit_message: "Sync shared files from github-config"
  via: pull_request
YAML
  fi
}

repo action-changed-objects "GitHub Action to get changed files compared with origin/master"
sync=87 repo afx "An alternative, fast package manager for plugins and executable commands"
repo babarot.github.io "babarot's personal website"
repo backup.nvim "Neovim plugin that automatically creates timestamped backups on save"
recolor="kind/bug=d73a4a" repo c-c-statusline "A Deno-powered status line for Claude Code CLI"
repo changed-objects "Get changed objects in Git commit"
repo changelog-empty-check-action "Check that CHANGELOG.md is updated in a pull request"
vars="NODE_VERSION=22" repo claude-linkify "A Claude Code mod that makes URLs and GitHub references clickable"
sync=24 repo claude-recall "Recall any past Claude Code session"
repo cursor-x.nvim "Neovim plugin to show the cursor line automatically on X-axis"
repo dotfiles "A repository that gathered files starting with dot"
repo emoji-cli ":scream: Emoji completion on the command line"
sync=231 extra=$'help wanted|008672|Extra attention is needed' \
  repo enhancd ":rocket: A next-generation cd command with your interactive filter"
repo enter "Show contextual info on every Enter press"
repo ftcolor.nvim "Automatic colorscheme switching based on filetype for Neovim"
sync=277 protect=1 repo gh-infra "Declarative GitHub infrastructure management via YAML"
absent=1 repo gh-infra-examples "Example manifests for gh-infra"
repo gh-mini "A small, local GitHub for a directory of Markdown"
repo gh-tidy "Keep one owner's local clones up to date and clear of finished work"
sync=112 gh_description=":octocat: Gist editor" repo gist ":octocat: A simple gist editor for CLI"
repo github-labeler "Declarative way to configure GitHub labels"
missing="kind/improvement" protect=1 repo gomi "🗑️ Your UNIX rm command with a safety net!"
repo homebrew-tap "Homebrew formulae for various tools"
missing="changelog/skip tagpr" repo httpstat "It's like curl -v, with graphs and colors"
repo iap_curl "A curl wrapper for making HTTP requests to IAP-protected apps"
vars="GO_VERSION=1.26 REGISTRY=ghcr.io" repo image-hosting-server "Lightweight Go image hosting server"
repo markdown-preview.nvim "Preview Markdown in Neovim with GitHub's official styling"
repo naminator "Bulk-rename w/ EXIF metadata"
repo nur-packages "Nix packages for babarot's tools, published by GoReleaser"
repo obsidian-image-uploader-to-api "Obsidian plugin that uploads images to any API endpoint"
repo oksskolten.com "Web page for Oksskolten, the AI-native RSS reader"
vars="NODE_VERSION=22" repo oksskolten "🏔️ The AI-native RSS reader"
repo rm.nvim "Safe file deletion plugin for Neovim with customizable commands"
repo scoop-bucket "Scoop bucket, see https://scoop.sh."
sync=19 protect=1 repo syno "Find the Synology NAS on your network and check on it"
repo tellme.tokyo ":tokyo_tower: @babarot's blog"
recolor="tagpr=cccccc" repo tmux-git-prompt "Git status on Tmux status line"
repo zenn.dev "Articles for zenn.dev"
repo zsh-mini-prompt "A small zsh prompt with smart defaults, configured through zstyle."
repo zsh-vimode-visual "Implement the vim-like visual mode to vi-mode of zsh"

cd "$work"
export GH_INFRA_OUTPUT="${GH_INFRA_OUTPUT:-}"
export PS1='$ '
