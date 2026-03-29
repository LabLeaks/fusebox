package container

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lableaks/fusebox/internal/ssh"
)

const (
	containerPrefix = "fusebox-"
	imageName       = "fusebox-remote"
	portsFile       = "~/.fusebox/ports.json"
	basePort        = 60001
)

// ContainerState represents the current state of a container.
type ContainerState string

const (
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
	StateNotFound ContainerState = "not-found"
	StateCrashed  ContainerState = "crashed"
)

// ContainerStatus holds the runtime status of a container.
type ContainerStatus struct {
	State    ContainerState
	Uptime   string
	ExitCode int
}

// PortMap tracks project-to-port assignments on the remote server.
type PortMap map[string]int

// remoteRunner abstracts command execution on a remote host (for testing).
type remoteRunner interface {
	RunCommand(cmd string) (stdout string, stderr string, exitCode int, err error)
}

// Manager manages Sysbox container lifecycle on a remote server via SSH.
type Manager struct {
	ssh remoteRunner
}

// NewManager creates a Manager that runs docker commands over the given SSH client.
func NewManager(client *ssh.Client) *Manager {
	return &Manager{ssh: client}
}

// newManagerWithRunner creates a Manager with a custom remoteRunner (for testing).
func newManagerWithRunner(r remoteRunner) *Manager {
	return &Manager{ssh: r}
}

// ContainerName returns the docker container name for a project.
func ContainerName(project string) string {
	return containerPrefix + project
}

// Create builds the image if needed and runs a new container for the project.
func (m *Manager) Create(projectName, token string, moshPort int) error {
	if err := m.EnsureImage(); err != nil {
		return fmt.Errorf("ensuring image: %w", err)
	}

	name := ContainerName(projectName)
	runCmd := fmt.Sprintf(
		"docker run --runtime=sysbox-runc -d --name %s -p %d:60001/udp %s",
		name, moshPort, imageName,
	)

	_, stderr, exitCode, err := m.ssh.RunCommand(runCmd)
	if err != nil {
		return fmt.Errorf("running container: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("docker run failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}

	// Inject OAuth token via file inside the container to avoid exposing
	// it in the process listing (docker run -e is visible via /proc).
	if err := m.InjectToken(name, token); err != nil {
		// Best-effort cleanup: remove the container we just started.
		m.ssh.RunCommand(fmt.Sprintf("docker rm -f %s", name))
		return fmt.Errorf("injecting token: %w", err)
	}

	return nil
}

// InjectToken writes the OAuth token to a file inside the container so that
// Claude Code can read it without the token appearing in process arguments.
func (m *Manager) InjectToken(containerName, token string) error {
	injectCmd := fmt.Sprintf(
		"docker exec %s bash -c %s",
		containerName,
		shellQuote("mkdir -p /root/.fusebox && echo -n "+shellQuote(token)+" > /root/.fusebox/token && chmod 600 /root/.fusebox/token"),
	)

	_, stderr, exitCode, err := m.ssh.RunCommand(injectCmd)
	if err != nil {
		return fmt.Errorf("exec into container: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("token injection failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return nil
}

// Start starts a stopped container.
func (m *Manager) Start(projectName string) error {
	name := ContainerName(projectName)
	_, stderr, exitCode, err := m.ssh.RunCommand(fmt.Sprintf("docker start %s", name))
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("docker start failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return nil
}

// Stop stops a running container.
func (m *Manager) Stop(projectName string) error {
	name := ContainerName(projectName)
	_, stderr, exitCode, err := m.ssh.RunCommand(fmt.Sprintf("docker stop %s", name))
	if err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("docker stop failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return nil
}

// Remove removes a container.
func (m *Manager) Remove(projectName string) error {
	name := ContainerName(projectName)
	_, stderr, exitCode, err := m.ssh.RunCommand(fmt.Sprintf("docker rm -f %s", name))
	if err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("docker rm failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return nil
}

// Status returns the current state of a container.
func (m *Manager) Status(projectName string) (ContainerStatus, error) {
	name := ContainerName(projectName)
	stdout, _, exitCode, err := m.ssh.RunCommand(
		fmt.Sprintf("docker inspect --format '{{.State.Status}}|{{.State.ExitCode}}|{{.State.StartedAt}}' %s", name),
	)
	if err != nil {
		return ContainerStatus{}, fmt.Errorf("inspecting container: %w", err)
	}
	if exitCode != 0 {
		return ContainerStatus{State: StateNotFound}, nil
	}

	return ParseInspectOutput(strings.TrimSpace(stdout)), nil
}

// ParseInspectOutput parses docker inspect formatted output into a ContainerStatus.
func ParseInspectOutput(output string) ContainerStatus {
	parts := strings.SplitN(output, "|", 3)
	if len(parts) < 3 {
		return ContainerStatus{State: StateNotFound}
	}

	status := parts[0]
	exitCode := 0
	fmt.Sscanf(parts[1], "%d", &exitCode)
	uptime := parts[2]

	switch status {
	case "running":
		return ContainerStatus{State: StateRunning, Uptime: uptime}
	case "exited":
		if exitCode != 0 {
			return ContainerStatus{State: StateCrashed, ExitCode: exitCode}
		}
		return ContainerStatus{State: StateStopped, ExitCode: exitCode}
	default:
		return ContainerStatus{State: StateStopped}
	}
}

// EnsureImage checks if the fusebox image exists on the remote, builds it if not.
func (m *Manager) EnsureImage() error {
	_, _, exitCode, err := m.ssh.RunCommand(fmt.Sprintf("docker image inspect %s", imageName))
	if err != nil {
		return fmt.Errorf("checking image: %w", err)
	}
	if exitCode == 0 {
		return nil
	}

	dockerfile := GenerateDockerfile()
	buildCmd := fmt.Sprintf("echo %s | docker build -t %s -f - .",
		shellQuote(dockerfile), imageName)

	_, stderr, exitCode, err := m.ssh.RunCommand(buildCmd)
	if err != nil {
		return fmt.Errorf("building image: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("docker build failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}

	return nil
}

// AllocatePort reads the port state file on the server and assigns a deterministic
// port for the project, starting at 60001.
func (m *Manager) AllocatePort(projectName string) (int, error) {
	ports, err := m.readPorts()
	if err != nil {
		return 0, err
	}

	if port, ok := ports[projectName]; ok {
		return port, nil
	}

	port := nextAvailablePort(ports)
	ports[projectName] = port

	if err := m.writePorts(ports); err != nil {
		return 0, err
	}

	return port, nil
}

func (m *Manager) readPorts() (PortMap, error) {
	stdout, _, exitCode, err := m.ssh.RunCommand(fmt.Sprintf("cat %s 2>/dev/null", portsFile))
	if err != nil {
		return nil, fmt.Errorf("reading ports file: %w", err)
	}
	if exitCode != 0 || strings.TrimSpace(stdout) == "" {
		return make(PortMap), nil
	}

	var ports PortMap
	if err := json.Unmarshal([]byte(stdout), &ports); err != nil {
		return nil, fmt.Errorf("parsing ports file: %w", err)
	}
	return ports, nil
}

func (m *Manager) writePorts(ports PortMap) error {
	data, err := json.Marshal(ports)
	if err != nil {
		return fmt.Errorf("marshaling ports: %w", err)
	}

	cmd := fmt.Sprintf("mkdir -p ~/.fusebox && echo %s > %s", shellQuote(string(data)), portsFile)
	_, stderr, exitCode, err := m.ssh.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("writing ports file: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("writing ports file failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return nil
}

// NextAvailablePort finds the lowest available port starting at basePort.
func nextAvailablePort(ports PortMap) int {
	used := make(map[int]bool, len(ports))
	for _, p := range ports {
		used[p] = true
	}
	for port := basePort; ; port++ {
		if !used[port] {
			return port
		}
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
