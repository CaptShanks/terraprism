# Terra-Prism Demo
# Run with: vhs demo.tape

Output assets/demo.gif

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700
Set Theme "Catppuccin Mocha"

# Title
Type "# Terra-Prism - Interactive Terraform Plan Viewer"
Enter
Sleep 1s

# Show version
Type "terraprism version"
Enter
Sleep 2s

# Clear and show plan
Type "clear"
Enter
Sleep 500ms

Type "# Let's view a terraform plan"
Enter
Sleep 1s

Type "cat testdata/sample-plan.txt | terraprism"
Enter
Sleep 2s

# Navigate down
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms

# Expand resource
Type@100ms "l"
Sleep 1.5s

# Scroll down inside
Type "d"
Sleep 1s

# Collapse
Type "h"
Sleep 1s

# Jump to bottom
Type "G"
Sleep 1s

# Jump to top
Type "gg"
Sleep 1s

# Expand all
Type "e"
Sleep 1.5s

# Collapse all
Type "c"
Sleep 1s

# Search
Type "/"
Sleep 500ms
Type@50ms "aws_instance"
Sleep 1s
Enter
Sleep 1s

# Next match
Type "n"
Sleep 1s

# Clear search and quit
Escape
Sleep 500ms
Type "q"
Sleep 1s

# Show history
Type "# Now let's check history"
Enter
Sleep 1s

Type "terraprism history list"
Enter
Sleep 3s

# Interactive picker
Type "terraprism history view"
Enter
Sleep 2s

# Navigate
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms

# Search in picker
Type "/"
Sleep 500ms
Type@50ms "apply success"
Sleep 1s
Enter
Sleep 1s

# Cancel
Type "q"
Sleep 1s

Type "# That's Terra-Prism!"
Enter
Sleep 2s
