//go:build linux

package main

import "github.com/lableaks/fusebox/internal/sandbox"

func runSandboxInit() {
	sandbox.RunInit()
}
