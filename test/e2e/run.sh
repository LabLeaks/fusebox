#!/usr/bin/env bash
# End-to-end integration test for fusebox sync + server commands.
# Spins up two Docker containers (client + server) connected via SSH,
# then runs the full workflow and verifies everything works.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
NETWORK="fusebox-e2e"
SERVER="fusebox-server"
CLIENT="fusebox-client"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

pass=0
fail=0

log()  { echo -e "${BOLD}=== $1${NC}"; }
ok()   { echo -e "  ${GREEN}✓ $1${NC}"; ((pass++)); }
fail() { echo -e "  ${RED}✗ $1${NC}"; ((fail++)); }
warn() { echo -e "  ${YELLOW}! $1${NC}"; }

cleanup() {
    log "Cleanup"
    docker rm -f "$SERVER" "$CLIENT" 2>/dev/null || true
    docker network rm "$NETWORK" 2>/dev/null || true
    # Keep .scratch for faster re-runs (mutagen binary cached)
}
trap cleanup EXIT

# ── Build ──────────────────────────────────────────────────────────────────────

log "Building fusebox binary (linux/arm64)"
cd "$PROJECT_DIR"
SCRATCH="$SCRIPT_DIR/.scratch"
mkdir -p "$SCRATCH"
GOOS=linux GOARCH=arm64 go build -o "$SCRATCH/fusebox" .

log "Downloading mutagen"
MUTAGEN_VER="0.18.1"
MUTAGEN_TAR="$SCRATCH/mutagen.tar.gz"
if [ ! -f "$SCRATCH/mutagen" ]; then
    curl -fsSL "https://github.com/mutagen-io/mutagen/releases/download/v${MUTAGEN_VER}/mutagen_linux_arm64_v${MUTAGEN_VER}.tar.gz" \
        -o "$MUTAGEN_TAR"
    tar xzf "$MUTAGEN_TAR" -C "$SCRATCH" 2>/dev/null || true
    chmod +x "$SCRATCH/mutagen"
    rm -f "$MUTAGEN_TAR"
fi

log "Generating SSH keypair"
rm -f "$SCRATCH/id_ed25519" "$SCRATCH/id_ed25519.pub"
ssh-keygen -t ed25519 -f "$SCRATCH/id_ed25519" -N "" -q

log "Building Docker images"
docker build -q -t fusebox-e2e-server -f "$SCRIPT_DIR/Dockerfile.server" "$SCRIPT_DIR"
docker build -q -t fusebox-e2e-client -f "$SCRIPT_DIR/Dockerfile.client" "$SCRIPT_DIR"

# ── Launch ─────────────────────────────────────────────────────────────────────

log "Creating network + containers"
docker network create "$NETWORK" 2>/dev/null || true

docker run -d --name "$SERVER" \
    --network "$NETWORK" \
    --hostname server \
    fusebox-e2e-server >/dev/null

docker run -d --name "$CLIENT" \
    --network "$NETWORK" \
    --hostname client \
    fusebox-e2e-client \
    sleep infinity >/dev/null

# Copy files into containers
docker cp "$SCRATCH/fusebox" "$SERVER:/tmp/fusebox"
docker cp "$SCRATCH/id_ed25519.pub" "$SERVER:/tmp/authorized_keys"
docker exec "$SERVER" bash -c "
    cp /tmp/fusebox /home/testuser/bin/fusebox
    chown testuser:testuser /home/testuser/bin/fusebox
    chmod 755 /home/testuser/bin/fusebox
    cp /tmp/authorized_keys /home/testuser/.ssh/authorized_keys
    chown testuser:testuser /home/testuser/.ssh/authorized_keys
    chmod 600 /home/testuser/.ssh/authorized_keys
    chmod 700 /home/testuser/.ssh
"

docker cp "$SCRATCH/fusebox" "$CLIENT:/tmp/fusebox"
docker cp "$SCRATCH/id_ed25519" "$CLIENT:/tmp/id_ed25519"
docker cp "$SCRATCH/mutagen" "$CLIENT:/tmp/mutagen"
test -f "$SCRATCH/mutagen-agents.tar.gz" && \
    docker cp "$SCRATCH/mutagen-agents.tar.gz" "$CLIENT:/tmp/mutagen-agents.tar.gz" || true
docker exec -u root "$CLIENT" bash -c "
    cp /tmp/fusebox /home/testuser/bin/fusebox
    chown testuser:testuser /home/testuser/bin/fusebox
    chmod 755 /home/testuser/bin/fusebox
    cp /tmp/id_ed25519 /home/testuser/.ssh/id_ed25519
    chown testuser:testuser /home/testuser/.ssh/id_ed25519
    chmod 600 /home/testuser/.ssh/id_ed25519
    chmod 700 /home/testuser/.ssh
    mkdir -p /home/testuser/.fusebox/bin
    cp /tmp/mutagen /home/testuser/.fusebox/bin/mutagen
    chown -R testuser:testuser /home/testuser/.fusebox
    chmod 755 /home/testuser/.fusebox/bin/mutagen
    test -f /tmp/mutagen-agents.tar.gz && \
        cp /tmp/mutagen-agents.tar.gz /home/testuser/.fusebox/bin/mutagen-agents.tar.gz && \
        chown testuser:testuser /home/testuser/.fusebox/bin/mutagen-agents.tar.gz || true
"

# SSH config on client
docker exec -u testuser "$CLIENT" bash -c '
    cat > ~/.ssh/config << EOF
Host server
    HostName server
    User testuser
    IdentityFile ~/.ssh/id_ed25519
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
EOF
    chmod 600 ~/.ssh/config
'

# Fusebox config on client
docker exec -u testuser "$CLIENT" bash -c '
    mkdir -p ~/.config/fusebox
    cat > ~/.config/fusebox/config.yaml << EOF
server:
  host: server
  user: testuser
  home_dir: /home/testuser
browse_roots:
  - ~/projects
claude:
  flags: "--dangerously-skip-permissions --remote-control"
EOF
'

# Start mutagen daemon on client
docker exec -u testuser "$CLIENT" /home/testuser/.fusebox/bin/mutagen daemon start 2>&1 || true
sleep 1

# Wait for SSH
log "Waiting for SSH"
for i in $(seq 1 10); do
    if docker exec -u testuser "$CLIENT" ssh server echo ok 2>/dev/null; then
        break
    fi
    sleep 1
done

# ── Tests ──────────────────────────────────────────────────────────────────────
set +e

run_client() { docker exec -u testuser "$CLIENT" env HOME=/home/testuser "$@" 2>&1; }
run_server() { docker exec -u testuser "$SERVER" env HOME=/home/testuser "$@" 2>&1; }

# -- SSH connectivity
log "Test: SSH connectivity"
result=$(run_client ssh server echo hello)
if [[ "$result" == *"hello"* ]]; then ok "client can SSH to server"
else fail "SSH failed: $result"; fi

# -- Server binary
log "Test: Server binary"
result=$(run_server /home/testuser/bin/fusebox list)
if [[ "$result" == "[]" ]]; then ok "fusebox list returns empty array"
else fail "fusebox list unexpected: $result"; fi

# -- Sync add
log "Test: Sync add"
run_client bash -c "mkdir -p /home/testuser/projects/test-project && echo 'hello from client' > /home/testuser/projects/test-project/README.md"
result=$(run_client /home/testuser/bin/fusebox sync add /home/testuser/projects/test-project)
if [[ "$result" == *"Syncing"* ]]; then ok "fusebox sync add succeeded"
else fail "fusebox sync add failed: $result"; fi

# -- Sync ls
log "Test: Sync list"
sleep 1
result=$(run_client /home/testuser/bin/fusebox sync ls)
if [[ "$result" == *"fusebox-test-project"* ]]; then ok "sync session visible in list"
else fail "sync session not found: $result"; fi

# -- File synced to server
log "Test: File sync client → server"
# Wait for mutagen to sync (agent install on server can take a few seconds)
result=""
for i in $(seq 1 30); do
    result=$(run_server cat /home/testuser/.fusebox/sync/test-project/README.md 2>/dev/null)
    if [[ "$result" == *"hello from client"* ]]; then break; fi
    sleep 1
done
if [[ "$result" == *"hello from client"* ]]; then ok "file synced from client to server"
else
    # Show sync status for debugging
    warn "mutagen status: $(run_client /home/testuser/.fusebox/bin/mutagen sync list 2>&1 | grep -E 'Status|Error' | head -3)"
    fail "file not synced: $result"
fi

# -- Reverse sync
log "Test: File sync server → client"
run_server bash -c "mkdir -p /home/testuser/.fusebox/sync/test-project && echo 'hello from server' > /home/testuser/.fusebox/sync/test-project/from-server.txt"
for i in $(seq 1 15); do
    result=$(run_client cat /home/testuser/projects/test-project/from-server.txt 2>/dev/null)
    if [[ "$result" == *"hello from server"* ]]; then break; fi
    sleep 1
done
if [[ "$result" == *"hello from server"* ]]; then ok "file synced from server to client"
else fail "reverse sync failed: $result"; fi

# -- Server binary responds to help
log "Test: Server help"
result=$(run_server /home/testuser/bin/fusebox help)
if [[ "$result" == *"Claude Code session manager"* ]]; then ok "fusebox help works"
else fail "fusebox help unexpected: $result"; fi

# -- Remote list via SSH
log "Test: Remote operations via SSH"
result=$(run_client ssh server /home/testuser/bin/fusebox list)
if [[ "$result" == "[]" ]]; then ok "remote fusebox list via SSH"
else fail "remote fusebox list unexpected: $result"; fi

# -- Remote dirs via SSH
run_server bash -c "echo '/home/testuser/projects' > ~/.config/fusebox/roots.conf"
run_server bash -c "mkdir -p ~/projects/myapp ~/projects/mylib"
result=$(run_client ssh server /home/testuser/bin/fusebox dirs)
if [[ "$result" == *"projects"* ]]; then ok "remote dirs via SSH"
else fail "remote dirs unexpected: $result"; fi

# -- Sync rm
log "Test: Sync remove"
result=$(run_client /home/testuser/bin/fusebox sync rm /home/testuser/projects/test-project)
if [[ "$result" == *"Stopped"* ]]; then ok "fusebox sync rm succeeded"
else fail "fusebox sync rm failed: $result"; fi

result=$(run_client /home/testuser/bin/fusebox sync ls)
if [[ "$result" == *"No active"* ]] || [[ ! "$result" == *"fusebox-test-project"* ]]; then
    ok "sync session removed from list"
else fail "sync session still present: $result"; fi

# -- Tmux sessions
log "Test: Tmux sessions"
run_server bash -c "tmux new-session -d -s test-session -c /home/testuser/projects 'bash'"
result=$(run_server /home/testuser/bin/fusebox list)
if [[ "$result" == *"test-session"* ]]; then ok "tmux session visible via fusebox list"
else fail "tmux session not found: $result"; fi

# Preview
run_server bash -c "tmux send-keys -t test-session 'echo hello-from-session' Enter"
sleep 1
result=$(run_server /home/testuser/bin/fusebox preview test-session 10)
if [[ "$result" == *"hello-from-session"* ]]; then ok "session preview contains output"
else fail "preview missing content: $result"; fi

# Stop
result=$(run_server /home/testuser/bin/fusebox stop test-session)
if [[ "$result" == *"ok"* ]]; then ok "session stopped"
else fail "session stop failed: $result"; fi

result=$(run_server /home/testuser/bin/fusebox list)
if [[ "$result" == "[]" ]]; then ok "session gone after stop"
else fail "session still present: $result"; fi

# -- Local mode: dirs and sessions work without SSH
log "Test: Local mode"
# Write a local-mode config (no server block)
run_client bash -c '
    cat > ~/.config/fusebox/config.yaml << EOF
browse_roots:
  - /home/testuser/projects
claude:
  flags: "--dangerously-skip-permissions --remote-control"
EOF
    echo "/home/testuser/projects" > ~/.config/fusebox/roots.conf
'
result=$(run_client /home/testuser/bin/fusebox list)
if [[ "$result" == "[]" ]]; then ok "local mode: fusebox list works"
else fail "local mode: fusebox list unexpected: $result"; fi

result=$(run_client /home/testuser/bin/fusebox dirs)
if [[ "$result" == *"projects"* ]]; then ok "local mode: fusebox dirs works"
else fail "local mode: fusebox dirs unexpected: $result"; fi

# ── Summary ────────────────────────────────────────────────────────────────────

echo ""
log "Results: ${GREEN}${pass} passed${NC}, ${RED}${fail} failed${NC}"
echo ""

if [ "$fail" -gt 0 ]; then
    exit 1
fi
