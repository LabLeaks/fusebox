package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/server"
	"github.com/lableaks/fusebox/internal/ssh"
	syncpkg "github.com/lableaks/fusebox/internal/sync"
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
	// Sandbox init re-exec — MUST be first.
	// When the binary re-execs itself for container init, this catches it immediately.
	if len(os.Args) > 1 && os.Args[1] == "init" && os.Getenv("FUSEBOX_ROOTFS") != "" {
		runSandboxInit()
		return
	}

	// Fusebox sync subcommand
	if len(os.Args) > 1 && os.Args[1] == "sync" {
		handleSync(os.Args[2:])
		return
	}

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
			syscall.Exec(exe, []string{"fusebox"}, os.Environ())
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

// handleSync routes fusebox sync subcommands.
func handleSync(args []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	dataDir := cfg.ResolveSandboxDataDir()
	if dataDir == "" {
		dataDir = filepath.Join(home, ".fusebox")
	}

	mgr := syncpkg.NewManager(dataDir, cfg.SSHTarget())

	if len(args) == 0 {
		syncUsage()
		return
	}

	switch args[0] {
	case "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: fusebox sync add <local-path>")
			os.Exit(1)
		}
		if err := mgr.Add(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Syncing %s\n", args[1])

	case "rm", "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: fusebox sync rm <local-path>")
			os.Exit(1)
		}
		if err := mgr.Remove(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stopped syncing %s\n", args[1])

	case "ls", "list":
		sessions, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No active sync sessions.")
			return
		}
		for _, s := range sessions {
			fmt.Printf("%-30s %s <-> %s  [%s]\n", s.Name, s.Local, s.Remote, s.Status)
		}

	case "pause":
		if err := mgr.Pause(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Paused all sync sessions.")

	case "resume":
		if err := mgr.Resume(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Resumed all sync sessions.")

	default:
		syncUsage()
		os.Exit(1)
	}
}

func syncUsage() {
	fmt.Println(`Usage: fusebox sync <command>

Commands:
  add <path>    Start syncing a local folder
  rm <path>     Stop syncing a folder
  ls            List active sync sessions
  pause         Pause all sync sessions
  resume        Resume all sync sessions`)
}
