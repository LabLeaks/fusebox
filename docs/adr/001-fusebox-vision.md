# ADR-001: Fusebox — One-Click Remote Agent Isolation

## Status

Accepted (directional)

## Context

Running Claude Code in "yolo mode" (`--dangerously-skip-permissions`) is powerful but risky on your local machine. An agent with unrestricted tool access can delete files, run destructive commands, or corrupt your working tree. The standard mitigations — sandboxing, containers, VMs — add friction and break the seamless coding experience.

Meanwhile, persistent sessions are essential for agentic work. SSH disconnects, laptop sleep, and walking away from your desk all kill running agents. tmux on a remote server solves this, but setting it up manually is tedious.

The original concept was **fusebox.cc**: a product that uses userspace FUSE to mount a bidirectional mirror between your local filesystem and a remote server. The agent runs on the remote side in a tmux session, edits files through the FUSE mount, and changes sync back to your local machine. You get isolation (agent can't `rm -rf ~` on your Mac), persistence (tmux survives disconnects), and a seamless local editing experience (your IDE sees the same files).

## Decision

Build `work` as the spiritual successor to fusebox.cc. Replace literal FUSE with mutagen (or equivalent bidirectional sync). The core value proposition is unchanged:

**Point `work` at a server. It installs itself, syncs your code, and gives you safe, persistent, phone-controllable Claude sessions.**

### Architecture

```
Your Mac                              Remote Server
┌──────────────────┐                  ┌──────────────────┐
│ ~/projects/foo/  │ ◄── mutagen ──► │ ~/projects/foo/  │
│ ~/projects/bar/  │    (real-time)   │ ~/projects/bar/  │
├──────────────────┤                  ├──────────────────┤
│ work (TUI)       │ ── SSH ────────► │ work (server)    │
│                  │ ◄── JSON ─────── │                  │
└──────────────────┘                  │ tmux sessions:   │
                                      │  foo: claude     │
                                      │  bar: claude     │
                                      └──────────────────┘
```

### The "one-click" flow

```
$ work init user@myserver.example.com
```

1. **Connect** — SSH to server, verify access
2. **Deploy** — SCP the `work` binary to `~/bin/work`
3. **Discover** — List directories on the server (or offer to create `~/projects/`)
4. **Pick roots** — User selects which directories to browse for sessions
5. **Sync setup** — Install mutagen if needed, create sync sessions for selected roots
6. **Hooks** — Install Claude Code PostToolUse hook for activity tracking
7. **Config** — Write `~/.config/work-cli/config.yaml` with all settings
8. **Launch** — Open the TUI dashboard

After init, `work` just works. `work new` creates a session + sync. `work kill` stops the session. The dashboard shows per-session sync status.

### Why mutagen (not FUSE)

| | FUSE | Mutagen |
|---|---|---|
| Latency | Every file op is a network round-trip | Batched, async — local-speed reads |
| Offline | Broken | Works, syncs on reconnect |
| Setup | Kernel module / macFUSE (signing issues) | Single binary, auto-deploys agent |
| IDE compat | Flaky (fsevents, indexing) | Perfect (real local files) |
| Conflict handling | Last-write-wins | Three-way merge, configurable |

Mutagen gives us the same bidirectional sync with none of the FUSE pain. The agent binary auto-deploys to remotes — same pattern as `work` itself.

### Why not containers / devcontainers

Containers add a layer of abstraction that fights the "just SSH to a server" simplicity. You need Docker on the remote, orchestration, volume mounts, port forwarding. For the use case of "run Claude on my code safely," a bare tmux session with file sync is simpler, faster, and more debuggable.

### Isolation model

The remote server IS the sandbox. The agent can do whatever it wants there — install packages, run tests, modify files. Changes sync back to your Mac through mutagen, which provides:

- **Conflict detection** — if both sides change the same file, mutagen flags it
- **Ignore rules** — `.git`, `node_modules`, build artifacts stay local to each side
- **Pause/resume** — stop sync during large refactors, resume when ready

If an agent goes rogue, the blast radius is one remote server. Your local machine is untouched until sync propagates, and you can pause sync instantly.

### Managed sync lifecycle

Today mutagen is external — user installs and configures it separately. The goal is for `work` to own the sync lifecycle:

| Action | Today | Goal |
|---|---|---|
| Install mutagen | Manual | `work init` handles it |
| Create sync session | `mutagen sync create ...` | `work new` does it automatically |
| Monitor sync | `mutagen sync list` | Dashboard shows per-session status |
| Pause sync | `mutagen sync pause ...` | `work pause <session>` |
| Teardown | `mutagen sync terminate ...` | `work kill` cleans up |

## What exists today

- `work` binary — unified TUI dashboard + server CLI + JSON server commands
- Session management — create, stop, attach, preview via tmux
- Activity monitoring — PostToolUse hook tracks what Claude is doing per-session
- Deploy — `make deploy` SCPs binary to server, installs hooks
- Mutagen status — dashboard shows global sync status (when mutagen is installed)

## What's next

1. **`work init` wizard** — interactive TUI for first-time setup (immediate next step)
2. **Managed mutagen sessions** — `work new` creates sync + tmux session together
3. **Per-session sync status** — dashboard shows sync state per session, not just global
4. **`work pause/resume`** — control sync per session
5. **Auto-install mutagen** — detect platform, download, install during `work init`

## Consequences

- `work` becomes opinionated about mutagen as the sync layer (but still works without it)
- The setup flow must handle mutagen installation across macOS and Linux
- Sync configuration (ignores, conflict resolution) needs sensible defaults
- The binary grows in scope from "session manager" to "remote development environment manager"
