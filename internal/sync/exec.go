package sync

import "os/exec"

// newExecCommand wraps exec.Command. Tests can replace this.
var newExecCommand = exec.Command
