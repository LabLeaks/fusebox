# fusebox

Remote Claude Code session manager. One binary — TUI dashboard on your machine, interactive CLI on the server, JSON over SSH.

```
┌───────────────────────────────────────────────┐
│  WORK · spotless-1 · 3 active sessions        │
├───────────────────────────────────────────────┤
│ Session        Directory      Up   Status     │
│▸ethics-mapper  lableaks/proj… 2h   Edit app.go│
│ 4d-instrument  lableaks/res…  45m  Bash: go t…│
│ conscience     lableaks/res…  10m  ○ idle     │
│                                               │
│ [n] new [enter] attach [d] stop [p] preview   │
├───────────────────────────────────────────────┤
│ Preview: ethics-mapper              ↻ 2s      │
│ I've updated the test file. Let me run        │
│ the tests now...                              │
│ $ go test ./internal/tui/ -v -count=1         │
│ --- PASS: TestDashboard_RendersSessions       │
│ PASS                                          │
└───────────────────────────────────────────────┘
```

## Prerequisites

- A Linux server with SSH access and tmux installed

## Setup

### Quick start (no Go needed)

```bash
curl -sSL https://raw.githubusercontent.com/lableaks/fusebox/master/install.sh | bash
work init user@your-server
```

The installer downloads the right binary for your OS/arch and puts it in `~/.local/bin`. Then `work init` walks you through setup interactively — it tests SSH, deploys the server binary, discovers directories, and writes your config.

The Mac release binary has Linux server binaries embedded, so `work init` just extracts and SCPs — no Go toolchain or cross-compilation needed.

### From source

```bash
make deploy SERVER=myserver.example.com SERVER_USER=deploy
```

This creates `~/.config/work-cli/config.yaml` automatically, installs `work` locally, and deploys to the server. After that, `make deploy` reads the config — no flags needed.

### Subsequent deploys

```bash
make deploy          # from source
work init user@host  # or re-run the wizard (reconfigures)
```

Both are safe to run while sessions are active. Make sure `~/.local/bin` is on your PATH.

## Usage

### TUI Dashboard (Mac)

```bash
work              # launches TUI when config has server.host
```

#### Keybindings

| Key | Action |
|-----|--------|
| `n` | Create a new session (drill-down directory browser) |
| `enter` | Attach to selected session / drill into directory |
| `d` | Stop session (with confirmation) |
| `p` | Toggle preview pane — see last 30 lines of session output |
| `t` | Open team detail (when selected session is a team lead) |
| `esc` | Close preview pane / cancel |
| `r` | Refresh session list |
| `q` | Quit |
| `ctrl+c` | Quit (or cancel current view) |

When the hostname matches `server.host` in your config, the TUI runs locally — no SSH to itself. This means you can run `work` directly on the server and get the full dashboard.

### Local CLI (server / phone SSH)

```bash
work                  # list sessions (no config)
work ls               # list sessions
work new [filter]     # create session (interactive dir picker)
work attach [n|name]  # attach to session by number or name
work kill [n|name]    # stop session by number or name
work peek [n|name]    # preview session output
work help             # show all commands
```

### Server Commands (JSON, called via SSH)

```bash
work list             # JSON array of sessions
work create <n> <dir> # create session
work create-team <n> <dir>  # create session with agent teams enabled
work stop <name>      # stop session
work dirs             # browsable directories (from roots.conf)
work subdirs <path>   # list subdirs of path with counts
work preview <n> [lines]
work activity         # tool activity for all sessions
work teams            # list active teams with members and tasks
work teams-toggle <on|off>  # enable/disable agent teams in settings.json
work panes <session>  # list tmux panes for a session
work pane-preview <session> <pane> [lines]  # capture a specific pane
work install-hooks    # install Claude Code PostToolUse hook
work fix-mouse        # enable mouse mode on all sessions
work hook             # PostToolUse hook handler (reads stdin)
```

### Live tool activity

The Status column shows what each Claude session is doing in real-time:

| Status | Meaning |
|--------|---------|
| `Edit app.go` | Editing a file |
| `Bash: go test ./...` | Running a shell command |
| `Read session.go` | Reading a file |
| `Grep: TODO` | Searching code |
| `Agent: subagent` | Spawned a subagent |
| `● running` | Active but no recent tool use |
| `○ idle` | No activity for 30s+ |

Activity updates every 5 seconds via a Claude Code [hook](https://docs.anthropic.com/en/docs/claude-code/hooks) that fires after each tool use. Run `make deploy` to install the hook automatically.

### Preview pane

Press `p` to open a live preview of the selected session's terminal output. It auto-refreshes every 2 seconds. Move your cursor to a different session and the preview follows. Press `p` or `esc` to close.

### Agent teams

Claude Code's experimental agent teams feature lets a lead session spawn teammate sessions that coordinate via shared task lists. Work-cli integrates with this — it doesn't orchestrate teams itself, but gives you visibility and control.

**Enabling:** Toggle teams globally with `work teams-toggle on`, or per-session in the create flow with `[t]`.

**Dashboard:** When a session is a team lead (detected by having multiple tmux panes), the Status column shows `Team: <name> X/Y` instead of tool activity.

**Team detail:** Press `[t]` on a team lead session to see:
- Teammate list with current activity
- Task board with completion status
- `[enter]` to attach to a specific teammate's pane
- `[p]` to preview a teammate's output
- `[esc]` to return to the dashboard

| Key (dashboard) | Action |
|-----|--------|
| `t` | Open team detail (on team lead sessions) |

| Key (team detail) | Action |
|-----|--------|
| `enter` | Attach to selected teammate's pane |
| `p` | Preview selected teammate's output |
| `up/down` | Navigate teammates |
| `esc` | Back to dashboard |

| Key (create) | Action |
|-----|--------|
| `t` | Toggle teams on/off for the new session |

### Attaching & detaching

`enter` hands you off to the remote tmux session via SSH. You're now in Claude Code. To get back to the dashboard, detach from tmux with `ctrl+b d`.

## Configuration

Config lives at `~/.config/work-cli/config.yaml`. Without it, `work` runs in local CLI mode.

```yaml
server:
  host: myserver.example.com   # any SSH-reachable hostname or IP
  user: deploy

claude:
  flags: "--dangerously-skip-permissions"

browse_roots:
  - ~/projects
  - ~/work
```

## Development

```bash
make test          # run all Go tests (client + server)
make test-server   # run server integration tests (needs tmux)
make build         # compile client binary (dev — no embedded server)
make build-server  # cross-compile for linux/arm64
make release       # build with embedded Linux binaries (for work init)
make deploy        # build everything, push to server, install hooks
make clean         # remove build artifacts
```

### Release builds

`make release` cross-compiles Linux arm64 and amd64 server binaries, then builds the Mac binary with them embedded via `go:embed`. The resulting binary can deploy itself to a server via `work init` without needing Go on the target machine.

Dev builds (`make build`, `go build`) work normally but `work init` will show a "use release build" error at the deploy step.

## Architecture

```
Mac (client)                    spotless-1 (server)
┌─────────┐   SSH commands      ┌──────────────┐
│  work    │ ──────────────────▸ │    work       │
│  (TUI)   │ ◂────────────────── │  (same bin)   │
└─────────┘   JSON / text       └──────┬───────┘
                                       │ tmux
                                       ▼
                                ┌──────────────┐
                                │ claude code   │
                                │ sessions      │
                                └──────┬───────┘
                                       │ PostToolUse hook
                                       ▼
                                  work hook
                                   writes JSON
                                       │
                                       ▼
                                /tmp/work-cli/*.json
                                       ▲
                                       │ reads
                                  work activity
```

One `work` binary runs everywhere. On the Mac (with config), it launches the TUI dashboard over SSH. On the server, if the hostname matches `server.host`, the TUI runs locally with no SSH. Otherwise it falls back to the interactive CLI. The TUI works identically in both modes — only the transport changes.

Tool activity flows through a separate path: Claude Code's PostToolUse hook fires `work hook`, which writes status to `/tmp/work-cli/<session>.json`. The dashboard polls `work activity` every 5 seconds to read those files.

`work` auto-detects the claude binary by checking `~/.local/bin/claude`, `/usr/local/bin/claude`, and then PATH.
