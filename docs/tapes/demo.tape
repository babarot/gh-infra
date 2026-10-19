# gh-infra demo: bring sixteen repositories back to their manifests
#
# Run with: mise run demo
# Requirements: Docker
#
# This tape uses a mock gh CLI (mock-gh) to simulate GitHub API responses.
# gh-infra runs for real — spinners, diffs, and colors are all genuine.

Output demo.gif
Output demo.mp4

# Dark theme (Tokyo Night inspired)
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
Type "source /data/setup.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. A github-config style directory: one manifest per repository
Sleep 700ms
Type "ls"
Sleep 350ms
Enter
Sleep 2s

# 2. Apply: fetch the sixteen repositories, show what drifted on GitHub
#    (labels, description, homepage, topics, wiki), confirm
Type "gh infra apply ."
Sleep 350ms
Enter
Sleep 9s

# 3. Confirm (single keypress, no Enter needed)
Type "y"
Sleep 7s

# End
Sleep 2s
