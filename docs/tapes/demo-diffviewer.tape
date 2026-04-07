# gh-infra demo: interactive diff viewer
#
# Shows: plan with file diffs → press 'd' to open diff viewer →
#        navigate files, toggle skip with Tab, scroll diff → confirm apply
#
# Run with: make demos
# Requirements: Docker

Output demo-diffviewer.gif
Output demo-diffviewer.mp4

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
Type "source /data/setup-diffviewer.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Apply — shows plan with file diffs, then confirmation prompt
Sleep 1s
Type "gh infra apply /tmp/demo/"
Sleep 500ms
Enter
Sleep 8s

# 2. Press 'd' to open the diff viewer
Type "d"
Sleep 2s

# 3. Navigate: move down to second file
Type "j"
Sleep 1.5s

# 4. Move to third file
Type "j"
Sleep 1.5s

# 5. Toggle this file to 'skip' with Tab
Tab
Sleep 1.5s

# 6. Move back up
Type "k"
Sleep 1.5s

# 7. Scroll the diff pane down
Type "d"
Sleep 1.5s

# 8. Scroll back up
Type "u"
Sleep 1.5s

# 9. Exit diff viewer
Escape
Sleep 2s

# 10. Confirm apply
Type "y"
Sleep 6s

# End
Sleep 3s
