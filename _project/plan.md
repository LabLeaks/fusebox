# work-cli: Remote Claude Session Manager

## Overview

A beautiful local TUI that manages Claude Code tmux sessions on a remote Hetzner server (spotless-1). Built with Go + Bubbletea v2 + Lipgloss.

The core idea: run `work` on your Mac (or Termius on iPhone), get an interactive dashboard of all active Claude sessions on the server. Create new ones, attach to existing ones, kill old ones. All sessions run in yolo mode (auto-accept) + remote-control mode.

## Architecture

```
┌─────────────────┐         SSH          ┌──────────────────────┐
│  Local Mac       │ ◄──────────────────► │  spotless-1 (Hetzner) │
│                  │                      │                       │
│  work (TUI)      │  ssh commands ──────►│  tmux sessions        │
│  - list sessions │  ◄── json output ──  │  ├── 4d-instrument    │
│  - create new    │                      │  ├── conscience-pipe   │
│  - attach        │  ssh -t attach ─────►│  └── spotless          │
│  - stop          │                      │                       │
│  - mutagen status│                      │  claude (yolo+remote) │
└─────────────────┘                      └──────────────────────┘
```

**Local binary** handles all TUI rendering (fast, responsive).
**Remote commands** are thin SSH calls that return JSON (session list, create, stop).
**Attach** drops out of TUI and does `ssh -t` to attach to the tmux session directly.

## Tech Stack

- **Language**: Go 1.22+
- **TUI**: Bubbletea v2 + Lipgloss + Bubbles
- **SSH**: golang.org/x/crypto/ssh (for command execution)
- **Remote**: tmux on spotless-1 (sessions persist across disconnects)
- **Claude**: claude --dangerously-skip-permissions (+ remote-control flags)

## Features

### v1 (MVP)

**Dashboard view:**
- List all active claude tmux sessions
- Show: session name, directory, uptime, status (running/idle)
- Pretty table with Lipgloss styling

**Create session:**
- Browse server directories (~/work/) to pick a project
- Auto-names session by directory (e.g., `4d-instrument`)
- Launches: `tmux new-session -d -s <name> -c <dir> "claude --dangerously-skip-permissions"`
- Remote-control mode enabled by default

**Attach to session:**
- Select from list, drops into `ssh -t spotless@spotless-1 tmux attach -t <name>`
- TUI exits, raw terminal takes over
- When you detach (ctrl-b d), you're back on your Mac

**Stop session:**
- Kill a tmux session cleanly
- Confirmation prompt

**Mutagen status:**
- Show sync status (watching/staging/error) in the dashboard footer
- Warn before creating sessions if sync is broken

### v2 (Future)

- Session output preview (last N lines without attaching)
- Resource usage per session (CPU/RAM on server)
- Auto-cleanup of idle sessions after configurable timeout
- Session groups (research / projects / trinsic)
- Notifications when a session completes a task
- Log viewer for past sessions (~/.work-logs/)
- Multiple server support

## Server-Side Requirements

- tmux installed (already is)
- Claude Code installed (already is)
- SSH key auth working (already is)
- A thin helper script at /home/spotless/bin/work-helper that:
  - Lists tmux sessions as JSON
  - Creates new sessions with proper claude flags
  - Returns session metadata

## Directory Detection

When creating a session, browse from these roots:
- ~/work/lableaks/research/
- ~/work/lableaks/projects/
- ~/work/trinsic/
- ~/work/azimuth/
- ~/work/paypal/
- ~/work/random/

## Session Naming

Auto-generated from directory:
- `/home/spotless/work/lableaks/research/4d-instrument` → `4d-instrument`
- `/home/spotless/work/trinsic/freezeray` → `freezeray`
- Collision handling: append `-2`, `-3` etc.

## Config

`~/.config/work-cli/config.yaml`:
```yaml
server:
  host: spotless-1
  user: spotless

claude:
  flags: "--dangerously-skip-permissions"

defaults:
  mode: yolo+remote-control

browse_roots:
  - ~/work/lableaks/research
  - ~/work/lableaks/projects
  - ~/work/trinsic
  - ~/work/azimuth
  - ~/work/paypal
  - ~/work/random
```

## Install

```bash
# Build
go build -o work .

# Install locally
cp work /usr/local/bin/work

# Or via go install
go install github.com/lableaks/work-cli@latest
```

## UX Flow

```
$ work

  ╭──────────────────────────────────────────────╮
  │  WORK  ·  spotless-1  ·  3 active sessions   │
  ╰──────────────────────────────────────────────╯

  SESSION            DIR                        UPTIME    STATUS
  ─────────────────────────────────────────────────────────────
  4d-instrument      lableaks/research/4d-in…   2h 34m    ● running
  spotless           lableaks/projects/spotl…   45m       ● running
  freezeray          trinsic/freezeray           12m       ○ idle

  [n] new session  [enter] attach  [d] stop  [q] quit

  mutagen: ✓ watching  ·  last sync: 2s ago
```
