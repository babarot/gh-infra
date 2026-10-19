# gh-infra showcase: one apply across forty repositories, with labels, a
# template synced by pull request, Actions variables, branch protection, a
# drifted description and a new repository. 16:9, about 16s, for use outside
# the docs.
#
# Run with: make demo TAPES=showcase (not copied to docs/public)

Output showcase.gif
Output showcase.mp4

Set Theme { "name": "gh-infra-dark", "black": "#16161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#565f89", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5", "background": "#1a1b26", "foreground": "#a9b1d6", "selection": "#283457", "cursor": "#c0caf5" }

Set Shell "bash"
Set FontSize 20
Set Width 1200
Set Height 676
Set Padding 20
Set WindowBar ""
Set TypingSpeed 50ms

# --- Hidden setup ---
Hide
Type "source /data/setup-showcase.sh"
Enter
Sleep 2s
Type "clear"
Enter
Sleep 500ms
Show

# --- Demo starts ---

# 1. Apply: fetch forty repositories, plan, confirm
Sleep 500ms
Type "gh infra apply ."
Sleep 300ms
Enter
Sleep 7s

# 2. Confirm (single keypress, no Enter needed)
Type "y"
Sleep 6s

# End
Sleep 2s
