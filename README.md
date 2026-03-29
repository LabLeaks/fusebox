# Fusebox

**Give remote AI agents access to Xcode, local GPUs, and hardware.**

Your AI coding agent runs sandboxed on a cheap remote Linux server. Your local machine runs native tools at bare-metal speed. A whitelisted RPC bridge defined in `fusebox.yaml` connects them. Fuse the boxes.

> Demo video coming soon.

---

## The Problem

AI coding agents need to be sandboxed. But every sandboxing approach has a fatal flaw:

**Trap 1: Sandbox locally.** Agent loses access to Xcode, local GPUs, USB hardware, VPN-bound services, and node-locked software. The tools that matter most are outside the sandbox.

**Trap 2: Run fully remote.** Cloud containers can't run `xcodebuild`, can't reach your GPU, can't flash your ESP32, can't access databases behind your VPN. Linux containers are Linux-bound by definition.

**Trap 3: Run unsandboxed locally.** Agent's continuous background loops (LSP, vector DB, testing) compete with your IDE and compiler for CPU and RAM. Close your laptop lid and the agent dies mid-task.

No existing tool resolves all three.

---

## The Solution

The agent runs remote. Your tools run local. Fusebox bridges them.

You define exactly what the agent can trigger in `fusebox.yaml`:

```yaml
version: 1
sync:
  ignore:
    - "DerivedData/"
    - "node_modules/"

actions:
  build_ios:
    description: "Build iOS app for simulator"
    exec: "xcodebuild -workspace App.xcworkspace -scheme {scheme} -sdk iphonesimulator"
    params:
      scheme:
        type: enum
        values: ["App", "AppTests"]

  deploy:
    description: "Deploy via GitHub CLI using local auth"
    exec: "gh pr create --title '{title}' --base {base}"
    params:
      title:
        type: regex
        pattern: "^.{1,120}$"
      base:
        type: regex
        pattern: "^[a-zA-Z0-9_.\\-/]+$"

  train:
    description: "Run PyTorch training on local GPU"
    exec: "python train.py --epochs {epochs} --lr {lr}"
    params:
      epochs:
        type: int
        range: [1, 1000]
      lr:
        type: regex
        pattern: "^0\\.[0-9]{1,6}$"
```

The agent discovers actions via `fusebox actions` or MCP tools. It sends structured intents like `fusebox exec build_ios --scheme=App`. The local daemon validates parameters, executes the command, and streams results back. Everything else stays remote.

### Architecture

```
+---------------------------+          SSH tunnel          +---------------------------+
|     Remote Server ($5)    |  <========================>  |     Your Local Machine    |
|                           |                              |                           |
|  +---------------------+ |    RPC (JSON over tunnel)    |  +---------------------+ |
|  |  Sysbox Container   | |  <-------------------------> |  |   Fusebox Daemon    | |
|  |                     | |                              |  |                     | |
|  |  Claude Code        | |    Mutagen (file sync)       |  |  fusebox.yaml       | |
|  |  fusebox exec       | |  <-------------------------> |  |  (action whitelist)  | |
|  |  fusebox actions    | |                              |  |                     | |
|  |  MCP server         | |                              |  |  xcodebuild         | |
|  +---------------------+ |                              |  |  python train.py    | |
+---------------------------+                              |  |  gh, aws, kubectl   | |
                                                           |  |  esptool.py         | |
                                                           |  +---------------------+ |
                                                           +---------------------------+
```

---

## Use Cases

### Pre-authenticated CLI tools (zero credential sprawl)

Agent triggers `gh pr create`, `aws deploy`, `gcloud run deploy`, `kubectl apply` -- all using your local machine's existing authenticated sessions. Credentials never leave your machine. No API keys, service accounts, or tokens on the remote server. The agent sends structured intents; your local daemon executes with your ambient auth.

### iOS / React Native

Agent writes code on a remote Linux server. Triggers Xcode builds on your Mac. Gets compiler output back. No cloud Mac needed. `xcodebuild` physically cannot run on Linux -- Fusebox bridges the gap.

### Local GPU training

Agent thinks on a $5/mo VPS. Your RTX 5090 does the actual training. Zero cloud GPU bills. The agent writes training scripts remotely, triggers `python train.py` locally, and streams metrics back.

### Hardware-in-the-loop

Agent writes firmware remotely. Triggers flash to ESP32/Arduino plugged into your USB port. Gets serial output back. You cannot flash a USB device from a cloud container.

### VPN-bound services

Agent writes database migrations. Triggers execution against your corporate database only accessible via VPN. No VPN tunneling through the remote server.

### Game engines

Agent writes Unity/Unreal scripts. Triggers builds using node-locked licenses and local GPU acceleration.

---

## Security Model

1. **`fusebox.yaml` is local-only.** It never leaves your machine. The agent cannot modify its own permissions.
2. **Parameter validation.** Every parameter is type-checked (regex, enum, int range) and validated before execution. Shell injection is prevented by design.
3. **No ambient authority.** The agent can only invoke named actions defined in `fusebox.yaml`, not arbitrary shell commands.
4. **RPC is authenticated.** A shared secret generated during `fusebox up` is required for every request. Without it, Fusebox is inert.
5. **Container isolation.** Each project gets its own Sysbox container on the remote server. Docker and docker-compose work natively inside.
6. **The local daemon is the trust boundary.** It validates every request against the whitelist before executing anything.

---

## Quickstart

### Prerequisites

- macOS or Linux (local machine)
- A remote Linux server with SSH access (BYO -- any VPS works)
  - Docker + [Sysbox](https://github.com/nestybox/sysbox) installed
- [Mutagen](https://mutagen.io/) installed locally (`brew install mutagen`)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed locally

### 1. Authenticate

```bash
fusebox auth
```

This runs `claude setup-token` and stores the token in `~/.fusebox/config`.

### 2. Configure your server

Copy and edit the global config:

```bash
mkdir -p ~/.fusebox
cp config.example.yaml ~/.fusebox/config
```

Set your server's hostname, SSH user, and port.

### 3. Define your actions

Copy the example config to your project root:

```bash
cp fusebox.example.yaml fusebox.yaml
```

Edit `fusebox.yaml` to whitelist the actions your agent needs. See `fusebox.example.yaml` for commented examples covering iOS, GPU training, CLI tools, and embedded development.

### 4. Start a session

```bash
fusebox up
```

This provisions a Sysbox container on your server, starts file sync via Mutagen, establishes the RPC tunnel, and starts the local daemon.

### 5. Use it

From the remote container (via Claude Code or manually):

```bash
# List available actions
fusebox actions

# Trigger a local action
fusebox exec build_ios --scheme=App

# Check session status
fusebox status
```

### 6. Tear down

```bash
fusebox down             # pause sync, keep container (warm restart)
fusebox down --destroy   # remove container + sync session
```

---

## Architecture

| Component | Role |
|-----------|------|
| `fusebox up` / `down` | Lifecycle: SSH, container, Mutagen sync, RPC tunnel, local daemon |
| `fusebox exec` | Remote-side CLI for triggering local actions |
| `fusebox actions` | Remote-side CLI for listing available actions |
| Local daemon | Runs on your machine, validates and executes whitelisted actions |
| RPC backchannel | JSON protocol over SSH reverse tunnel |
| `fusebox.yaml` | Local-only action whitelist (never synced to remote) |
| MCP server | Exposes actions as MCP tools for Claude Code |
| Mutagen sidecar | Source-only file sync between local and remote |

---

## Requirements

| Requirement | Detail |
|-------------|--------|
| Local OS | macOS (arm64, amd64) or Linux (amd64) |
| Remote OS | Linux with Docker + Sysbox (Debian 13+ or Ubuntu 24.04+ recommended) |
| Sync | [Mutagen](https://mutagen.io/) (`brew install mutagen`) |
| AI agent | [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (first-class support). CLI exec works with any agent. |
| SSH | Key-based auth to remote server (no password auth) |

---

## Building from Source

```bash
make build          # local binary (macOS/Linux)
make build-remote   # cross-compile for Linux amd64 (remote server)
make all            # both
make test           # run tests
```

Binaries are statically linked (`CGO_ENABLED=0`).

---

## Status

Under active development. Not yet released.

---

## License

MIT
