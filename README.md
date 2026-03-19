# fusebox

Remote Claude Code session manager with managed file sync and optional sandbox isolation. One binary — TUI dashboard on your machine, interactive CLI on the server, JSON over SSH.

```
┌───────────────────────────────────────────────┐
│  FUSEBOX · my-server · 3 active sessions      │
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
- (Optional) Kernel ≥5.11 with user namespaces for sandbox isolation

## Setup

### Quick start (no Go needed)

```bash
curl -sSL https://raw.githubusercontent.com/lableaks/fusebox/master/install.sh | bash
fusebox init user@your-server
```

The installer downloads the right binary for your OS/arch and puts it in `~/.local/bin`. Then `fusebox init` walks you through setup interactively — it tests SSH, deploys the server binary, discovers directories, and writes your config.

The Mac release binary has Linux server binaries embedded, so `fusebox init` just extracts and SCPs — no Go toolchain or cross-compilation needed.

### From source

```bash
make deploy SERVER=myserver.example.com SERVER_USER=deploy
```

This creates `~/.config/fusebox/config.yaml` automatically, installs `fusebox` locally, and deploys to the server. After that, `make deploy` reads the config — no flags needed.

### Subsequent deploys

```bash
make deploy             # from source
fusebox init user@host  # or re-run the wizard (reconfigures)
```

Both are safe to run while sessions are active. Make sure `~/.local/bin` is on your PATH.

## Usage

### TUI Dashboard (Mac)

```bash
fusebox              # launches TUI when config has server.host
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

When the hostname matches `server.host` in your config, the TUI runs locally — no SSH to itself. This means you can run `fusebox` directly on the server and get the full dashboard.

### Local CLI (server / phone SSH)

```bash
fusebox                  # list sessions (no config)
fusebox ls               # list sessions
fusebox new [filter]     # create session (interactive dir picker)
fusebox attach [n|name]  # attach to session by number or name
fusebox kill [n|name]    # stop session by number or name
fusebox peek [n|name]    # preview session output
fusebox help             # show all commands
```

### Server Commands (JSON, called via SSH)

```bash
fusebox list             # JSON array of sessions
fusebox create <n> <dir> # create session
fusebox create-team <n> <dir>  # create session with agent teams enabled
fusebox stop <name>      # stop session
fusebox dirs             # browsable directories (from roots.conf)
fusebox subdirs <path>   # list subdirs of path with counts
fusebox preview <n> [lines]
fusebox activity         # tool activity for all sessions
fusebox teams            # list active teams with members and tasks
fusebox teams-toggle <on|off>  # enable/disable agent teams in settings.json
fusebox panes <session>  # list tmux panes for a session
fusebox pane-preview <session> <pane> [lines]  # capture a specific pane
fusebox install-hooks    # install Claude Code PostToolUse hook
fusebox fix-mouse        # enable mouse mode on all sessions
fusebox hook             # PostToolUse hook handler (reads stdin)
```

### File sync

Fusebox wraps [mutagen](https://mutagen.io) for bidirectional file sync between your Mac and the server. Your code lives in both places, always in sync.

```bash
fusebox sync add ~/projects        # start syncing a local folder
fusebox sync add ~/work/my-app     # add another
fusebox sync ls                    # list active syncs with status
fusebox sync rm ~/projects         # stop syncing a folder
fusebox sync pause                 # pause all syncs
fusebox sync resume                # resume all syncs
```

Synced files land at `~/.fusebox/sync/<name>/` on the server. Mutagen is auto-installed to `~/.fusebox/bin/` if not already in PATH.

**Data flow:**
```
Your Mac                              Server
~/projects/ ◄──── mutagen ────► ~/.fusebox/sync/projects/
~/work/app/ ◄──── mutagen ────► ~/.fusebox/sync/app/
              (bidirectional)
```

Edit locally → changes appear on server within seconds. Claude edits on server → changes sync back to your IDE.

### Sandbox isolation (Linux, optional)

Sandbox mode runs Claude sessions inside a Linux namespace with OverlayFS — an isolated environment where `rm -rf /` can't hurt the host.

```bash
fusebox up                         # start sandbox (downloads rootfs on first run)
fusebox down                       # stop sandbox (sync continues)
fusebox sandbox-status             # show sandbox + sync status
fusebox update                     # update Claude Code inside sandbox
```

Enable sandbox during `fusebox init` (detects kernel support automatically) or in config:

```yaml
sandbox:
  enabled: true
```

When sandbox is enabled, `fusebox create` auto-starts it. `fusebox down` kills sessions but doesn't stop sync. Requires kernel ≥5.11 with user namespaces. Falls back to bare-host mode on older kernels.

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

Claude Code's experimental agent teams feature lets a lead session spawn teammate sessions that coordinate via shared task lists. Fusebox integrates with this — it doesn't orchestrate teams itself, but gives you visibility and control.

**Enabling:** Toggle teams globally with `fusebox teams-toggle on`, or per-session in the create flow with `[t]`.

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

**Tmux tip:** Sessions run inside tmux, which captures mouse events for scrolling and pane selection. To click links, select text, or right-click normally, hold **Shift** while clicking.

## Configuration

Config lives at `~/.config/fusebox/config.yaml`. Without it, `fusebox` runs in local CLI mode.

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

## Troubleshooting

### `~/.local/bin` not in PATH

Claude Code installs to `~/.local/bin`, but on Ubuntu this directory is only added to PATH in `~/.profile` (login shells). Tmux spawns non-login shells, so `~/.local/bin` is missing.

`fusebox init` fixes this automatically by adding an `export PATH` line to `~/.bashrc`. If you set up manually, add this to your server's `~/.bashrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Tmux mouse mode

Fusebox enables mouse mode in tmux sessions for scrolling. To click links, select text, or use your terminal's native mouse behavior, hold **Shift** while clicking.

## Development

```bash
make test          # run all Go tests (client + server)
make test-server   # run server integration tests (needs tmux)
make test-e2e      # run end-to-end tests (needs docker)
make build         # compile client binary (dev — no embedded server)
make build-server  # cross-compile for linux/arm64
make release       # build with embedded Linux binaries (for fusebox init)
make rootfs        # build rootfs tarballs for sandbox (needs docker + buildx)
make deploy        # build everything, push to server, install hooks
make clean         # remove build artifacts
```

### Release builds

`make release` cross-compiles Linux arm64 and amd64 server binaries, then builds the Mac binary with them embedded via `go:embed`. The resulting binary can deploy itself to a server via `fusebox init` without needing Go on the target machine.

Dev builds (`make build`, `go build`) work normally but `fusebox init` will show a "use release build" error at the deploy step.

## Architecture

```
Your Mac                              Server
┌──────────────┐                      ┌────────────────────────┐
│ ~/projects/  │◄──── mutagen ────►│ ~/.fusebox/sync/projects/ │
│              │   (bidirectional)    │                          │
│ fusebox      │                      │ [sandbox namespace]      │
│  (TUI)  ────┼──── SSH ────────────►│  ├── tmux + claude       │
│              │                      │  ├── Node.js, git, etc.  │
│ fusebox sync │                      │  └── full permissions    │
└──────────────┘                      └────────────────────────┘
```

One `fusebox` binary runs everywhere. On the Mac (with config), it launches the TUI dashboard over SSH. On the server, if the hostname matches `server.host`, the TUI runs locally with no SSH. Otherwise it falls back to the interactive CLI.

**Sync layer:** `fusebox sync` wraps mutagen for bidirectional file sync. Runs on the client, independent of the sandbox. Synced files land at `~/.fusebox/sync/<name>/` on the server.

**Sandbox layer:** `fusebox up` creates a Linux namespace with OverlayFS (Alpine rootfs + Node.js + Claude Code). The sandbox bind-mounts synced directories, `~/.claude/`, and `/tmp/fusebox/`. All tmux commands route through a custom socket so existing server tmux is unaffected.

**Activity:** Claude Code's PostToolUse hook fires `fusebox hook`, which writes status to `/tmp/fusebox/<session>.json`. The dashboard polls every 5 seconds.

`fusebox` auto-detects the claude binary by checking `~/.local/bin/claude`, `/usr/local/bin/claude`, and then PATH. Inside a sandbox, it uses the known rootfs path.
