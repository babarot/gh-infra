#!/usr/bin/env bash
# Setup for the intro demo (demo.tape, demo-light.tape): a github-config
# style directory with sixteen repositories, each managing its labels. Seven
# of them drifted on GitHub (a label missing, recolored or added by hand, a
# description, homepage or topics edited in the UI, the wiki turned on), so plan has
# a mix of changes to show while the rest are up to date.
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
mkdir -p "$work"

# The labels every repository declares, as in babarot/github-config:
# name|color|description
labels=(
  "changelog/skip|ededed|Used in case of no need to update changelog"
  "kind/breaking-change|425df5|Denotes a PR that introduces potentially breaking changes"
  "kind/bug|425df5|A bug; unintended behavior"
  "kind/cleanup|425df5|Categorizes issue or PR as related to cleaning up code"
  "kind/deprecation|425df5|Related to a feature or part of code being deprecated"
  "kind/feature|425df5|A feature request; new or enhanced behavior"
  "kind/improvement|425df5|An improvement to existing functionality"
  "tagpr|ededed|Used by tagpr for managing the release"
)

# repo <name> <description> <homepage> <topics, comma separated>
# writes <name>.yaml and the mock's state for it. GitHub matches the
# manifest unless these are set for the call:
#   gh_description  the description on GitHub
#   gh_homepage     the homepage on GitHub
#   gh_topics       the topics on GitHub
#   wiki=true       the wiki is on
#   missing         labels GitHub does not have, space separated
#   recolor         <label>=<color>: GitHub has another color for the label
#   extra           labels only GitHub has, name|color|description per line
repo() {
  local name="$1" description="$2" homepage="$3" topics="$4"
  local dir="$MOCK_DIR/babarot/$name" yaml="$work/$name.yaml"
  local t l lname lcolor ldesc
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
    if [[ -n "$homepage" ]]; then echo "  homepage: $homepage"; fi
    echo "  visibility: public"
    if [[ -n "$topics" ]]; then
      echo "  topics:"
      for t in ${topics//,/ }; do echo "  - $t"; done
    fi
    echo "  labels:"
    for l in "${labels[@]}"; do
      IFS='|' read -r lname lcolor ldesc <<< "$l"
      echo "  - name: $lname"
      echo "    description: \"$ldesc\""
      echo "    color: $lcolor"
    done
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

  local gh_topics="${gh_topics-$topics}" json_topics=""
  for t in ${gh_topics//,/ }; do
    json_topics+="${json_topics:+,}{\"name\":\"$t\"}"
  done
  cat > "$dir/view.json" << JSON
{
  "description": "${gh_description-$description}",
  "homepageUrl": "${gh_homepage-$homepage}",
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

  local json_labels=""
  while IFS='|' read -r lname lcolor ldesc; do
    if [[ -z "$lname" || " ${missing:-} " == *" $lname "* ]]; then continue; fi
    if [[ "${recolor:-}" == "$lname="* ]]; then lcolor="${recolor#*=}"; fi
    json_labels+="${json_labels:+,}{\"name\":\"$lname\",\"color\":\"$lcolor\",\"description\":\"$ldesc\"}"
  done < <(printf '%s\n' "${labels[@]}" "${extra:-}")
  echo "[$json_labels]" > "$dir/labels.json"
}

gh_homepage="https://github.com/babarot/afx" missing="kind/improvement" \
  repo afx "An alternative, fast package manager for plugins and executable commands" "https://babarot.me/afx/" "cli,package-manager,go"
repo claude-recall "Recall any past Claude Code session: ask Claude to look into it, or find it yourself and go back to it." "" ""
repo dotfiles "A repository that gathered files starting with dot" "" "dotfiles,vim,zsh,tmux"
repo emoji-cli ":scream: Emoji completion on the command line" "" ""
extra=$'help wanted|008672|Extra attention is needed\nquestion|d876e3|Further information is requested' \
  repo enhancd ":rocket: A next-generation cd command with your interactive filter" "" "bash,zsh,shell,fuzzy-search,fzf,cd,cli"
recolor="kind/feature=a2eeef" gh_topics="cli,go,github" \
  repo gh-infra "Declarative GitHub infrastructure management via YAML" "https://babarot.github.io/gh-infra" "cli,go,github,gh-extension"
repo gh-tidy "Keep one owner's local clones up to date and clear of finished work, ready to hand to a coding agent" "" ""
wiki=true gh_description=":octocat: Gist editor" \
  repo gist ":octocat: A simple gist editor for CLI" "https://babarot.me/gist" "gist,go,cli,gist-editor"
repo github-labeler "Declarative way to configure GitHub labels" "" "github,labels,github-actions"
missing="kind/cleanup kind/deprecation" \
  repo gomi "🗑️ Your UNIX rm command with a safety net!" "https://gomi.dev" "trash,go,cli,rm,trash-cli,trashcan"
repo homebrew-tap "Homebrew formulae for various tools" "" ""
recolor="changelog/skip=cccccc" \
  repo httpstat "It's like curl -v, with graphs and colors" "" "bash,zplug,httpstat"
extra="enhancement|a2eeef|New feature or request" missing="kind/feature" \
  repo syno "Find the Synology NAS on your network and check on it from the terminal" "" "cli,dsm,go,monitoring,nas,synology"
repo tmux-git-prompt "Git status on Tmux status line" "" "tmux,tmux-plugin"
repo zsh-mini-prompt "A small zsh prompt with smart defaults, configured through zstyle." "" "zsh-theme,zsh-prompt,prompt,zsh"
repo zsh-vimode-visual "Implement the vim-like visual mode to vi-mode of zsh" "" "zsh,vim"

cd "$work"
export GH_INFRA_OUTPUT="${GH_INFRA_OUTPUT:-}"
export PS1='$ '
