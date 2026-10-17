# gh-infra demo: interactive diff viewer
#
# Shows: FileSet apply with 3 files × 2 repos →
#        diff viewer navigation, scrolling, Tab cycle (apply↔skip)
#
# Run with: make demos
# Requirements: Docker

Output demo-diffviewer.gif
Output demo-diffviewer.mp4

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
Type "source /data/setup-diffviewer.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Apply — shows plan with file diffs
Sleep 700ms
Type "gh infra apply /tmp/demo/"
Sleep 350ms
Enter
Sleep 6s

# 2. Open diff viewer
Type "d"
Sleep 1.5s

# 3. CODEOWNERS diff — multiple ownership lines changed
Sleep 1.2s

# 4. Move to ci.yml
Type "j"
Sleep 1.2s

# 5. Scroll ci.yml diff down and back
Type "d"
Sleep 800ms
Type "u"
Sleep 800ms

# 6. Move to dependabot.yml (new file — all green)
Type "j"
Sleep 1.2s

# 7. Tab cycle demo: skip → see it dim
Tab
Sleep 1.2s

# 8. Tab again: back to apply
Tab
Sleep 1.2s

# 9. Move to gomi CODEOWNERS
Type "j"
Sleep 800ms

# 10. Move to gomi ci.yml
Type "j"
Sleep 800ms

# 11. Skip gomi ci.yml
Tab
Sleep 1.2s

# 12. Move to gomi dependabot.yml — skip this too
Type "j"
Sleep 800ms
Tab
Sleep 1.2s

# 13. Exit diff viewer — shows skipped summary
Escape
Sleep 1.5s

# 14. Confirm apply (single keypress, no Enter needed)
Type "y"
Sleep 5s

# End
Sleep 2s
