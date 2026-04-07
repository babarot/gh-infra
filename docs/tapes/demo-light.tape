# gh-infra demo (light theme variant)
#
# Run with: make demos
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

# 1. Import current repo settings as YAML (two projects)
Sleep 1s
Type "gh infra import babarot/my-project babarot/my-service > /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 4s

# 2. Show the imported YAML
Type "cat /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 3s

# 3. Edit the YAML with sed (change description & enable squash merge for my-project only)
Type "sed -i 's/My project/My awesome project/' /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 1s

Type "sed -i 's/allow_squash_merge: false/allow_squash_merge: true/' /tmp/demo/repos.yaml"
Sleep 500ms
Enter
Sleep 1s

# 4. Plan the changes (only my-project shows a diff)
Type "gh infra plan /tmp/demo/"
Sleep 500ms
Enter
Sleep 6s

# End
Sleep 3s
