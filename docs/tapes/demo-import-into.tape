# gh-infra demo: import --into (reverse sync from GitHub to local YAML)
#
# Shows: import --into detects diffs between local YAML and GitHub state →
#        shows plan → confirms → YAML is updated in place
#
# Run with: make demos
# Requirements: Docker

Output demo-import-into.gif
Output demo-import-into.mp4

Set Theme { "name": "gh-infra-dark", "black": "#16161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#565f89", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "background": "#1a1b26", "foreground": "#a9b1d6", "selection": "#283457", "cursor": "#c0caf5" }

Set Shell "bash"
Set FontSize 22
Set Width 1200
Set Height 800
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# --- Hidden setup ---
Hide
Type "source /data/setup-import-into.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Show current local YAML (stale state)
Sleep 1s
Type "cat /tmp/demo/my-project.yaml"
Sleep 500ms
Enter
Sleep 4s

# 2. Run import --into to pull GitHub state back
Type "gh infra import babarot/my-project --into=/tmp/demo/"
Sleep 500ms
Enter
Sleep 8s

# 3. Confirm (huh prompt expects y then Enter)
Type "y"
Enter
Sleep 3s

# 4. Show the updated YAML (fields have been patched in place)
Type "cat /tmp/demo/my-project.yaml"
Sleep 500ms
Enter
Sleep 5s

# End
Sleep 3s
