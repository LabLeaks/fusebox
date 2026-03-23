# ADR-002: Sandbox Is Not Optional — Remote vs Local Mode

## Status

Accepted

## Context

Fusebox's original sandbox implementation (Linux namespaces + OverlayFS + Alpine rootfs) was exposed as an optional toggle — users could run with or without it. This is wrong. The sandbox is core to fusebox's value proposition on remote servers: you deploy to bare metal and get isolation for free, without needing to set up Docker or manage containers.

Meanwhile, there's a separate use case: running fusebox locally on your Mac for persistent session management. Local mode needs access to system packages (Xcode, Homebrew, simulators) because that's the whole point — you can't do iOS dev inside a namespace sandbox. But you still want the dashboard, session persistence, and baseline safety.

Claude Code ships with built-in OS-level sandboxing (Seatbelt on macOS, bubblewrap on Linux) that restricts filesystem writes and network access. This is configurable via `settings.json`.

## Decision

### Two modes, two isolation strategies

**Remote mode** (deploying to a server):
- Linux namespace sandbox is **always on**. No toggle, no opt-out.
- `fusebox create` runs sessions inside the namespace sandbox automatically.
- Clean Alpine rootfs via OverlayFS — throwaway filesystem, can't damage the host.
- Synced files (via mutagen) are bind-mounted in. Everything else is disposable.
- `up`/`down`/`sandbox-status` become internal lifecycle — not user-facing commands. Fusebox manages the sandbox transparently.

**Local mode** (running on your Mac):
- No namespace sandbox. Sessions run directly on the host.
- Claude Code's built-in Seatbelt sandbox provides baseline filesystem/network isolation.
- Fusebox configures sandbox settings via `settings.json` during `fusebox init`.
- System packages, toolchains, and SDKs are accessible by design — this is "leaky" isolation because the value is using the full local environment.
- The value proposition is persistent tmux sessions + TUI dashboard + activity monitoring, not hard isolation.

### Why two strategies

| | Remote | Local |
|---|---|---|
| Goal | Protect the server from Claude | Persistent sessions on your own machine |
| Isolation | Hard — namespace + OverlayFS | OS-level — Seatbelt filesystem/network |
| Permissions | `--dangerously-skip-permissions` | `--dangerously-skip-permissions` |
| System access | None (clean rootfs) | Full (Xcode, brew, etc.) |
| Sync | Mutagen (bidirectional) | Not needed (files are local) |
| Persistence | tmux on server | tmux locally |

### What changes

1. Remove sandbox enable/disable toggle from config and init wizard
2. Remote mode: sandbox starts automatically with `fusebox create`, stops with `fusebox kill`
3. Local mode: `fusebox init --local` (or auto-detect) skips SSH/deploy, configures Claude Code sandbox settings, sets up local tmux management
4. `fusebox up`/`down`/`sandbox-status` removed from user-facing help and README — sandbox lifecycle is internal
5. `fusebox init` detects mode based on whether a remote host is provided

## Consequences

- Simpler mental model: remote = hard sandbox, local = soft sandbox + session management
- No more "should I enable sandbox?" decision for users
- Local mode enables iOS/mobile dev workflows that namespace sandboxing would break
- Fusebox manages more lifecycle internally, reducing commands users need to know
- Claude Code's sandbox config becomes a dependency for local mode — need to track upstream changes
