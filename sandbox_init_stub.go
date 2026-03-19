//go:build !linux

package main

import (
	"fmt"
	"os"
)

func runSandboxInit() {
	fmt.Fprintln(os.Stderr, "sandbox init is only supported on Linux")
	os.Exit(1)
}
