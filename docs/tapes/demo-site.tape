# gh-infra demo for the docs site (dark): the same run as demo.tape, in the
# site's neutral colors. README keeps demo.gif in the original colors.
#
# Keep the steps below the same as demo.tape when either changes.
#
# Run with: mise run demo demo-site
# Requirements: Docker

Output demo-site.gif
Output demo-site.mp4

# Neutral dark theme matching the docs site: grays, with color only for
# additions (green), changes (yellow) and deletions (red).
Set Theme { "name": "gh-infra-site-dark", "black": "#141414", "red": "#f85149", "green": "#3fb950", "yellow": "#d29922", "blue": "#b8b8b8", "magenta": "#c4c4c4", "cyan": "#a1a1a1", "white": "#a1a1a1", "brightBlack": "#808080", "brightRed": "#f85149", "brightGreen": "#3fb950", "brightYellow": "#d29922", "brightBlue": "#d4d4d4", "brightMagenta": "#d4d4d4", "brightCyan": "#c4c4c4", "brightWhite": "#ededed", "background": "#141414", "foreground": "#cccccc", "selection": "#2e2e2e", "cursor": "#ededed" }

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
