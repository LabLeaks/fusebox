# Handoff: Sprint 1 → Sprint 2

## What Exists

Fusebox is a local execution bridge for remote AI coding agents. Agent runs sandboxed on a cheap Linux server, local machine runs native tools (Xcode, GPUs, hardware, pre-authed CLIs) at bare-metal speed, connected by a whitelisted RPC bridge defined in `fusebox.yaml`.

### Code
- 57 Go source files, ~10K lines, 258+ tests, all passing
- Both binaries build: macOS arm64 (`make build`) + Linux amd64 (`make build-remote`)
- Repo: `github.com/LabLeaks/fusebox`, branch `main`

### CLI Commands Implemented
- `fusebox up` — provisions Sysbox container, Mutagen sync, SSH reverse tunnel, local daemon
- `fusebox down` — soft stop (warm restart) or `--destroy`
- `fusebox exec <action>` — trigger whitelisted local action from remote agent
- `fusebox actions` — list available actions
- `fusebox status` — session status with `--json`
- `fusebox auth` — wraps `claude setup-token`, stores in `~/.fusebox/config`
- `fusebox mcp-serve` — MCP stdio server exposing actions as Claude Code tools

### Architecture (all locked, see PRD)
- Sysbox containers (Docker-inside-Docker works, no KVM needed)
- Mutagen sidecar for source sync + Claude state sync (path-mapped)
- JSON-over-SSH-reverse-tunnel RPC
- Dual-ended sync-wait (Mutagen CLI polling before exec)
- Parameterized actions with regex/enum/int validation
- `fusebox.yaml` is local-only, never synced

### Key Files
- `CLAUDE.md` — conventions, team roles, process rules
- `_project/prds/prd-001-fusebox.md` — full PRD
- `_project/prds/synthesis.md` — competitive landscape, use case ranking, launch strategy
- `_project/sprints/sprint-001-mvp-bridge.md` — sprint plan + retro (both iterations)
- `fusebox.example.yaml` — example project config
- `config.example.yaml` — example global config

## What Has NOT Been Tested

None of this has been run against a real server. The full loop is untested:
1. `fusebox up` against spotless-2
2. Mosh into the container
3. Claude Code runs `fusebox exec` inside the container
4. Local tool executes, results stream back
5. `fusebox down` tears it down cleanly

The integration tests use a local daemon over TCP — no SSH, no containers, no Mutagen.

## What Sprint 2 Should Cover

### Smoke test (first)
Actually run `fusebox up` against spotless-2. See what breaks. Fix it. This will surface real integration issues that unit tests can't catch.

### Docker test harness
Local Docker-based test harness so container lifecycle, orchestrator, and init/setup can be tested without a remote server. Sysbox-specific testing deferred to sprint 3.

### New commands
- `fusebox init` — interactive setup wizard (server config, auth, generate fusebox.yaml)
- `fusebox setup` — install Docker + Sysbox on remote server

### Architecture debt (from retro)
- `orchestrator/up.go` — refactor 325-line procedural blob into state machine with rollback
- Unify duplicate type hierarchies (`rpc.ActionInfo` vs `mcp.ActionDescriptor`)
- Add tunnel reconnection logic (SSH drop = everything dies currently)
- Replace Mutagen CLI string parsing with `--template` for structured output
- Add `context.Context` throughout for cancellation support

## Process Learnings (codified in CLAUDE.md + skills)

### Three-phase pipeline
```
Phase 1: Dev + Test Engineer (parallel, strict file ownership)
Phase 2: Code Review (different agent, mandatory)
Phase 3: QA (uses the product, brings receipts)
```

### Role skills (in ~/.claude/skills/)
Every team role has a skill that defines how to think, communicate, and what NOT to do. Spawn prompts must mention the role skill by name to trigger loading.

### Key rules
1. One agent per file at a time
2. No task dispatched twice
3. Build check between parallel waves
4. Review gate before complete (no self-review)
5. Test engineer treats implementation as BLACK BOX
6. QA is product verification, not test execution
7. Agents use `SendMessage` for peer communication

## Test Server
- **spotless-2**: Debian 13 trixie, kernel 6.12, Docker + Sysbox installed and verified
- SSH: `spotless@spotless-2`
