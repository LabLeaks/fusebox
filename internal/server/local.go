package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ANSI colors
const (
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cReset  = "\033[0m"
)

// CmdLocal dispatches local interactive CLI commands.
func CmdLocal(args []string) {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	var rest []string
	if len(args) > 1 {
		rest = args[1:]
	}

	switch cmd {
	case "", "ls":
		localLs()
	case "new":
		filter := ""
		if len(rest) > 0 {
			filter = rest[0]
		}
		localNew(filter)
	case "attach":
		target := ""
		if len(rest) > 0 {
			target = rest[0]
		}
		localAttach(target)
	case "kill":
		target := ""
		if len(rest) > 0 {
			target = rest[0]
		}
		localKill(target)
	case "peek":
		target := ""
		if len(rest) > 0 {
			target = rest[0]
		}
		localPeek(target)
	case "help", "-h", "--help":
		localHelp()
	default:
		// Treat bare arg as attach target
		localAttach(cmd)
	}
}

func localHelp() {
	fmt.Println("fusebox — Claude Code session manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  fusebox                  Dashboard (TUI, requires config)")
	fmt.Println("  fusebox init [user@host] Setup wizard (deploy, sync, config)")
	fmt.Println("  fusebox ls               List sessions")
	fmt.Println("  fusebox new [filter]     Create session (interactive)")
	fmt.Println("  fusebox attach [n|name]  Attach to session")
	fmt.Println("  fusebox kill [n|name]    Stop session")
	fmt.Println("  fusebox peek [n|name]    Preview session output")
	fmt.Println()
	fmt.Println("Sync:")
	fmt.Println("  fusebox sync add <path>  Start syncing a local folder")
	fmt.Println("  fusebox sync rm <path>   Stop syncing a folder")
	fmt.Println("  fusebox sync ls          List active sync sessions")
	fmt.Println("  fusebox sync pause       Pause all syncs")
	fmt.Println("  fusebox sync resume      Resume all syncs")
	fmt.Println()
	fmt.Println("Other:")
	fmt.Println("  fusebox version          Show version")
	fmt.Println("  fusebox update           Self-update to latest release")
	fmt.Println()
	fmt.Println("Server commands (JSON, called via SSH):")
	fmt.Println("  list, create, create-team, create-resume, create-team-resume,")
	fmt.Println("  stop, dirs, subdirs, preview, activity, teams, teams-toggle,")
	fmt.Println("  panes, pane-preview, install-hooks, fix-mouse, hook")
}

func localLs() {
	sessions, err := getSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println("No active sessions.")
		return
	}

	home, _ := os.UserHomeDir()
	homePrefix := home + "/"
	now := time.Now().Unix()
	activity := readActivityDir("/tmp/fusebox", 60)

	fmt.Println()
	for i, s := range sessions {
		shortDir := s.Dir
		if strings.HasPrefix(shortDir, homePrefix) {
			shortDir = strings.TrimPrefix(shortDir, homePrefix)
		}
		up := formatUptime(now - s.Created)
		status := sessionStatus(s.Name, activity)
		fmt.Printf("  %s%d)%s %-20s %s%-30s%s %s%5s%s  %s\n",
			cBold, i+1, cReset,
			s.Name,
			cDim, shortDir, cReset,
			cDim, up, cReset,
			status)
	}
	fmt.Println()
}

func formatUptime(diff int64) string {
	switch {
	case diff < 60:
		return fmt.Sprintf("%ds", diff)
	case diff < 3600:
		return fmt.Sprintf("%dm", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%dh%dm", diff/3600, (diff%3600)/60)
	default:
		return fmt.Sprintf("%dd%dh", diff/86400, (diff%86400)/3600)
	}
}

func sessionStatus(name string, activity map[string]json.RawMessage) string {
	if raw, ok := activity[name]; ok {
		var a struct {
			Tool string `json:"tool"`
		}
		if json.Unmarshal(raw, &a) == nil && a.Tool != "" {
			return cYellow + "working" + cReset
		}
	}
	return cGreen + "idle" + cReset
}

func localNew(filter string) {
	dirs, err := getDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(dirs) == 0 {
		fmt.Fprintf(os.Stderr, "error: no directories found\n")
		os.Exit(1)
	}

	home, _ := os.UserHomeDir()
	homePrefix := home + "/"

	if filter != "" {
		var filtered []string
		lf := strings.ToLower(filter)
		for _, d := range dirs {
			if strings.Contains(strings.ToLower(d), lf) {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "error: no directories match '%s'\n", filter)
			os.Exit(1)
		}
		if len(filtered) == 1 {
			dir := filtered[0]
			name := filepath.Base(dir)
			fmt.Printf("Creating session %s%s%s → %s\n", cBold, name, cReset, dir)
			if _, err := doCreate(name, dir, createOpts{}); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println()
			if promptYN("Attach?") {
				tmuxAttach(name)
			} else {
				fmt.Println("Session running in background.")
			}
			return
		}
		dirs = filtered
	}

	fmt.Println()
	for i, dir := range dirs {
		short := dir
		if strings.HasPrefix(short, homePrefix) {
			short = strings.TrimPrefix(short, homePrefix)
		}
		fmt.Printf("  %s%2d)%s %s\n", cBold, i+1, cReset, short)
	}
	fmt.Println()

	choice := prompt("Pick a directory (number): ")
	if choice == "" {
		return
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(dirs) {
		fmt.Fprintf(os.Stderr, "error: invalid choice: %s\n", choice)
		os.Exit(1)
	}

	dir := dirs[n-1]
	name := filepath.Base(dir)

	if tmuxHasSession(name) {
		fmt.Printf("Session %s%s%s already exists.\n", cBold, name, cReset)
		if promptYN("Attach?") {
			tmuxAttach(name)
		}
		return
	}

	fmt.Printf("Creating session %s%s%s...\n", cBold, name, cReset)
	if _, err := doCreate(name, dir, createOpts{}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	if promptYN("Attach?") {
		tmuxAttach(name)
	} else {
		fmt.Println("Session running in background.")
	}
}

func localAttach(target string) {
	sessions, err := getSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if target == "" {
		if len(sessions) == 0 {
			fmt.Println("No active sessions.")
			return
		}
		localLs()
		choice := prompt("Attach to (number): ")
		if choice == "" {
			return
		}
		target = choice
	}

	// If it's a number, resolve to session name
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(sessions) {
			fmt.Fprintf(os.Stderr, "error: no session #%d\n", n)
			os.Exit(1)
		}
		tmuxAttach(sessions[n-1].Name)
		return
	}

	tmuxAttach(target)
}

func localKill(target string) {
	sessions, err := getSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if target == "" {
		if len(sessions) == 0 {
			fmt.Println("No active sessions.")
			return
		}
		localLs()
		choice := prompt("Stop which session (number): ")
		if choice == "" {
			return
		}
		target = choice
	}

	// If it's a number, resolve to session name
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(sessions) {
			fmt.Fprintf(os.Stderr, "error: no session #%d\n", n)
			os.Exit(1)
		}
		name := sessions[n-1].Name
		fmt.Printf("Stopping %s%s%s...\n", cBold, name, cReset)
		if err := doStop(name); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("ok")
		return
	}

	fmt.Printf("Stopping %s%s%s...\n", cBold, target, cReset)
	if err := doStop(target); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func localPeek(target string) {
	sessions, err := getSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if target == "" {
		if len(sessions) == 0 {
			fmt.Println("No active sessions.")
			return
		}
		localLs()
		choice := prompt("Peek at (number): ")
		if choice == "" {
			return
		}
		target = choice
	}

	var name string
	if n, err := strconv.Atoi(target); err == nil {
		if n < 1 || n > len(sessions) {
			fmt.Fprintf(os.Stderr, "error: no session #%d\n", n)
			os.Exit(1)
		}
		name = sessions[n-1].Name
	} else {
		name = target
	}

	out, err := doPreview(name, "20")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

func prompt(msg string) string {
	fmt.Print(msg)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func promptYN(msg string) bool {
	resp := prompt(msg + " [Y/n] ")
	return resp == "" || strings.HasPrefix(strings.ToLower(resp), "y")
}

func tmuxAttach(name string) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: tmux not found\n")
		os.Exit(1)
	}
	syscall.Exec(tmux, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}
