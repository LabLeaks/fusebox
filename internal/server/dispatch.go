package server

// serverCommands lists commands that always dispatch as server (JSON) commands.
var serverCommands = map[string]bool{
	"list": true, "create": true, "create-team": true,
	"create-resume": true, "create-team-resume": true, "stop": true,
	"dirs": true, "subdirs": true,
	"preview": true, "activity": true, "install-hooks": true,
	"fix-mouse": true, "hook": true,
	"teams": true, "teams-toggle": true, "panes": true, "pane-preview": true,
	"up": true, "down": true, "sandbox-status": true, "update": true,
}

// Dispatch handles server subcommands. Returns true if the command was handled.
func Dispatch(cmd string, args []string) bool {
	if !serverCommands[cmd] {
		return false
	}
	switch cmd {
	case "list":
		CmdList()
	case "create":
		if len(args) < 2 {
			ExitError("usage: fusebox create <name> <dir>")
		}
		CmdCreate(args[0], args[1])
	case "stop":
		if len(args) < 1 {
			ExitError("usage: fusebox stop <name>")
		}
		CmdStop(args[0])
	case "dirs":
		CmdDirs()
	case "subdirs":
		if len(args) < 1 {
			ExitError("usage: fusebox subdirs <path>")
		}
		CmdSubdirs(args[0])
	case "preview":
		if len(args) < 1 {
			ExitError("usage: fusebox preview <name> [lines]")
		}
		lines := "30"
		if len(args) > 1 {
			lines = args[1]
		}
		CmdPreview(args[0], lines)
	case "activity":
		CmdActivity()
	case "install-hooks":
		CmdInstallHooks()
	case "fix-mouse":
		CmdFixMouse()
	case "hook":
		CmdHook()
	case "teams":
		CmdTeams()
	case "teams-toggle":
		if len(args) < 1 {
			ExitError("usage: fusebox teams-toggle <on|off>")
		}
		CmdTeamsToggle(args[0])
	case "panes":
		if len(args) < 1 {
			ExitError("usage: fusebox panes <session>")
		}
		CmdPanes(args[0])
	case "pane-preview":
		if len(args) < 1 {
			ExitError("usage: fusebox pane-preview <session> <pane> [lines]")
		}
		pane := "0"
		if len(args) > 1 {
			pane = args[1]
		}
		lines := "30"
		if len(args) > 2 {
			lines = args[2]
		}
		CmdPanePreview(args[0], pane, lines)
	case "create-team":
		if len(args) < 2 {
			ExitError("usage: fusebox create-team <name> <dir>")
		}
		CmdCreateTeam(args[0], args[1])
	case "create-resume":
		if len(args) < 2 {
			ExitError("usage: fusebox create-resume <name> <dir>")
		}
		CmdCreateResume(args[0], args[1])
	case "create-team-resume":
		if len(args) < 2 {
			ExitError("usage: fusebox create-team-resume <name> <dir>")
		}
		CmdCreateTeamResume(args[0], args[1])
	case "up":
		CmdUp()
	case "down":
		CmdDown()
	case "sandbox-status":
		CmdSandboxStatus()
	case "update":
		CmdUpdate()
	}
	return true
}
