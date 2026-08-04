Require qcc
Require kubectl

Output docs/assets/figures/qcc-demo.gif

Set Shell zsh

# 1728x1440 at FontSize 26 gives a 108x45 terminal, displayed at 864x720.
# Keep Width/FontSize in proportion: halving FontSize alone doubles the
# column count and makes the text unreadable at display size.
Set Width 1728
Set Height 1440
Set FontSize 26
Set Padding 20

# Pinned so file size is predictable across machines rather than left to
# whatever VHS picks. Menlo is pinned because JetBrains Mono, which VHS
# prefers, is not installed here.
Set Framerate 10
Set FontFamily "Menlo"

Set Theme "Github"
Set TypingSpeed 35ms

Hide
Type "clear" Enter
Show

Type "kubectl get qpu"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc get qpus"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc get qpu ibm-kingston"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc draw examples/thesis/algorithms/shor.py"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc run examples/thesis/algorithms/shor.py --backend fake-brisbane --algorithm shor --experiment demo --version v1"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc run examples/thesis/algorithms/shor.py --backend ibm-kingston --algorithm shor --experiment demo --version v1 --detach"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc get circuits --experiment demo"
Sleep 500ms
Enter
Sleep 4s
Type "clear" Enter

Type "qcc get circuit shor-2vv42"
Sleep 500ms
Enter
Sleep 4s
