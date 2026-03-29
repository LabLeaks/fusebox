# PRD-001: Fusebox

**Status:** Active
**Author:** GK
**Created:** 2026-03-28
**Last updated:** 2026-03-28

---

## One-liner

Fusebox is the local execution bridge for remote AI coding agents. Your cloud AI agent gets physical hands on your local machine.

**Tagline:** "Fuse the boxes."

---

## Problem

AI coding agents (Claude Code, Aider, OpenHands, Cline) need to be sandboxed. But sandboxing them locally creates a paradox:

1. **Sandbox locally (Docker/bwrap)** -- agent loses access to Xcode, local GPUs, USB hardware, VPN-bound services. The tools that matter most are outside the sandbox.
2. **Run fully remote (Devcontainers)** -- cloud containers can't run xcodebuild, iOS SDK, CoreML on Apple Silicon. Can't reach local USB devices, local GPUs, or node-locked software (Unity, Unreal, SolidWorks).
3. **Local sandbox with holes (bwrap/seatbelt)** -- must punch so many holes for real dev work that the sandbox is meaningless. macOS sandbox-exec is deprecated and brittle. Agent still competes for local CPU/RAM.
4. **Run unsandboxed locally** -- agent's continuous background loops (LSP, vector DB, testing) compete with your IDE/compiler. Close laptop lid, agent dies mid-task.

No existing tool resolves all four.

---

## Solution

AI agent runs sandboxed on a cheap remote Linux server. Local machine runs native tools at bare-metal speed. A whitelisted RPC bridge connects them securely.

The developer defines what the agent can trigger locally in `fusebox.yaml`. The agent discovers and invokes those capabilities. Everything else stays remote.

---

## Architecture

### Lifecycle

```
fusebox up    -->  SSH into BYO Linux server
              -->  Provision container (Debian + Claude Code + mosh-server + fusebox binary)
              -->  Start Mutagen source-only sync
              -->  Establish encrypted RPC backchannel

user connects -->  mosh (via claudebar) for low-latency interactive terminal

agent needs   -->  fusebox exec <action> (CLI) or MCP tool call
local tool    -->  travels through RPC tunnel
              -->  local daemon validates against fusebox.yaml
              -->  executes, streams stdout/stderr back

local offline -->  agent gets immediate "local unavailable" error

fusebox down  -->  stops Mutagen, tears down container
```

### Components

| Component | Role |
|-----------|------|
| `fusebox up/down` | Lifecycle management: SSH, container, sync, RPC |
| `fusebox exec` | Remote-side CLI for triggering local actions |
| Local daemon | Runs on dev machine, validates and executes whitelisted actions |
| RPC backchannel | Authenticated encrypted tunnel (separate from mosh session) |
| `fusebox.yaml` | Local-only action whitelist (never synced to remote) |
| Mutagen sidecar | Source-only file sync (not embedded, treated as infra dependency) |

---

## Key Design Decisions

These are locked. Not up for debate.

| Decision | Rationale |
|----------|-----------|
| **BYO server** | Fusebox doesn't provision cloud infra. User brings their own Linux box. Keeps scope tight, avoids billing/cloud-provider lock-in. |
| **Sysbox containers** | Each project gets a `--runtime=sysbox-runc` Docker container that behaves like a lightweight VM. Agent can run Docker, docker-compose, systemd natively inside — no socket mounting, no DinD hacks, no KVM required. Verified working on Debian 13 (kernel 6.12) and Ubuntu 24.04 (kernel 6.8). `fusebox setup` installs Docker + Sysbox on the remote server. |
| **Mutagen as sidecar** | Not embedded. Source-only sync (ignores node_modules, build artifacts, DerivedData, target/, .build/). Mutagen is infrastructure, not a feature. |
| **Git is orthogonal** | Mutagen handles sync, git stays purely for version control. No synthetic commits, no git-as-transport. |
| **No built-in TUI** | Fusebox is infrastructure. claudebar (tmux wrapper for Claude Code) is the UX layer. Clean separation. |
| **fusebox.yaml is LOCAL ONLY** | Defines whitelisted actions the local daemon will execute. Never synced to remote — agent could modify it. Agent discovers capabilities via `fusebox actions` CLI and MCP tools on the remote side, which query the local daemon through the RPC tunnel. |
| **Claude Code first, agent-agnostic RPC** | Zero-click setup is opinionated for Claude Code. The exec CLI and MCP layer work with any agent. |
| **Container per project** | Multiple fusebox sessions on one server, each in isolated Sysbox containers. |
| **Auth token management** | `fusebox up` sends a long-lived Claude session token (from `claude setup-token` flow). Fusebox wraps and stores a reusable token in `~/.fusebox/config`. Injected into container as env var. |
| **RPC transport** | JSON protocol over SSH reverse tunnel. SSH handles encryption and auth. Simple message types: exec, stdout, stderr, exit, actions. Easy to debug and extend, no codegen. MCP server on remote is a thin wrapper over the same protocol. |
| **RPC auth** | Backchannel authenticated via shared secret established during `fusebox up`. Without this, fusebox is RCE-as-a-service. |
| **Sync-wait mechanism** | Dual-ended Mutagen state polling via gRPC over Unix socket (`~/.mutagen/daemon.sock`). Remote side: `fusebox exec` blocks until outbound sync queue is clear. Local side: daemon blocks execution until inbound sync has landed. Both check for "Watching" status. Prevents stale-code builds from RPC/sync race condition. |
| **Mosh for user terminal** | SSH is too laggy for interactive remote terminal. Mosh is mandatory for the user session. RPC backchannel is separate (non-interactive, latency tolerance is fine). |
| **Parameterized actions** | fusebox.yaml actions support typed parameters (regex, enum, int range) validated before execution. Prevents injection attacks while giving the agent flexibility to pass arguments. |

---

## fusebox.yaml

Local-only configuration file. Defines what the remote agent is allowed to trigger on the local machine.

```yaml
version: 1
sync:
  ignore:
    - "DerivedData/"
    - "target/"
    - "node_modules/"
    - ".build/"

actions:
  build_ios:
    description: "Build iOS app for simulator"
    exec: "xcodebuild -workspace App.xcworkspace -scheme App -sdk iphonesimulator"

  run_tests:
    description: "Run test suite"
    exec: "xcodebuild test -scheme App -only-testing:AppTests/{test_class}"
    params:
      test_class:
        type: regex
        pattern: "^[a-zA-Z0-9_]+Tests$"

  train_local:
    description: "Run training on local GPU"
    exec: "python train.py --epochs {epochs}"
    params:
      epochs:
        type: int
        range: [1, 1000]
```

Key properties:
- **Actions are whitelisted** -- the agent can only run what's defined here
- **Parameters are validated** -- type-checked and constrained (regex, range) before execution
- **Descriptions are agent-facing** -- the agent uses these to understand what each action does
- **Never leaves the local machine** -- not synced, not version-controlled with the project

---

## Use Cases

### 1. iOS / React Native
Agent writes code remotely. Triggers Xcode builds on local Mac. Gets compiler output back. No cloud Mac needed.

### 2. Local GPU training
Agent on $5/mo VPS writes PyTorch scripts. Triggers training on local RTX 5090. Gets metrics back. Cost arbitrage vs cloud GPUs ($3/hr for A100 vs $0 for hardware you already own).

### 3. Hardware-in-the-loop
Agent writes firmware. Triggers flash to ESP32/Arduino plugged into local USB. Gets serial output back.

### 4. VPN-bound services
Agent writes migrations. Triggers execution against database only accessible via corporate VPN. No VPN tunneling through the remote server.

### 5. Game engines
Agent writes Unity/Unreal scripts. Triggers local GPU-accelerated builds via node-locked licenses that can't run in the cloud.

### 6. Apple Silicon ML
Agent exports CoreML models. Benchmarks on local M-series Neural Engine. Results stream back.

### 7. Pre-authed local CLI tools (zero credential sprawl)
Agent triggers `gh pr create`, `aws deploy`, `gcloud run deploy`, `kubectl apply`, `op read` — all using your local machine's existing authenticated sessions. Credentials never leave your machine. No API keys, service accounts, or tokens need to exist on the remote server. The agent sends structured intents; your local daemon executes with your ambient auth. This eliminates credential sprawl and reduces the blast radius of a compromised remote container to zero for sensitive operations.

Examples: `gh` (GitHub), `aws`/`gcloud`/`az` (cloud CLIs), `op` (1Password), `xcrun`/`codesign` (Apple developer identity on Keychain), `kubectl` (cluster access via local kubeconfig), `terraform` (state backends behind local auth).

---

## Target Users

Solo developers and small teams using Claude Code for real work who:

- Need persistent AI sessions that survive laptop close
- Work on projects requiring platform-specific tools (Xcode, CUDA, hardware)
- Want cheap remote compute for the agent without paying cloud GPU prices
- Use multiple devices (laptop to phone handoff via Claude app)

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Cold start (`fusebox up` to working session) | < 5 minutes |
| Warm start (container exists) | < 60 seconds |
| GitHub stars (6 months post-launch) | 1,000 |
| Community fusebox.yaml templates | 3+ ecosystem-specific templates contributed |
| AI tool community presence | Featured in Aider, OpenHands, Cline discords |

---

## Open Source Strategy

- **License:** MIT
- **Dogfood first** -- build as personal tool, use daily, stabilize through real usage
- **README leads with the demo** -- 60-second split-screen video: agent thinks remotely, local Xcode builds automatically
- **Position against Devcontainers explicitly** -- "Devcontainers can't reach your Xcode. Fusebox can."
- **Community growth:** solve real issues in AI agent communities, don't spam. Contribute fusebox.yaml templates. File issues upstream when agents break.
- **Launch:** "Show HN" with ruthlessly technical positioning. No marketing fluff. Show the architecture, show the latency numbers, show the security model.

---

## Non-goals

- Cloud infrastructure provisioning (user brings their own server)
- Built-in TUI or session management UI (that's claudebar's job)
- Replacing git workflows (git stays orthogonal)
- Supporting Windows as a local host (macOS and Linux first)
- Embedding Mutagen (it's a sidecar dependency)

---

## Security Model

1. **fusebox.yaml is local-only** -- the remote agent cannot modify what it's allowed to execute
2. **RPC backchannel is authenticated** -- shared secret established during `fusebox up`, required for every exec call
3. **Parameter validation** -- all action parameters are type-checked and constrained before execution
4. **No ambient authority** -- the agent can only invoke named actions, not arbitrary shell commands
5. **Container isolation** -- each project gets its own container on the remote server
6. **Local daemon is the trust boundary** -- it validates every request against the whitelist before executing anything

---

## Milestones

Tracked in sprint docs under `_project/sprints/`. This PRD defines the product vision. Sprint docs define what ships when.
