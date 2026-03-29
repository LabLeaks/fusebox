# Sprint 001: MVP Bridge

**Status:** Complete
**Goal:** Build the minimum viable Fusebox: `fusebox up` provisions a Sysbox container on a remote server with Claude Code, syncs source via Mutagen, establishes the RPC bridge, and the agent can trigger whitelisted local actions via `fusebox exec` and MCP tools.
**Target:** 60-second demo video showing split-screen agent-on-VPS + local Xcode build
**Started:** 2026-03-29
**Completed:** 2026-03-28

---

## Scope

### IN

1. `fusebox up` -- provisions Sysbox container, starts Mutagen sync, establishes SSH reverse tunnel for RPC, starts local daemon
2. `fusebox exec <action>` -- remote CLI triggers local actions through RPC tunnel, parameterized with validation, streams stdout/stderr
3. `fusebox actions` -- remote CLI lists available actions via RPC tunnel query
4. MCP server -- remote-side thin wrapper exposing fusebox.yaml actions as MCP tools
5. `fusebox down` -- stops sync + tunnel, leaves container (warm restart); `--destroy` removes container
6. `fusebox status` -- shows current project session status
7. Sync-wait mechanism -- dual-ended Mutagen gRPC polling; remote blocks until outbound clear, local blocks until inbound landed
8. Local daemon -- listens on SSH reverse tunnel port, validates against fusebox.yaml, executes whitelisted actions, streams results
9. Claude Code state sync -- bidirectional Mutagen sync of project memories + session logs (path-mapped), one-time copy of global CLAUDE.md, agents, skills, and transformed settings.json. Enables `claude --continue` across local/remote.
10. `fusebox.example.yaml` -- commented examples for iOS, GPU training, CLI tools, embedded
11. Config files hand-written by user for sprint 1 (`~/.fusebox/config` + `fusebox.yaml`)

### OUT

- `fusebox setup` (server prep -- document as manual prereq)
- `fusebox init` (full interactive setup wizard -- sprint 2)
- `fusebox list` (multi-project management)
- `fusebox logs` (RPC execution log viewer)
- `fusebox sync reset` (escape hatch)
- `fusebox auth refresh`
- Auto-detection of project type
- Background daemon mode (`fusebox up -d`)
- claudebar auto-attach integration
- Multi-server support in config
- Token encryption (plaintext with chmod 600 for sprint 1)
- Any UI/TUI

---

## Architecture Decisions (locked)

All decisions below are final. Do not re-debate.

| Decision | Detail |
|----------|--------|
| Container runtime | Sysbox (`--runtime=sysbox-runc`). Docker + docker-compose work natively inside. |
| RPC transport | JSON protocol over SSH reverse tunnel (`ssh -R`). Message types: exec, stdout, stderr, exit, actions, error. No gRPC, no protobuf. |
| Sync | Mutagen as sidecar (not embedded). Source-only sync with configurable ignores from fusebox.yaml. |
| Sync-wait | Dual-ended Mutagen gRPC polling over Unix socket (`~/.mutagen/daemon.sock`). Check for "Watching" status before transmitting/executing. |
| Auth token | Injected into container as env var during `docker run`. |
| RPC auth | Shared secret generated during `fusebox up`, required in every RPC message. |
| MCP | Remote MCP server auto-registers fusebox.yaml actions as tools. Thin wrapper over JSON-over-tunnel protocol. |
| Parameter validation | regex, enum, int range -- validated on local daemon before execution. |
| Language | Go. Single binary for local (macOS/Linux) and remote (Linux). |

---

## Task Breakdown

### Phase 1: Project Scaffolding + Core Types

**P1.1 -- Go module + CLI skeleton** [M]

Initialize Go module, set up cobra CLI with subcommand stubs for all Sprint 1 commands. Wire up build system.

- Acceptance criteria:
  - `go build ./cmd/fusebox` produces a binary
  - `fusebox` prints usage with all subcommands listed
  - `fusebox setup --help`, `fusebox init --help`, etc. all print placeholder help
- Files created:
  - `go.mod`, `go.sum`
  - `cmd/fusebox/main.go` -- entry point
  - `cmd/fusebox/root.go` -- root command + subcommand registration
  - `cmd/fusebox/setup.go`, `init.go`, `up.go`, `down.go`, `exec.go`, `actions.go`, `status.go` -- stubs
  - `Makefile` -- `build`, `build-remote` (GOOS=linux), `install` targets
- Size: M
- Blocks: everything

**P1.2 -- Core types and config parsing** [M]

Define Go types for fusebox.yaml, ~/.fusebox/config, and the RPC protocol messages. Implement YAML parsing and validation for both config files.

- Acceptance criteria:
  - `fusebox.yaml` parses into `ProjectConfig` struct: version, server, sync.ignore, actions map
  - `~/.fusebox/config` parses into `GlobalConfig` struct: servers map, defaults
  - Action struct has: name, description, exec template, timeout, params map
  - Param struct has: type (regex/enum/int), pattern, values, range
  - RPC message types defined: `ExecRequest`, `ExecResponse` (stdout/stderr/exit), `ActionsRequest`, `ActionsResponse`, `ErrorResponse`
  - All messages include `secret` field for auth
  - Invalid YAML produces clear error with line number
  - Unit tests for parsing valid and invalid configs
- Files created:
  - `internal/config/project.go` -- ProjectConfig, Action, Param types + parsing
  - `internal/config/global.go` -- GlobalConfig, ServerConfig types + parsing
  - `internal/config/validation.go` -- YAML validation with line-number errors
  - `internal/config/config_test.go` -- unit tests
  - `internal/rpc/protocol.go` -- RPC message type definitions
  - `testdata/valid.fusebox.yaml`, `testdata/invalid.fusebox.yaml` -- fixtures
- Size: M
- Blocks: P2.1, P4.1, P5.1

**P1.3 -- Project resolver** [S]

Utility that finds fusebox.yaml by walking up from cwd, resolves the effective server (CLI flag > fusebox.yaml > global default), and loads both configs.

- Acceptance criteria:
  - Walks up from cwd to find fusebox.yaml (like git finds .git)
  - Returns merged config: project config + resolved server from global config
  - Clear error when no fusebox.yaml found: "Not in a fusebox project (no fusebox.yaml found)."
  - Clear error when referenced server not in global config
  - Unit tests for resolution precedence
- Files created:
  - `internal/config/resolver.go`
  - `internal/config/resolver_test.go`
- Size: S
- Blocks: P2.3 (fusebox up needs resolved config)

---

### Phase 2: Server Setup + Container Management

**P2.1 -- SSH client wrapper** [M]

Go wrapper around SSH for running commands on the remote server. Supports command execution with streaming stdout/stderr, SCP file transfer, and SSH reverse tunnel creation.

- Acceptance criteria:
  - Connects using host/user/port from GlobalConfig (uses system SSH agent for keys)
  - `RunCommand(cmd string) (stdout, stderr, exitCode)` -- blocking execution
  - `RunCommandStream(cmd string, stdout, stderr io.Writer)` -- streaming execution
  - `CopyFile(local, remote string)` -- SCP upload
  - `ReverseTunnel(remotePort, localPort int) (closer, error)` -- establishes `ssh -R`
  - Timeout handling (10s connect, configurable command timeout)
  - Integration test that connects to localhost (skipped if no SSH)
- Files created:
  - `internal/ssh/client.go`
  - `internal/ssh/tunnel.go`
  - `internal/ssh/client_test.go`
- Size: M
- Blocks: P2.2, P4.2

**P2.2 -- Container lifecycle (create, start, stop, remove)** [M]

Manages Sysbox containers on the remote server. One container per project, named `fusebox-<project>`.

- Acceptance criteria:
  - `Create(projectName string, claudeToken string)` -- runs `docker run --runtime=sysbox-runc -d --name fusebox-<project> -e CLAUDE_TOKEN=<token> ...` with nestybox/ubuntu-jammy-systemd-docker base image + Claude Code + mosh-server + fusebox binary layered on top
  - `Start(projectName)` -- starts a stopped container
  - `Stop(projectName)` -- stops a running container
  - `Remove(projectName)` -- removes a container
  - `Status(projectName)` -- returns running/stopped/not-found/crashed + details
  - Container image built from Dockerfile layered on nestybox base (Node.js 22 + Claude Code CLI + mosh-server + fusebox binary)
  - Auth token injected as env var, not baked into image layer
  - Idempotent: Create checks if container exists first
  - Unit tests for container name generation, status parsing
- Files created:
  - `internal/container/manager.go` -- container lifecycle operations
  - `internal/container/dockerfile.go` -- Dockerfile generation/template
  - `internal/container/manager_test.go`
  - `build/Dockerfile.remote` -- Dockerfile for the remote container
- Size: M
- Blocks: P3.1 (up command needs container running), P4.2

**P2.3 -- Example config files** [S]

Create fusebox.example.yaml with commented examples and document ~/.fusebox/config format.

- Acceptance criteria:
  - `fusebox.example.yaml` in repo root with commented sections for: iOS, GPU training, pre-authed CLI tools, embedded/hardware
  - Each section shows sync ignores and parameterized actions
  - `config.example.yaml` showing ~/.fusebox/config format (server, user, port, token)
  - Both files are well-commented and self-documenting
- Files created:
  - `fusebox.example.yaml`
  - `config.example.yaml`
- Size: S
- Blocks: nothing

**P2.4 -- `fusebox auth` command** [S]

Wraps `claude setup-token` to capture the auth token and store it in `~/.fusebox/config`. Minimal — no server config, just auth.

- Acceptance criteria:
  - Runs `claude setup-token` as a subprocess, captures the output token
  - Creates `~/.fusebox/config` if it doesn't exist (with just the token field)
  - Updates the token field if config already exists (preserves other fields)
  - Prints confirmation: "Token stored in ~/.fusebox/config"
  - Errors clearly if `claude` binary not found
- Files created:
  - `cmd/fusebox/auth.go` -- command implementation
- Size: S
- Blocks: P4.3 (fusebox up reads the token)

---

### Phase 3: Mutagen Sync Integration

**P3.1 -- Mutagen session manager** [M]

Wraps Mutagen CLI to create, resume, pause, and terminate sync sessions. Configures ignores from fusebox.yaml.

- Acceptance criteria:
  - `Create(localPath, remoteHost, remotePath, ignores)` -- runs `mutagen sync create` with correct alpha/beta URLs and ignore flags
  - `Resume(sessionName)` -- resumes a paused session
  - `Pause(sessionName)` -- pauses a session
  - `Terminate(sessionName)` -- terminates a session
  - Session named `fusebox-<project>` for identification
  - Ignores sourced from fusebox.yaml `sync.ignore` list + hardcoded defaults (.git, fusebox.yaml)
  - fusebox.yaml is ALWAYS in the ignore list (local-only, never synced)
  - Waits for initial sync to complete before returning (polls `mutagen sync list` for "Watching" status)
  - Checks that `mutagen` binary is on PATH, clear error if not
  - Unit tests for command construction, ignore list merging
- Files created:
  - `internal/sync/mutagen.go` -- Mutagen CLI wrapper
  - `internal/sync/mutagen_test.go`
- Size: M
- Blocks: P3.2, P7.1 (sync-wait builds on this), P8.1

**P3.2 -- Claude Code state sync** [M]

Syncs Claude Code state between local and remote so sessions can resume across machines. Two mechanisms: Mutagen session for live bidirectional sync of project state, and one-time copy for global config.

- Acceptance criteria:
  - **Mutagen session 2:** Syncs `~/.claude/projects/<local-path-encoded>/` ↔ `~/.claude/projects/<remote-path-encoded>/` inside container. Path mapping computed from local project path and remote project path. Bidirectional — memories and session logs sync both ways.
  - **One-time copy on `fusebox up`:** Copies the following into the container:
    - `~/.claude/CLAUDE.md` → container `~/.claude/CLAUDE.md`
    - `~/.claude/agents/` → container `~/.claude/agents/`
    - `~/.claude/skills/` → container `~/.claude/skills/`
    - `~/.claude/settings.json` → container `~/.claude/settings.json` (transformed: keep `alwaysThinkingEnabled`, `permissions`, `env`, `skipDangerousModePermissionPrompt`; drop `hooks`, `statusLine`, `feedbackSurveyState`; rewrite hooks to point to remote fusebox binary path)
  - Path encoding matches Claude Code convention: absolute path with `/` replaced by `-` (e.g., `/Users/gk/work/project` → `-Users-gk-work-project`)
  - `claude --continue` works locally after remote session (and vice versa)
  - Unit tests for path encoding, settings.json transformation
- Files created:
  - `internal/sync/claude_state.go` -- path mapping, settings transform, copy logic
  - `internal/sync/claude_state_test.go`
- Size: M
- Blocks: P4.3 (fusebox up orchestrator wires this in)

---

### Phase 4: RPC Tunnel + Local Daemon

**P4.1 -- RPC protocol implementation** [M]

JSON-over-TCP protocol encoder/decoder. Newline-delimited JSON messages. Shared between local daemon and remote CLI.

- Acceptance criteria:
  - `Encoder` writes JSON messages terminated by newline to an `io.Writer`
  - `Decoder` reads newline-delimited JSON messages from an `io.Reader`
  - Message types: `exec` (request to run action), `stdout` (streaming output line), `stderr` (streaming error line), `exit` (action completed with exit code + duration), `actions` (list available actions), `actions_response` (action list), `error` (structured error)
  - Every message includes `secret` field, validated on receive
  - Auth validation: daemon rejects messages with wrong secret
  - Unit tests for roundtrip encode/decode of every message type
  - Unit tests for auth rejection
- Files created:
  - `internal/rpc/protocol.go` -- already has types from P1.2, add encoder/decoder
  - `internal/rpc/codec.go` -- JSON codec (encode/decode over io.Reader/Writer)
  - `internal/rpc/auth.go` -- shared secret generation + validation
  - `internal/rpc/codec_test.go`
  - `internal/rpc/auth_test.go`
- Size: M
- Blocks: P4.2, P5.1

**P4.2 -- Local daemon** [L]

TCP server that listens on the local end of the SSH reverse tunnel. Receives RPC requests, validates against fusebox.yaml, executes whitelisted actions, streams results back.

- Acceptance criteria:
  - Listens on configurable port (default 9600)
  - Accepts TCP connections, authenticates via shared secret
  - Handles `exec` messages: looks up action in fusebox.yaml, validates params, executes command, streams stdout/stderr as individual messages, sends exit message with code + duration
  - Handles `actions` messages: returns list of available actions with descriptions and param schemas
  - Rejects unknown actions with structured error
  - Executes commands with `os/exec`, working directory set to project root
  - Streams stdout/stderr line-by-line in real-time (not buffered)
  - Timeout enforcement: kills process after action timeout (default 600s, per-action override)
  - SIGTERM then SIGKILL after 5s grace period on timeout
  - Logs RPC activity to stderr: `[HH:MM:SS] rpc: <action> started/completed/failed`
  - Concurrent: can handle multiple connections (one action at a time per connection)
  - Exposes status via Unix socket at `~/.fusebox/run/<project>.sock` (JSON blob for claudebar)
  - Unit tests for action lookup, param validation dispatch, timeout handling
- Files created:
  - `internal/daemon/server.go` -- TCP server, connection handler, request router
  - `internal/daemon/executor.go` -- command execution with streaming + timeout
  - `internal/daemon/status.go` -- Unix socket status endpoint
  - `internal/daemon/server_test.go`
  - `internal/daemon/executor_test.go`
- Size: L
- Blocks: P5.1, P6.1

**P4.3 -- `fusebox up` command (orchestrator)** [L]

Ties together container creation, Mutagen sync, SSH reverse tunnel, and local daemon startup. This is the main lifecycle command.

- Acceptance criteria:
  - Loads resolved config (project + global)
  - Checks/creates container on remote (warm start: resumes existing; cold start: creates new)
  - Starts Mutagen sync session (or resumes existing)
  - Waits for initial sync to complete
  - Generates shared secret, writes to `~/.fusebox/run/<project>.secret`
  - Copies shared secret into running container (`docker exec`)
  - Establishes SSH reverse tunnel (remote port -> local daemon port)
  - Starts local daemon in foreground
  - Prints status lines following UX spec: "spotless-1: creating container... ok"
  - Prints "Ready. Connect with: claudebar attach <server>/<project>"
  - Handles warm start path: container exists + sync session exists = fast reconnect
  - Handles interrupts (Ctrl-C): clean shutdown of daemon + tunnel + sync pause
  - Blocks until daemon exits (foreground mode only for Sprint 1)
- Files created:
  - `cmd/fusebox/up.go` -- command implementation
  - `internal/orchestrator/up.go` -- lifecycle orchestration logic
  - `internal/orchestrator/up_test.go` -- unit tests for orchestration state machine
- Size: L
- Blocks: P8.1

**P4.4 -- `fusebox down` command** [S]

Stops sync, tunnel, daemon. Optionally destroys container.

- Acceptance criteria:
  - Stops local daemon (sends signal via PID file or Unix socket)
  - Terminates SSH reverse tunnel
  - Pauses Mutagen sync session (not terminated -- preserves cache for warm restart)
  - Default: leaves container running, prints "Container left running (warm restart available)."
  - `--destroy`: stops + removes container, terminates Mutagen session, prints freed disk space
  - `--force`: kills running actions before stopping
  - Warns if action is currently running (without `--force`)
  - Graceful handling when components are already stopped
- Files created:
  - `cmd/fusebox/down.go` -- command implementation
  - `internal/orchestrator/down.go` -- teardown logic
  - `internal/orchestrator/down_test.go`
- Size: S
- Blocks: P8.1

**P4.5 -- `fusebox status` command** [S]

Shows current project session status.

- Acceptance criteria:
  - Reads status from Unix socket (`~/.fusebox/run/<project>.sock`) if daemon running
  - Falls back to checking container status via SSH if daemon not running
  - Prints: project, server, container state, sync state, RPC state, action count, last RPC
  - "Not in a fusebox project" error when no fusebox.yaml found
  - Machine-readable JSON output with `--json` flag (for claudebar)
- Files created:
  - `cmd/fusebox/status.go` -- command implementation
- Size: S
- Blocks: nothing (can be done in parallel with P4.3)

---

### Phase 5: fusebox exec + Parameter Validation

**P5.1 -- Parameter validation engine** [M]

Validates action parameters against their declared types before execution.

- Acceptance criteria:
  - `regex` type: compiles pattern from config, matches against provided value
  - `enum` type: checks value is in allowed values list
  - `int` type: parses as integer, checks within [min, max] range
  - Template substitution: replaces `{param_name}` in exec string with validated value
  - Rejects unrecognized parameters
  - Requires all declared parameters (no optional params in Sprint 1)
  - Returns structured error on validation failure: which param, what was expected, what was provided
  - Prevents shell injection: validated params are passed as literal strings, not shell-expanded
  - Unit tests for each validation type, edge cases (empty string, boundary values, unicode)
- Files created:
  - `internal/validation/params.go` -- validation engine
  - `internal/validation/template.go` -- parameter substitution in exec strings
  - `internal/validation/params_test.go`
  - `internal/validation/template_test.go`
- Size: M
- Blocks: P4.2 (daemon uses this)

**P5.2 -- `fusebox exec` remote CLI** [M]

CLI command that runs inside the remote container. Sends exec request through RPC tunnel, streams results to stdout/stderr.

- Acceptance criteria:
  - `fusebox exec <action> [--param=value ...]` parses action name and params from CLI args
  - Connects to local daemon via the RPC tunnel (localhost:<tunnel-port>)
  - Reads shared secret from well-known path inside container
  - Sends exec request with action name, params, and secret
  - Streams stdout messages to os.Stdout, stderr messages to os.Stderr in real-time
  - Prints `[local] <expanded command>` before streaming (shows what's being executed)
  - Prints `[exit <code>, <duration>]` after completion
  - Exits with the same exit code as the remote action
  - Handles connection refused: "Error: local machine unreachable"
  - Handles auth failure: "Error: RPC authentication failed"
  - Handles unknown action: "Error: unknown action '<name>'"
  - Handles param validation failure: prints structured error from daemon
- Files created:
  - `cmd/fusebox/exec.go` -- command implementation
  - `internal/rpc/client.go` -- RPC client (connects, sends, receives stream)
  - `internal/rpc/client_test.go`
- Size: M
- Blocks: P6.1, P8.1

**P5.3 -- `fusebox actions` remote CLI** [S]

CLI command that lists available actions by querying the local daemon through the RPC tunnel.

- Acceptance criteria:
  - Connects to local daemon via RPC tunnel
  - Sends actions request with shared secret
  - Prints formatted table: action name, description, params (if any)
  - Matches UX spec format: `build_ios     Build iOS app for simulator`
  - Handles connection refused: "Error: local machine unreachable"
- Files created:
  - `cmd/fusebox/actions.go` -- command implementation
- Size: S
- Blocks: nothing

---

### Phase 6: MCP Server

**P6.1 -- MCP server for fusebox actions** [L]

Remote-side MCP server that exposes fusebox.yaml actions as MCP tools. Runs inside the container alongside Claude Code.

- Acceptance criteria:
  - Implements MCP protocol (stdio transport for Claude Code integration)
  - On startup, queries local daemon via RPC tunnel to get available actions
  - Registers each action as an MCP tool named `fusebox_<action_name>`
  - Tool description comes from action description in fusebox.yaml
  - Tool input schema generated from action params (regex -> string with pattern, enum -> string with enum, int -> integer with min/max)
  - Tool invocation: sends exec request through RPC tunnel, collects full output, returns as tool result
  - Returns structured error for param validation failures, unreachable daemon, etc.
  - Tool result includes: output text, exit_code, duration_ms
  - Refreshes action list when daemon reconnects (handles local daemon restart)
  - Started automatically inside container during `fusebox up`
  - Claude Code discovers it via `.claude/mcp.json` config written into container
- Files created:
  - `internal/mcp/server.go` -- MCP server implementation (stdio transport)
  - `internal/mcp/tools.go` -- action-to-tool mapping, schema generation
  - `internal/mcp/server_test.go`
  - `internal/mcp/tools_test.go`
- Size: L
- Blocks: P8.1

---

### Phase 7: Sync-Wait Mechanism

**P7.1 -- Mutagen gRPC state polling** [M]

Queries Mutagen daemon via gRPC over Unix socket to determine sync state. Used by both remote exec (wait for outbound clear) and local daemon (wait for inbound landed).

- Acceptance criteria:
  - Connects to Mutagen daemon via `~/.mutagen/daemon.sock` (local) or equivalent path (remote)
  - Queries session status for `fusebox-<project>` session
  - Returns sync state: syncing, watching (idle), error, paused
  - `WaitForWatching(timeout)` blocks until state is "Watching" or timeout
  - Default timeout: 30 seconds (configurable)
  - Returns error with current state if timeout exceeded
  - Unit tests with mocked gRPC responses
- Files created:
  - `internal/sync/state.go` -- Mutagen gRPC client for state queries
  - `internal/sync/wait.go` -- WaitForWatching with polling loop
  - `internal/sync/state_test.go`
- Size: M
- Blocks: P7.2

**P7.2 -- Integrate sync-wait into exec flow** [S]

Wire sync-wait into both sides of the exec path.

- Acceptance criteria:
  - Remote side (`fusebox exec`): before sending exec request, calls WaitForWatching on remote Mutagen to ensure local changes have been pushed
  - Local side (daemon): before executing action, calls WaitForWatching on local Mutagen to ensure remote changes have landed
  - Both sides print debug line when waiting: `[sync] waiting for sync to complete...`
  - Both sides print when sync confirmed: `[sync] synced, proceeding`
  - If timeout, exec proceeds with warning: `[sync] warning: sync not confirmed after 30s, proceeding anyway`
  - Integration test: modify file remotely, exec build, verify sync completes before build starts
- Files modified:
  - `internal/rpc/client.go` -- add pre-exec sync wait
  - `internal/daemon/executor.go` -- add pre-exec sync wait
  - `internal/rpc/client_test.go`
  - `internal/daemon/executor_test.go`
- Size: S
- Blocks: P8.1

---

### Phase 8: Integration Testing + Demo

**P8.1 -- End-to-end integration test** [L]

Full lifecycle test: up, sync, exec, actions, down. Requires a real remote server.

- Acceptance criteria:
  - Test tagged `//go:build integration` (skipped in normal `go test`)
  - Uses SSH to a test server (configured via env vars: `FUSEBOX_TEST_HOST`, `FUSEBOX_TEST_USER`)
  - Full lifecycle: init (programmatic) -> up -> sync files -> exec action -> verify output -> actions list -> down -> verify cleanup
  - Tests warm restart: down (no destroy) -> up -> verify fast reconnect
  - Tests destroy: down --destroy -> verify container removed
  - Tests param validation: exec with invalid param -> verify rejection
  - Tests sync-wait: modify file -> exec immediately -> verify sync completes first
  - Tests auth: exec with wrong secret -> verify rejection
  - Tests offline local daemon: kill daemon -> exec from remote -> verify "unreachable" error
  - Test fixtures: minimal Go project with a `fusebox.yaml` that has a `build` action running `go build ./...`
- Files created:
  - `test/integration/lifecycle_test.go`
  - `test/integration/exec_test.go`
  - `test/integration/sync_test.go`
  - `test/integration/auth_test.go`
  - `test/fixtures/go-project/` -- minimal Go project for testing
  - `test/fixtures/go-project/fusebox.yaml`
  - `test/fixtures/go-project/main.go`
- Size: L
- Blocks: P8.2

**P8.2 -- Cross-compile + demo prep** [S]

Build both binaries, verify on real hardware, prep demo script.

- Acceptance criteria:
  - `make build` produces macOS (arm64 + amd64) local binary
  - `make build-remote` produces Linux (amd64) remote binary
  - Both binaries are statically linked (CGO_ENABLED=0)
  - Smoke test: full lifecycle on real server (spotless-1)
  - Demo script documented: exact commands for the 60-second video
  - README quickstart section written
- Files created/modified:
  - `Makefile` -- finalized build targets
  - `README.md` -- quickstart section
- Size: S
- Blocks: nothing (final task)

---

## Dependency Graph

```
P1.1 (scaffold)
 |
 +--> P1.2 (types) --+--> P1.3 (resolver)
 |                    |
 |                    +--> P4.1 (protocol) --+--> P4.2 (daemon) --+
 |                    |                      |                     |
 |                    +--> P5.1 (validation) +                     |
 |                                                                 |
 +--> P2.1 (SSH) --+--> P2.2 (container) -------------+-----------+
 |                                                     |
 +--> P3.1 (mutagen) --+--> P3.2 (claude state sync) -+
 |                                                     |
 +--> P2.3 (example configs)                           |
                                                       |
                                        +--> P4.3 (up) <---------+
                                        |    P4.4 (down)
                                        |    P4.5 (status)
                                        |
                                        +--> P5.2 (exec CLI) --> P6.1 (MCP)
                                        |    P5.3 (actions CLI)
                                        |
                                        +--> P7.1 (sync state) --> P7.2 (sync-wait integration)
                                        |
                                        +--> P8.1 (integration tests) --> P8.2 (demo)
```

## Critical Path

P1.1 -> P1.2 -> P2.1 -> P2.2 -> P4.1 -> P4.2 -> P4.3 -> P5.2 -> P8.1 -> P8.2

Parallelizable work off the critical path:
- P2.3 (example configs) can be done anytime
- P3.1 (mutagen) can start after P1.1, runs in parallel with Phase 2
- P3.2 (claude state sync) can start after P3.1
- P5.1 (validation) can start after P1.2, runs in parallel with Phase 2
- P4.4, P4.5 can be done in parallel with P4.3
- P5.3 can be done in parallel with P5.2
- P7.1 can start after P3.1, runs in parallel with Phase 4-5
- P6.1 (MCP) can start after P5.2

## Story Point Totals

| Size | Count |
|------|-------|
| S | 6 |
| M | 10 |
| L | 4 |
| **Total** | **20 tasks** |

---

## File Tree (projected)

```
fusebox/
  cmd/fusebox/
    main.go
    root.go
    up.go
    down.go
    exec.go
    actions.go
    status.go
  internal/
    config/
      project.go        # fusebox.yaml types + parsing
      global.go         # ~/.fusebox/config types + parsing
      validation.go     # YAML validation
      resolver.go       # config resolution (CLI > project > global)
      config_test.go
      resolver_test.go
    ssh/
      client.go         # SSH command execution + SCP
      tunnel.go         # SSH reverse tunnel management
      client_test.go
    container/
      manager.go        # container create/start/stop/remove/status
      dockerfile.go     # Dockerfile generation
      manager_test.go
    sync/
      mutagen.go        # Mutagen CLI wrapper
      state.go          # Mutagen gRPC state polling
      wait.go           # WaitForWatching
      mutagen_test.go
      state_test.go
    rpc/
      protocol.go       # message type definitions
      codec.go          # JSON encode/decode over stream
      auth.go           # shared secret generation + validation
      client.go         # RPC client (remote side)
      codec_test.go
      auth_test.go
      client_test.go
    daemon/
      server.go         # TCP server + request router
      executor.go       # command execution with streaming
      status.go         # Unix socket status endpoint
      server_test.go
      executor_test.go
    validation/
      params.go         # parameter validation (regex/enum/int)
      template.go       # exec string template substitution
      params_test.go
      template_test.go
    mcp/
      server.go         # MCP stdio server
      tools.go          # action-to-tool mapping
      server_test.go
      tools_test.go
    orchestrator/
      up.go             # fusebox up lifecycle
      down.go           # fusebox down teardown
      up_test.go
      down_test.go
  build/
    Dockerfile.remote   # container image for remote side
  test/
    integration/
      lifecycle_test.go
      exec_test.go
      sync_test.go
      auth_test.go
    fixtures/
      go-project/
        fusebox.yaml
        main.go
  testdata/
    valid.fusebox.yaml
    invalid.fusebox.yaml
  fusebox.example.yaml
  config.example.yaml
  go.mod
  go.sum
  Makefile
  README.md
```

---

## Progress Tracking

| Task | Status | Notes |
|------|--------|-------|
| P1.1 Go module + CLI skeleton | Complete | |
| P1.2 Core types + config parsing | Complete | |
| P1.3 Project resolver | Complete | |
| P2.1 SSH client wrapper | Complete | |
| P2.2 Container lifecycle | Complete | |
| P2.3 Example config files | Complete | |
| P2.4 `fusebox auth` | Complete | |
| P3.1 Mutagen session manager | Complete | |
| P3.2 Claude Code state sync | Complete | |
| P4.1 RPC protocol | Complete | |
| P4.2 Local daemon | Complete | |
| P4.3 `fusebox up` orchestrator | Complete | |
| P4.4 `fusebox down` | Complete | |
| P4.5 `fusebox status` | Complete | |
| P5.1 Parameter validation | Complete | |
| P5.2 `fusebox exec` remote CLI | Complete | |
| P5.3 `fusebox actions` remote CLI | Complete | |
| P6.1 MCP server | Complete | |
| P7.1 Mutagen gRPC state polling | Complete | |
| P7.2 Sync-wait integration | Complete | |
| P8.1 Integration tests | Complete | |
| P8.2 Cross-compile + demo | Complete | |
