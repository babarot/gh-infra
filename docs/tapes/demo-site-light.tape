# gh-infra demo for the docs site (light): the same run as demo-light.tape, in the
# site's neutral colors. README keeps demo-light.gif in the original colors.
#
# Keep the steps below the same as demo.tape when either changes.
#
# Run with: mise run demo demo-site-light
# Requirements: Docker

Output demo-site-light.gif
Output demo-site-light.mp4

# Neutral light theme matching the docs site: grays, with color only for
# additions (green), changes (yellow) and deletions (red).
Set Theme { "name": "gh-infra-site-light", "black": "#171717", "red": "#cf222e", "green": "#1a7f37", "yellow": "#9a6700", "blue": "#404040", "magenta": "#383838", "cyan": "#525252", "white": "#525252", "brightBlack": "#737373", "brightRed": "#cf222e", "brightGreen": "#1a7f37", "brightYellow": "#9a6700", "brightBlue": "#262626", "brightMagenta": "#262626", "brightCyan": "#383838", "brightWhite": "#171717", "background": "#fafafa", "foreground": "#383838", "selection": "#e0e0e0", "cursor": "#171717" }

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
