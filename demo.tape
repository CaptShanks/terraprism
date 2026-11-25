# Terra-Prism Demo — Full Feature Showcase
# Run with: vhs demo.tape

Output assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 700
Set Padding 10
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

# ─── SCENE 1: Launch with pipe mode ───────────────────────────────────────────

Type "cat testdata/sample-plan.txt | terraprism"
Enter
Sleep 3s

# ─── SCENE 2: Vim Navigation ─────────────────────────────────────────────────

# Navigate down through resources
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "j"
Sleep 400ms

# Jump to last resource
Type "G"
Sleep 800ms

# Jump to first resource
Type "g"
Sleep 100ms
Type "g"
Sleep 800ms

# ─── SCENE 3: Expand/Collapse Resources ──────────────────────────────────────

# Expand first resource (aws_instance.web - create)
Type "l"
Sleep 1.5s

# Scroll down inside the expanded resource
Type "d"
Sleep 1s

# Collapse it back
Type "h"
Sleep 800ms

# Move to the destroy resource (aws_s3_bucket.logs)
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

# Expand it
Type "l"
Sleep 1.5s

# Collapse
Type "h"
Sleep 600ms

# ─── SCENE 4: Expand/Collapse All ────────────────────────────────────────────

# Expand all resources
Type "e"
Sleep 1.5s

# Collapse all
Type "c"
Sleep 1s

# ─── SCENE 5: Filter by Action ───────────────────────────────────────────────

# Open filter picker
Type "f"
Sleep 1s

# Move to "destroy" and select it
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type " "
Sleep 500ms

# Apply filter
Enter
Sleep 1.5s

# Clear filter (Esc from main view)
Escape
Sleep 1s

# ─── SCENE 6: Sort ───────────────────────────────────────────────────────────

# Open sort picker
Type "s"
Sleep 800ms

# Select sort by action
Type "j"
Sleep 300ms
Enter
Sleep 1.5s

# ─── SCENE 7: Search ─────────────────────────────────────────────────────────

# Start search
Type "/"
Sleep 500ms

# Type search query
Type "security_group"
Sleep 800ms
Enter
Sleep 1.5s

# Next match
Type "n"
Sleep 800ms

# Clear search
Escape
Sleep 800ms

# ─── SCENE 8: Expand Sub-blocks + Context Control ────────────────────────────

# Navigate to helm_release (has heredoc diff)
Type "G"
Sleep 400ms
Type "k"
Sleep 300ms

# Expand the resource
Type "l"
Sleep 1.5s

# Navigate into sub-blocks
Type "j"
Sleep 300ms

# Expand a foldable sub-block
Type "l"
Sleep 1.5s

# Increase diff context
Type "+"
Sleep 800ms
Type "+"
Sleep 800ms

# Decrease context
Type "-"
Sleep 800ms

# Collapse back
Type "h"
Sleep 600ms
Type "h"
Sleep 600ms

# ─── SCENE 9: Half-page scroll + line scroll ─────────────────────────────────

# Expand all for scrolling demo
Type "e"
Sleep 1s

# Half page down
Type "d"
Sleep 600ms
Type "d"
Sleep 600ms

# Half page up
Type "u"
Sleep 600ms

# Line scroll
Ctrl+E
Sleep 400ms
Ctrl+E
Sleep 400ms
Ctrl+Y
Sleep 400ms

# Collapse all to reset
Type "c"
Sleep 800ms

# ─── END ──────────────────────────────────────────────────────────────────────

Type "q"
Sleep 1s
