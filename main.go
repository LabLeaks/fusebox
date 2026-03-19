package main

import (
	"fmt"
	"os"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/server"
	"github.com/lableaks/fusebox/internal/ssh"
	"github.com/lableaks/fusebox/internal/tui"
)

// isLocalHost checks if the configured host matches the current machine.
func isLocalHost(host string) bool {
	hostname, err := os.Hostname()
	if err != nil {
		return false
	}
	return hostname == host
}

func main() {
	// Server subcommands dispatch first (used via SSH from TUI, must always work)
	if len(os.Args) > 1 && server.Dispatch(os.Args[1], os.Args[2:]) {
		return
	}

	// Init wizard
	if len(os.Args) > 1 && os.Args[1] == "init" {
		hostArg := ""
		if len(os.Args) > 2 {
			hostArg = os.Args[2]
		}
		model := tui.NewInit(hostArg)
		p := tea.NewProgram(model)
		result, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if initModel, ok := result.(tui.InitModel); ok && initModel.Launch() {
			// Re-exec as bare "work" to launch dashboard
			exe, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding executable: %v\n", err)
				os.Exit(1)
			}
			syscall.Exec(exe, []string{"work"}, os.Environ())
		}
		return
	}

	// Any other subcommand → local interactive CLI
	if len(os.Args) > 1 {
		server.CmdLocal(os.Args[1:])
		return
	}

	// Bare "work" — config determines mode
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Server.Host == "" {
		server.CmdLocal(nil)
		return
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// If we're on the server, use local execution instead of SSH
	var model tui.Model
	if isLocalHost(cfg.Server.Host) {
		runner := ssh.NewLocalRunner(cfg.ResolveServerPath())
		model = tui.NewWithRunner(cfg, runner)
	} else {
		model = tui.New(cfg)
	}
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
