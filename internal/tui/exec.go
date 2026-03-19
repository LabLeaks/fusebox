package tui

import "os/exec"

// newExecCommand wraps exec.Command for use in init_cmds.
// Tests can replace this via the test file.
var newExecCommand = exec.Command
