# Fusebox

Local execution bridge for remote AI coding agents. Agent runs sandboxed on cheap remote Linux server, local machine runs native tools at bare-metal speed, whitelisted RPC bridge connects them via `fusebox.yaml`.

## Runtime

- **Go** — single binary, cross-compiled for macOS (arm64/amd64) and Linux (amd64).
- No CGO (`CGO_ENABLED=0`) — static binaries.
- `make build` for local, `make build-remote` for Linux remote binary, `make all` for both.

## Architecture

See `_project/prds/prd-001-fusebox.md` for the full PRD.

- `cmd/fusebox/` — CLI entry points (cobra subcommands: up, down, exec, actions, status, auth, mcp-serve)
- `internal/config/` — fusebox.yaml + ~/.fusebox/config parsing, validation, resolution
- `internal/ssh/` — SSH client (golang.org/x/crypto/ssh), reverse tunnel, SCP
- `internal/container/` — Sysbox container lifecycle via SSH (create/start/stop/remove/port allocation)
- `internal/sync/` — Mutagen CLI wrapper, Claude state sync (path-mapped), sync-wait polling
- `internal/rpc/` — JSON-over-TCP protocol, codec, auth, client
- `internal/daemon/` — local TCP server, action executor with streaming, status socket
- `internal/validation/` — parameter validation (regex/enum/int), shell-safe template substitution
- `internal/mcp/` — MCP stdio server (JSON-RPC 2.0) exposing actions as Claude Code tools
- `internal/orchestrator/` — `fusebox up`/`down` lifecycle orchestration

## Key Design Decisions (locked)

- **Sysbox containers** — `--runtime=sysbox-runc`. Docker/docker-compose work natively inside. No KVM needed.
- **Mutagen as sidecar** — not embedded. Source-only sync. fusebox.yaml always ignored.
- **Git is orthogonal** — Mutagen syncs, git is for version control only.
- **fusebox.yaml is LOCAL ONLY** — never synced. Agent discovers capabilities via `fusebox actions` or MCP.
- **JSON over SSH reverse tunnel** — RPC transport. Messages: exec, stdout, stderr, exit, actions, error.
- **Dual-ended sync-wait** — Mutagen CLI polling before exec on both sides.
- **Parameterized actions** — regex, enum, int range validation before execution.

## Agent Team

Each role has a skill that defines how to think, communicate, and what NOT to do. Spawn prompts should mention the role skill by name to trigger loading.

| Role | Role Skill | Tool Skills Used |
|------|-----------|-----------------|
| Team Lead | `/team-lead` | `/write-prd`, `/write-sprint-doc` |
| Architect | `/agent-team-architect` | `/write-adr` |
| Researcher | `/agent-team-researcher` | `/web-extract` |
| Developer | `/agent-team-developer` | `/simplify` |
| Code Reviewer | `/agent-team-code-reviewer` | `/code-review` |
| Test Engineer | `/agent-team-test-engineer` | `/test-audit` |
| QA Tester | `/agent-team-qa` | `/qa-verify` |
| Docs Maintainer | `/agent-team-docs-maintainer` | `/documentation-sync` |

Flow: Architect + Test Engineer → Developer → Code Reviewer → QA Tester → Docs Maintainer.

### Coordination

**Agents use `SendMessage` to talk directly to teammates by name.** Team lead dispatches and unblocks — does NOT relay messages between agents who can talk directly. Agents discover teammates via `~/.claude/teams/<team-name>/config.json`.

Examples:
- Dev → Reviewer: "internal/rpc ready for review. Changed codec.go, auth.go. Watch the streaming flow."
- Reviewer → Dev: "killTimer not cancelled at executor.go:60. Fix?"
- Dev → Test-eng: "Timeout behavior changed — SIGTERM then SIGKILL after 5s now."
- Dev-A → Dev-B: "Bug #33 is in up.go:93 which you own. Remove redundant EnsureImage()."

### Process Rules

1. **One agent per file at a time.** No concurrent edits to the same file.
2. **No task dispatched twice.** Team lead tracks dispatched set.
3. **`go build && go test` between parallel waves.** Clean state before next phase.
4. **Review gate before Complete.** Different agent must approve. No self-review.
5. **Never skip process steps.** If the process says 7 roles, use 7 roles. Change the process if it's too heavy — don't silently skip.
6. **Parallelize at package boundary, never within.** Same-file tasks serialize.
7. **File ownership is explicit.** Every agent prompt: "You own X. Do not modify files outside."
8. **NEVER give time estimates.** Story points (S/M/L) only.

## Conventions

- Strict Go. `go vet`, `go test ./...` must pass.
- Errors returned, not panicked. Wrap: `fmt.Errorf("context: %w", err)`.
- No global state. Pass dependencies explicitly.
- Test files next to source: `foo.go` → `foo_test.go`.
- Integration tests tagged `//go:build integration`.
- Test server: `spotless-2` (Debian 13, kernel 6.12, Sysbox installed).

## Config

- `fusebox.yaml` — per-project action whitelist (local-only, never synced). See `fusebox.example.yaml`.
- `~/.fusebox/config` — global server/auth config. See `config.example.yaml`.

## Known Constraints

- Sysbox requires Docker on the remote server (not Podman-compatible).
- Mutagen must be installed locally (`brew install mutagen`).
- Mutagen licensing is MIT/SSPL — fine for local tool, blocker for hosted SaaS.
- SSH key-based auth required (no password auth).
