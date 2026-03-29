# Fusebox

Local execution bridge for remote AI coding agents. Agent runs sandboxed on cheap remote Linux server, local machine runs native tools at bare-metal speed, whitelisted RPC bridge connects them via `fusebox.yaml`.

## Runtime

- **Go** — single binary, cross-compiled for macOS (arm64/amd64) and Linux (amd64).
- No CGO (`CGO_ENABLED=0`) — static binaries.
- `make build` for local, `make build-remote` for Linux remote binary, `make all` for both.

## Architecture

See `_project/prds/prd-001-fusebox.md` for the full PRD.

- `cmd/fusebox/` — CLI entry points (cobra subcommands)
- `internal/config/` — fusebox.yaml + ~/.fusebox/config parsing, validation, resolution
- `internal/ssh/` — SSH client wrapper, reverse tunnel management
- `internal/provision/` — Docker + Sysbox installation on remote server
- `internal/container/` — Sysbox container lifecycle (create/start/stop/remove)
- `internal/sync/` — Mutagen CLI wrapper + gRPC state polling for sync-wait
- `internal/rpc/` — JSON-over-TCP protocol, codec, auth, client
- `internal/daemon/` — local TCP server, action executor, status socket
- `internal/validation/` — parameter validation (regex/enum/int), template substitution
- `internal/mcp/` — MCP stdio server exposing actions as tools
- `internal/orchestrator/` — `fusebox up`/`down` lifecycle orchestration

## Key Design Decisions (locked)

- **Sysbox containers** — each project gets `--runtime=sysbox-runc`. Docker/docker-compose work natively inside. No KVM needed.
- **Mutagen as sidecar** — not embedded. Source-only sync. fusebox.yaml always in ignore list.
- **Git is orthogonal** — Mutagen syncs, git is for version control only. No synthetic commits.
- **fusebox.yaml is LOCAL ONLY** — never synced to remote. Agent discovers capabilities via `fusebox actions` CLI or MCP tools.
- **JSON over SSH reverse tunnel** — RPC transport. Simple message types: exec, stdout, stderr, exit, actions, error.
- **Dual-ended sync-wait** — Mutagen gRPC polling before exec on both sides.
- **Parameterized actions** — regex, enum, int range validation before execution.

## Agent Team

Sprint execution uses parallel Claude subagents with these roles:

- **Architect** — Designs the approach before code is written. Reviews task scope, identifies affected files, flags risks, proposes the implementation plan. Runs first on each task.
- **Researcher** — Investigates unknowns: Mutagen gRPC API, Sysbox integration, MCP stdio protocol, SSH tunneling in Go. Feeds findings to the architect or developer.
- **Developer** — Writes the code. Follows the architect's plan and existing patterns.
- **Code Reviewer** — Reviews diffs for correctness, style, edge cases, and adherence to the architect's plan. Runs after the developer.
- **Test Engineer** — Writes tests for new and changed code. Covers happy path, edge cases, and regressions.
- **QA Tester** — Runs `go build` and `go test`, verifies the change works end-to-end. Reports pass/fail with exact output.
- **Docs Maintainer** — Updates CLAUDE.md, sprint docs, and README after changes land. Ensures docs match reality.

Typical flow: Architect + Test Engineer in parallel → Developer (+ Researcher as needed) → Code Reviewer → QA Tester → Docs Maintainer.

**Testing rule:** Test engineer runs IN PARALLEL with the developer, not after. Tests are written against the public interface and spec, not implementation internals. Tests should still pass if the implementation is refactored. Never treat testing as an afterthought.

## Conventions

- Strict Go. `go vet`, `go test ./...` must pass.
- Errors returned, not panicked. Wrap with `fmt.Errorf("context: %w", err)`.
- No global state. Pass dependencies explicitly.
- Test files next to source: `foo.go` → `foo_test.go`.
- Integration tests tagged `//go:build integration`, skipped in normal `go test`.
- Test server: `spotless-2` (Debian 13, kernel 6.12, Sysbox installed).
- **NEVER give time estimates.** Use story points (S/M/L) to convey relative effort. Time predictions are always wrong with agentic dev.

## Config Files

- `fusebox.yaml` — project-level action whitelist (local-only, never synced). See `fusebox.example.yaml`.
- `~/.fusebox/config` — global server/auth config. See `config.example.yaml`.
- `cmd/fusebox/auth.go` has a local `globalConfig` struct for token storage. This should be replaced with `internal/config.GlobalConfig` when aligning types.

## Known Constraints

- Sysbox requires Docker on the remote server (not Podman-compatible).
- Mutagen must be installed locally (`brew install mutagen`).
- Mutagen licensing is MIT/SSPL — fine for local tool, would be a blocker for hosted SaaS.
- SSH key-based auth required for remote server (no password auth).
- `fusebox status` daemon socket path (`~/.fusebox/run/<project>.sock`) is stubbed — will report "daemon not running" until P4.2 local daemon implements the socket.
