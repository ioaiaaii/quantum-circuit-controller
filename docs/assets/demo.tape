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
Set FontFamily "Menlo"
Set Theme "Github"
Set Framerate 10
Set TypingSpeed 35ms

Hide
Type "clear" Enter
Show

Type "kubectl get qpu"
Enter
Wait@30s
Sleep 3s
Type "clear" Enter

Type "qcc get qpus"
Enter
Wait@30s
Sleep 3s
Type "clear" Enter

Type "qcc get qpu ibm-kingston"
Enter
Wait@30s
Sleep 4s
Type "clear" Enter

Type "qcc draw examples/thesis/algorithms/shor.py"
Enter
Wait@75s
Sleep 4s
Type "clear" Enter

Type "qcc run examples/thesis/algorithms/shor.py --backend fake-brisbane --algorithm shor --experiment demo-gif --version v1 --timeout 2m"
Enter
Wait@135s
Sleep 4s
Type "clear" Enter

Type "qcc run examples/thesis/algorithms/shor.py --backend ibm-kingston --algorithm shor --experiment demo-gif --version v1 --detach --timeout 2m"
Enter
Wait@135s
Sleep 4s
Type "clear" Enter

Type "qcc get circuits --experiment demo-gif"
Enter
Wait@30s
Sleep 5s

Type "qcc get circuits shor-2vv42"
Enter
Wait@30s
Sleep 5s
