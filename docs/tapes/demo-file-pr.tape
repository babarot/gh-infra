# gh-infra demo: File with many files → PR-based delivery
#
# Shows: File manifest with 7 files → plan fetches each file →
#        apply creates PR → gh pr view shows the result
#
# Run with: make demos
# Requirements: Docker

Output demo-file-pr.gif
Output demo-file-pr.mp4

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
Type "source /data/setup-file-pr.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Show the File manifest (many files, via: pull_request)
Sleep 700ms
Type "cat /tmp/demo/files.yaml"
Sleep 350ms
Enter
Sleep 5s

# 2. Apply — shows plan fetching 7 files, then file diffs
Type "gh infra apply /tmp/demo/"
Sleep 350ms
Enter
Sleep 6s

# 3. Open diff viewer to browse the file changes
Type "d"
Sleep 1.5s

# 4. Browse a few files
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "j"
Sleep 1s

# 5. Exit diff viewer
Escape
Sleep 1.5s

# 6. Confirm — apply creates PR
Type "y"
Enter
Sleep 6s

# 7. View the created PR (human-readable)
Type "gh pr view"
Sleep 350ms
Enter
Sleep 4s

# End
Sleep 2s
