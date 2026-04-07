# gh-infra demo: RepositorySet defaults change → plan shows diffs across all repos
#
# Shows: current YAML with required_reviews: 1 → edit to 2 → plan shows all 4 repos affected
#
# Run with: make demos
# Requirements: Docker

Output demo-repositoryset-plan.gif
Output demo-repositoryset-plan.mp4

Set Theme { "name": "gh-infra-dark", "black": "#16161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#565f89", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "background": "#1a1b26", "foreground": "#a9b1d6", "selection": "#283457", "cursor": "#c0caf5" }

Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 1500
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# --- Hidden setup ---
Hide
Type "source /data/setup-repositoryset-plan.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Show the current RepositorySet YAML
Sleep 1s
Type "cat /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 5s

# 2. Edit: change required_reviews from 1 to 2 (one line change in defaults)
Type "sed -i 's/required_reviews: 1/required_reviews: 2/' /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 1.5s

# 3. Plan: all 4 repos show a diff for required_reviews
Type "gh infra plan /tmp/demo/"
Sleep 500ms
Enter
Sleep 8s

# End
Sleep 3s
