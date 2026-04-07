# gh-infra demo: apply with multi-repo parallel spinners
#
# Shows: plan output → confirmation prompt → parallel apply with step-level progress
#
# Run with: make demos
# Requirements: Docker

Output demo-apply.gif
Output demo-apply.mp4

Set Theme { "name": "gh-infra-dark", "black": "#16161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#565f89", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "background": "#1a1b26", "foreground": "#a9b1d6", "selection": "#283457", "cursor": "#c0caf5" }

Set Shell "bash"
Set FontSize 20
Set Width 1400
Set Height 900
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# --- Hidden setup ---
Hide
Type "source /data/setup-apply.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Show the RepositorySet YAML (briefly)
Sleep 700ms
Type "cat /tmp/demo/repos.yaml"
Sleep 350ms
Enter
Sleep 3s

# 2. Apply — shows plan, then confirmation, then parallel spinners
Type "gh infra apply /tmp/demo/"
Sleep 350ms
Enter
Sleep 6s

# 3. Confirm with 'y' (huh prompt expects Enter after y)
Type "y"
Enter
Sleep 6s

# End
Sleep 2s
