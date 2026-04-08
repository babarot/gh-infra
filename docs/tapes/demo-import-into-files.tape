# gh-infra demo: import --into with file changes (write/patch/skip cycle)
#
# Shows: import --into detects file drift on GitHub (5 files) →
#        diff viewer with write/patch/skip Tab cycling →
#        confirm and write changes back to local YAML
#
# Run with: make demos
# Requirements: Docker

Output demo-import-into-files.gif
Output demo-import-into-files.mp4

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
Type "source /data/setup-import-into-files.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Run import --into to detect file drift
Sleep 700ms
Type "gh infra import babarot/my-project --into=/tmp/demo/"
Sleep 350ms
Enter
Sleep 6s

# 2. Open diff viewer
Type "d"
Sleep 1.5s

# 3. CODEOWNERS — Tab full cycle: write → patch → skip → write
Sleep 1s
Tab
Sleep 1s
Tab
Sleep 1s
Tab
Sleep 1s

# 4. Move to ci.yml — set to "patch"
Type "j"
Sleep 1s
Tab
Sleep 1.2s

# 5. Move to dependabot.yml — set to "skip" (write → patch → skip)
Type "j"
Sleep 1s
Tab
Sleep 800ms
Tab
Sleep 1.2s

# 6. Move to .golangci.yml — leave as "write"
Type "j"
Sleep 1s

# 7. Move to PR template — leave as "write"
Type "j"
Sleep 1s

# 8. Exit diff viewer — shows non-default actions summary
Escape
Sleep 1.5s

# 9. Confirm
Type "y"
Enter
Sleep 3s

# 10. Show the updated YAML
Type "cat /tmp/demo/my-project.yaml"
Sleep 350ms
Enter
Sleep 4s

# End
Sleep 2s
