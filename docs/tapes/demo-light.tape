# gh-infra demo (light theme variant of demo.tape)
#
# Run with: mise run demo
# Requirements: Docker

Output demo-light.gif
Output demo-light.mp4

# Light theme (Tokyo Night Day inspired)
Set Theme { "name": "gh-infra-light", "black": "#0f0f14", "red": "#f52a65", "green": "#587539", "yellow": "#8c6c3e", "blue": "#2e7de9", "magenta": "#9854f1", "cyan": "#007197", "white": "#6172b0", "brightBlack": "#a1a6c5", "brightRed": "#f52a65", "brightGreen": "#587539", "brightYellow": "#8c6c3e", "brightBlue": "#2e7de9", "brightMagenta": "#9854f1", "brightCyan": "#007197", "brightWhite": "#3760bf", "background": "#e1e2e7", "foreground": "#3760bf", "selection": "#b6bfe2", "cursor": "#3760bf" }

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
