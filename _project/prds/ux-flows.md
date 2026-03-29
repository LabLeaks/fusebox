# Fusebox UX Flows

**Status:** Draft
**Author:** GK
**Created:** 2026-03-28
**Parent:** PRD-001-fusebox.md

---

## Design Principles

1. **Silent success, loud failure.** Don't narrate steps that work. Speak up immediately when something breaks.
2. **Zero-click best practice.** Defaults are opinionated. The user shouldn't need flags for the common case.
3. **Infrastructure, not UI.** Fusebox is a daemon and CLI. No spinners, no color gradients, no ASCII art. Monochrome status lines.
4. **Power user assumptions.** No "are you sure?" prompts. No tutorials. Confirmations only for destructive operations.
5. **Composable with claudebar.** Fusebox outputs machine-parseable status. claudebar renders it pretty.

---

## 1. First-time Setup: `fusebox init`

### Prerequisites

User has:
- SSH access to a Linux server (key-based auth configured)
- `mutagen` installed locally (`brew install mutagen`)
- A Claude Code installation locally (for `claude setup-token`)

### Flow

```
$ fusebox init

Server hostname or IP: spotless-1
SSH user [gk]:
SSH port [22]:

Connecting to spotless-1... ok
  Linux 6.1, Debian 12, 4 cores, 8GB RAM, 112GB free

Installing fusebox daemon on server... ok
  /usr/local/bin/fusebox-remote v0.3.0

Claude Code auth token:
  No reusable token found in ~/.fusebox/config.
  Running: claude setup-token
  [Claude Code token setup flow runs here]
  Token stored in ~/.fusebox/config (encrypted, server: spotless-1)

Detecting project: ~/work/lableaks/projects/my-ios-app
  Xcode workspace detected: App.xcworkspace
  Suggested fusebox.yaml written to ./fusebox.yaml

Done. Review fusebox.yaml, then run: fusebox up
```

### What `fusebox init` creates

**~/.fusebox/config** (global, created once):
```yaml
servers:
  spotless-1:
    host: spotless-1
    user: gk
    port: 22
    claude_token: enc:v1:aes256:... # encrypted at rest

defaults:
  server: spotless-1
```

**./fusebox.yaml** (per-project, generated with detected defaults):
```yaml
version: 1
server: spotless-1

sync:
  ignore:
    - "DerivedData/"
    - "*.xcuserdata"
    - ".build/"
    - "Pods/"

actions:
  build_ios:
    description: "Build iOS app for simulator"
    exec: "xcodebuild -workspace App.xcworkspace -scheme App -sdk iphonesimulator"

  run_tests:
    description: "Run Xcode test suite"
    exec: "xcodebuild test -workspace App.xcworkspace -scheme App -only-testing:{test_target}"
    params:
      test_target:
        type: regex
        pattern: "^[a-zA-Z0-9_]+Tests$"
```

### Detection heuristics

| Detected file | Project type | Generated actions |
|---------------|-------------|-------------------|
| `*.xcworkspace` / `*.xcodeproj` | iOS/macOS | `build_ios`, `run_tests`, `archive` |
| `Cargo.toml` | Rust | `build`, `test`, `clippy` |
| `train.py` / `requirements.txt` with torch | ML/PyTorch | `train`, `evaluate` |
| `platformio.ini` | Embedded | `build`, `flash`, `monitor` |
| `*.uproject` | Unreal Engine | `build`, `cook` |
| (fallback) | Generic | Empty actions section with commented examples |

### Error cases

```
$ fusebox init
Server hostname or IP: badhost

Connecting to badhost... failed
  SSH connection refused. Verify hostname and that SSH key auth is configured.
  Tried: ssh gk@badhost -p 22

$ fusebox init
# (in a directory that already has fusebox.yaml)

fusebox.yaml already exists. To reconfigure server: edit ~/.fusebox/config
To regenerate actions: fusebox init --actions
```

---

## 2. Starting a Session: `fusebox up`

### Cold start (first time for this project on this server)

```
$ fusebox up

spotless-1: creating container my-ios-app... ok
spotless-1: syncing source (2,847 files, 48MB)... ok [14s]
spotless-1: installing claude code... ok
spotless-1: starting rpc bridge... ok
spotless-1: injecting auth token... ok

Ready. Connect with:
  claudebar attach spotless-1/my-ios-app

Local daemon listening on :9600 (rpc backchannel)
```

Total cold start: ~45-90 seconds depending on project size and network.

### Warm start (container exists, reconnecting)

```
$ fusebox up

spotless-1: container my-ios-app running
spotless-1: resuming sync... ok [2s, 3 files changed]
spotless-1: rpc bridge... ok

Ready. Connect with:
  claudebar attach spotless-1/my-ios-app

Local daemon listening on :9600 (rpc backchannel)
```

Total warm start: ~3-5 seconds.

### What "ready" means

`fusebox up` blocks until all of these are true:
1. Container is running
2. Mutagen sync has completed initial transfer
3. RPC backchannel is authenticated and connected
4. Claude Code is installed and auth token is valid

Only then does it print "Ready." If any step fails, it fails fast with the specific error.

### Foreground vs background

`fusebox up` runs the local daemon in the foreground by default. The RPC log streams below the "Ready" line:

```
Ready. Connect with:
  claudebar attach spotless-1/my-ios-app

Local daemon listening on :9600 (rpc backchannel)
[09:14:22] rpc: build_ios started (pid 48201)
[09:14:35] rpc: build_ios completed (0) [13s]
[09:15:01] rpc: run_tests started (pid 48215)
[09:15:08] rpc: run_tests completed (0) [7s]
```

For background mode:
```
$ fusebox up -d

spotless-1: container my-ios-app running
spotless-1: resuming sync... ok
spotless-1: rpc bridge... ok

Ready. Daemon running in background (pid 71042).
  claudebar attach spotless-1/my-ios-app
  fusebox logs    # view rpc log
  fusebox down    # stop
```

### Auto-launch claudebar

If the `FUSEBOX_AUTO_ATTACH` environment variable is set (or configured in `~/.fusebox/config`), `fusebox up` automatically runs `claudebar attach` after "Ready":

```yaml
# ~/.fusebox/config
defaults:
  auto_attach: true
```

```
$ fusebox up

spotless-1: container my-ios-app running
spotless-1: resuming sync... ok
spotless-1: rpc bridge... ok

Ready. Attaching via claudebar...
# (user is now in the remote tmux session)
```

This is opt-in because some users run `fusebox up` in one terminal tab and `claudebar attach` in another.

---

## 3. Working Session (Agent-side)

The agent is running inside the remote container. It discovers and invokes local tools.

### Discovery

The agent sees available actions via CLI or MCP:

```
$ fusebox actions

build_ios     Build iOS app for simulator
run_tests     Run Xcode test suite (params: test_target)
train_local   Run training on local GPU (params: epochs)
```

This is what the agent uses to understand what local tools are available. Descriptions are written for the agent, not the human.

### CLI execution: `fusebox exec`

```
$ fusebox exec build_ios

[local] xcodebuild -workspace App.xcworkspace -scheme App -sdk iphonesimulator
...
Build Succeeded
0 errors, 2 warnings

[exit 0, 13.4s]
```

Stdout and stderr stream in real-time over the RPC channel. The agent sees output as it happens, not buffered at the end.

**With parameters:**
```
$ fusebox exec run_tests --test_target=AuthTests

[local] xcodebuild test -scheme App -only-testing:AuthTests
...
Test Suite 'AuthTests' passed (4/4)

[exit 0, 7.1s]
```

**Parameter validation failure:**
```
$ fusebox exec run_tests --test_target="rm -rf /"

Error: parameter 'test_target' failed validation
  Expected: regex ^[a-zA-Z0-9_]+Tests$
  Got: "rm -rf /"
```

The agent gets a clear, structured error. No execution happens.

### MCP tool execution

When the agent calls a fusebox action via MCP tool_use, the flow is identical but the transport is JSON-RPC instead of CLI stdout:

```json
// Agent sends:
{
  "tool": "fusebox_build_ios",
  "input": {}
}

// Agent receives (streaming):
{
  "output": "Build Succeeded\n0 errors, 2 warnings",
  "exit_code": 0,
  "duration_ms": 13400
}
```

MCP tools are auto-registered from fusebox.yaml actions. The agent sees them as `fusebox_<action_name>` in its tool list.

### Timeouts

Default timeout: 10 minutes per action. Configurable per action:

```yaml
actions:
  train_local:
    description: "Run training on local GPU"
    exec: "python train.py --epochs {epochs}"
    timeout: 3600  # 1 hour
```

Timeout behavior:
```
$ fusebox exec train_local --epochs=100

[local] python train.py --epochs=100
Epoch 1/100: loss=2.341
Epoch 2/100: loss=1.892
...

Error: action timed out after 600s
  Process killed (SIGTERM, then SIGKILL after 5s)
  Partial output above.

[exit 137, 600.0s]
```

### Local machine offline

The local daemon is unreachable (laptop lid closed, WiFi dropped, machine off):

```
$ fusebox exec build_ios

Error: local machine unreachable
  The local fusebox daemon is not responding.
  Last seen: 3 minutes ago.
  The developer's machine may be offline. This action requires local execution.

[exit 1]
```

The error is written so the agent can reason about it: this is a temporary infrastructure problem, not a code problem. The agent should continue with work that doesn't require local tools and retry later.

### No matching action

```
$ fusebox exec deploy_prod

Error: unknown action 'deploy_prod'
  Available actions: build_ios, run_tests, train_local
  Actions are defined in fusebox.yaml on the local machine.

[exit 1]
```

---

## 4. Stopping: `fusebox down`

### Default: soft stop

```
$ fusebox down

spotless-1/my-ios-app:
  Stopping rpc bridge... ok
  Stopping sync... ok
  Container left running (warm restart available).

Stopped. Run 'fusebox up' to reconnect.
```

Default behavior preserves the container. This is the common case: you're done for the day, but you want a fast reconnect tomorrow. The container idles at near-zero CPU. Disk is cheap.

### Destroy: hard stop

```
$ fusebox down --destroy

spotless-1/my-ios-app:
  Stopping rpc bridge... ok
  Stopping sync... ok
  Removing container... ok
  Freed 1.2GB disk on spotless-1.

Destroyed. Run 'fusebox up' for a fresh container.
```

This is for cleanup: switching projects, freeing disk, starting fresh.

### What about running actions?

If an action is currently executing when `fusebox down` is called:

```
$ fusebox down

Warning: action 'train_local' is running (pid 48201, 4m elapsed).
  --force to kill it, or wait for completion.
```

No `--force` flag needed for the common case (nothing running). When something is running, the user must be explicit.

---

## 5. Multi-project Management

### `fusebox list`

Shows all fusebox sessions across all servers:

```
$ fusebox list

SERVER       PROJECT         STATUS    SYNC     LAST RPC
spotless-1   my-ios-app      running   synced   build_ios (2m ago)
spotless-1   ml-experiment   stopped   --       train_local (3d ago)
hetzner-2    game-engine     running   syncing  --
```

### `fusebox status`

Shows detailed status for the current project (detected from cwd):

```
$ fusebox status

Project:    my-ios-app
Server:     spotless-1
Container:  running (uptime 4h12m)
Sync:       synced (2,847 files, last change 30s ago)
RPC:        connected (local daemon pid 71042)
Actions:    3 registered (build_ios, run_tests, train_local)
Last RPC:   build_ios -> exit 0 (2m ago)
```

If not in a fusebox project directory:

```
$ fusebox status

Not in a fusebox project (no fusebox.yaml found).
Run 'fusebox init' to set up, or 'fusebox list' to see all sessions.
```

### `fusebox logs`

Shows the RPC execution log for the current project:

```
$ fusebox logs

[09:14:22] build_ios started (pid 48201)
[09:14:35] build_ios completed (exit 0) [13s]
[09:15:01] run_tests --test_target=AuthTests started (pid 48215)
[09:15:08] run_tests completed (exit 0) [7s]
[09:22:44] build_ios started (pid 48301)
[09:22:58] build_ios completed (exit 0) [14s]

$ fusebox logs -f  # follow (tail -f style)
$ fusebox logs -n 50  # last 50 entries
```

---

## 6. Configuration

### Two config files, strict separation

| File | Scope | Contains | Synced to remote? |
|------|-------|----------|-------------------|
| `~/.fusebox/config` | Global (all projects) | Server connections, auth tokens, user preferences | No |
| `./fusebox.yaml` | Per-project | Sync ignore rules, whitelisted actions, action params | No |

Neither file ever leaves the local machine.

### ~/.fusebox/config

```yaml
servers:
  spotless-1:
    host: spotless-1
    user: gk
    port: 22
    claude_token: enc:v1:aes256:...

  hetzner-2:
    host: 159.69.xx.xx
    user: deploy
    port: 22
    claude_token: enc:v1:aes256:...

defaults:
  server: spotless-1
  auto_attach: false
  log_level: info
```

### fusebox.yaml

```yaml
version: 1
server: spotless-1  # override default server for this project

sync:
  ignore:
    - "DerivedData/"
    - "*.xcuserdata"
    - ".build/"
    - "Pods/"
    - "*.ipa"

actions:
  build_ios:
    description: "Build iOS app for simulator"
    exec: "xcodebuild -workspace App.xcworkspace -scheme App -sdk iphonesimulator"

  run_tests:
    description: "Run Xcode test suite"
    exec: "xcodebuild test -workspace App.xcworkspace -scheme App -only-testing:{test_target}"
    timeout: 300
    params:
      test_target:
        type: regex
        pattern: "^[a-zA-Z0-9_]+Tests$"
```

### Precedence

1. CLI flags override everything: `fusebox up --server=hetzner-2`
2. `fusebox.yaml` `server:` field overrides global default
3. `~/.fusebox/config` `defaults.server` is the fallback

### Editing config

No `fusebox config` subcommand. These are YAML files. Users edit them directly.

Adding a new server after initial setup:
```
$ fusebox init --server
Server hostname or IP: hetzner-2
...
Server added to ~/.fusebox/config.
```

---

## 7. Error States

Every error follows the same format:
```
Error: <one-line summary>
  <detail line 1>
  <detail line 2>
  <suggested fix or next step>
```

### Server unreachable

```
$ fusebox up

Error: cannot connect to spotless-1
  SSH connection timed out after 10s.
  Last successful connection: 2h ago.
  Check: ssh gk@spotless-1
```

### Container crashed

```
$ fusebox status

Project:    my-ios-app
Server:     spotless-1
Container:  crashed (exit 137, OOM killed)
  Container ran out of memory. Server has 8GB RAM.
  fusebox up       # restart with existing state
  fusebox up --fresh  # restart from scratch
```

### Mutagen sync conflict

Mutagen handles conflicts automatically (local wins for fusebox.yaml, newest-wins for source files). But if sync is stuck:

```
$ fusebox status

Project:    my-ios-app
Server:     spotless-1
Sync:       error (3 conflicts)
  DerivedData/Build/... -- add to sync.ignore in fusebox.yaml
  Run: fusebox sync reset   # force re-sync from local
```

`fusebox sync reset` is the escape hatch. It does a full re-push from local to remote. Destructive to remote state, safe for local state.

### Auth token expired

```
$ fusebox up

spotless-1: container my-ios-app running
spotless-1: resuming sync... ok
spotless-1: injecting auth token... failed

Error: Claude Code auth token expired
  Token for spotless-1 was last refreshed 31 days ago.
  Run: fusebox auth refresh
```

`fusebox auth refresh` re-runs the `claude setup-token` flow and updates `~/.fusebox/config`.

### Local daemon not running

From the agent's perspective (inside the remote container):

```
$ fusebox exec build_ios

Error: local machine unreachable
  The local fusebox daemon is not responding.
  Last seen: never (daemon may not be running).
  The developer needs to run 'fusebox up' on their local machine.

[exit 1]
```

From the local machine (user forgot to start it):

```
$ fusebox status

Error: local daemon not running
  The fusebox daemon is not active for this project.
  Run: fusebox up
```

### Invalid fusebox.yaml

```
$ fusebox up

Error: invalid fusebox.yaml
  Line 12: action 'build_ios' missing required field 'exec'
  Fix fusebox.yaml and retry.
```

Validation happens at `fusebox up` time. If fusebox.yaml is invalid, nothing starts.

---

## 8. Integration with claudebar

### Handoff model

Fusebox and claudebar are separate tools with a clean boundary:

| Responsibility | Fusebox | claudebar |
|---------------|---------|-----------|
| Server/container lifecycle | Yes | No |
| File sync | Yes | No |
| RPC bridge | Yes | No |
| mosh connection | No | Yes |
| tmux session management | No | Yes |
| Interactive terminal UX | No | Yes |

### Connection handoff

`fusebox up` outputs a connection command that claudebar understands:

```
Ready. Connect with:
  claudebar attach spotless-1/my-ios-app
```

If `auto_attach: true`, fusebox execs into claudebar directly. Otherwise, the user runs it manually (likely in another terminal pane).

### Status interface for claudebar

Fusebox exposes session state via a Unix socket at `~/.fusebox/run/<project>.sock` that claudebar can poll:

```json
{
  "project": "my-ios-app",
  "server": "spotless-1",
  "container": "running",
  "sync": "synced",
  "rpc": "connected",
  "last_rpc": {
    "action": "build_ios",
    "exit_code": 0,
    "duration_ms": 13400,
    "timestamp": "2026-03-28T09:14:35Z"
  },
  "actions_available": ["build_ios", "run_tests", "train_local"]
}
```

claudebar can render this however it wants: tmux status bar, overlay, nothing. Fusebox doesn't care.

### What claudebar could show (suggestions, not fusebox's job)

```
tmux status bar:
[fusebox: synced | rpc: ok | last: build_ios exit 0 (2m)]
```

If local daemon goes offline:
```
[fusebox: synced | rpc: OFFLINE | local daemon unreachable]
```

This is claudebar's design problem. Fusebox just provides the data.

---

## Summary of Commands

| Command | Purpose |
|---------|---------|
| `fusebox init` | First-time setup: server, auth, generate fusebox.yaml |
| `fusebox up` | Start/reconnect session (container + sync + rpc) |
| `fusebox up -d` | Start session, daemon in background |
| `fusebox down` | Soft stop (container stays, sync/rpc stop) |
| `fusebox down --destroy` | Hard stop (remove container) |
| `fusebox exec <action>` | Trigger local action from remote (agent-facing) |
| `fusebox actions` | List available actions (agent-facing) |
| `fusebox status` | Current project session status |
| `fusebox list` | All sessions across servers |
| `fusebox logs` | RPC execution log |
| `fusebox logs -f` | Follow RPC log |
| `fusebox sync reset` | Force re-sync from local |
| `fusebox auth refresh` | Re-run Claude Code token setup |
| `fusebox init --server` | Add another server to global config |
| `fusebox init --actions` | Regenerate actions from project detection |

### No subcommand

```
$ fusebox

Usage: fusebox <command>

  init      Set up server and project
  up        Start session
  down      Stop session
  exec      Run local action (agent-facing)
  actions   List local actions (agent-facing)
  status    Session status
  list      All sessions
  logs      RPC execution log
  sync      Sync management
  auth      Auth token management

Run 'fusebox <command> --help' for details.
```

No `--version` spam. No `--verbose` in help text. The help is one screen, no scrolling.
