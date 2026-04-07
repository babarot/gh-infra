# gh-infra demo: import → edit in vim → apply workflow
#
# Run with: make demos
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

# 1. Import current repo settings as YAML
Sleep 700ms
Type "gh infra import babarot/my-project > /tmp/demo/my-project.yaml"
Sleep 350ms
Enter
Sleep 3s

# 2. Open in vim
Type "vim /tmp/demo/my-project.yaml"
Sleep 350ms
Enter
Sleep 1s

# 3. Search for description
Type "/My project"
Enter
Sleep 700ms

# 4. Replace "My project" with "My awesome project"
Type "cgn"
Sleep 200ms
Type "My awesome project"
Escape
Sleep 1s

# 5. Search for allow_squash_merge: false
Type "/false"
Enter
Sleep 700ms

# 6. Replace false with true
Type "cgn"
Sleep 200ms
Type "true"
Escape
Sleep 1s

# 7. Save and quit
Type ":wq"
Enter
Sleep 700ms

# 8. Apply (shows plan → confirm → apply)
Type "gh infra apply /tmp/demo/"
Sleep 350ms
Enter
Sleep 6s

# 9. Confirm
Type "y"
Enter
Sleep 6s

# End
Sleep 2s
