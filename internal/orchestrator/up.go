package orchestrator

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/container"
	"github.com/lableaks/fusebox/internal/daemon"
	"github.com/lableaks/fusebox/internal/rpc"
	"github.com/lableaks/fusebox/internal/ssh"
	fusesync "github.com/lableaks/fusebox/internal/sync"
)

const (
	rpcPort         = 9600
	syncTimeout     = 120 * time.Second
	remoteBinPath   = "/usr/local/bin/fusebox"
	remoteProjectFmt = "/root/%s/"
)

// UpConfig holds inputs for the Up orchestrator.
type UpConfig struct {
	ResolveOpts   config.ResolveOptions
	FuseboxBinary string // path to fusebox-remote binary for docker cp
	Log           io.Writer
}

// Up runs the full fusebox up lifecycle: provision, sync, tunnel, daemon.
// It blocks until interrupted (Ctrl-C) or an error occurs.
func Up(ucfg UpConfig) error {
	w := ucfg.Log
	if w == nil {
		w = os.Stdout
	}

	// 1. Resolve config
	cfg, err := config.Resolve(ucfg.ResolveOpts)
	if err != nil {
		return fmt.Errorf("resolving config: %w", err)
	}

	projectName := filepath.Base(cfg.ProjectRoot)

	serverHost := cfg.Server.Host
	serverUser := cfg.Server.User
	serverPort := cfg.Server.Port

	fmt.Fprintf(w, "%s: connecting...\n", serverHost)

	// 2. SSH connect
	sshOpts := []ssh.ConnectOption{}
	if serverPort != 0 {
		sshOpts = append(sshOpts, ssh.WithPort(serverPort))
	}
	sshClient, err := ssh.Connect(serverHost, serverUser, sshOpts...)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer sshClient.Close()

	fmt.Fprintf(w, "%s: connected\n", serverHost)

	cm := container.NewManager(sshClient)

	// Check if container already exists and is running (warm start)
	status, err := cm.Status(projectName)
	if err != nil {
		return fmt.Errorf("checking container status: %w", err)
	}

	warmStart := status.State == container.StateRunning

	var moshPort int

	if warmStart {
		fmt.Fprintf(w, "%s: container already running, warm start\n", serverHost)
		// Allocate port even on warm start (will return existing allocation)
		moshPort, err = cm.AllocatePort(projectName)
		if err != nil {
			return fmt.Errorf("allocating port: %w", err)
		}
	} else {
		// 3. Allocate port
		moshPort, err = cm.AllocatePort(projectName)
		if err != nil {
			return fmt.Errorf("allocating port: %w", err)
		}

		// 5. Create or start container
		if status.State == container.StateStopped || status.State == container.StateCrashed {
			fmt.Fprintf(w, "%s: starting container... ", serverHost)
			if err := cm.Start(projectName); err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("starting container: %w", err)
			}
			fmt.Fprintf(w, "ok\n")
		} else {
			fmt.Fprintf(w, "%s: creating container (port %d)... ", serverHost, moshPort)
			if err := cm.Create(projectName, cfg.Token, moshPort); err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("creating container: %w", err)
			}
			fmt.Fprintf(w, "ok\n")
		}

		// 5. Copy fusebox binary into container
		if ucfg.FuseboxBinary != "" {
			fmt.Fprintf(w, "%s: copying fusebox binary... ", serverHost)

			// SCP binary to remote server temp path
			const remoteTmpBin = "/tmp/fusebox-remote"
			if err := sshClient.CopyFile(ucfg.FuseboxBinary, remoteTmpBin); err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("uploading binary to server: %w", err)
			}

			// docker cp from remote temp path into container
			containerName := container.ContainerName(projectName)
			cpCmd := fmt.Sprintf("docker cp %s %s:%s", remoteTmpBin, containerName, remoteBinPath)
			_, stderr, exitCode, err := sshClient.RunCommand(cpCmd)
			if err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("copying binary into container: %w", err)
			}
			if exitCode != 0 {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("docker cp failed (exit %d): %s", exitCode, stderr)
			}

			// Clean up temp file
			_, _, _, _ = sshClient.RunCommand("rm " + remoteTmpBin)

			fmt.Fprintf(w, "ok\n")
		}
	}

	// 7. Mutagen source sync
	mutagen, err := fusesync.NewMutagenManager()
	if err != nil {
		return fmt.Errorf("initializing mutagen: %w", err)
	}

	srcSession := fusesync.SrcSessionName(projectName)
	remotePath := fmt.Sprintf(remoteProjectFmt, projectName)

	if warmStart {
		fmt.Fprintf(w, "%s: resuming source sync... ", serverHost)
		if err := mutagen.Resume(srcSession); err != nil {
			// If resume fails, session might not exist — create it
			fmt.Fprintf(w, "creating... ")
			if err := mutagen.Create(srcSession, cfg.ProjectRoot, serverUser, serverHost, remotePath, cfg.Project.Sync.Ignore); err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("creating source sync: %w", err)
			}
		}
		fmt.Fprintf(w, "ok\n")
	} else {
		fmt.Fprintf(w, "%s: creating source sync... ", serverHost)
		if err := mutagen.Create(srcSession, cfg.ProjectRoot, serverUser, serverHost, remotePath, cfg.Project.Sync.Ignore); err != nil {
			fmt.Fprintf(w, "failed\n")
			return fmt.Errorf("creating source sync: %w", err)
		}
		fmt.Fprintf(w, "ok\n")
	}

	// 8. Claude state sync
	fmt.Fprintf(w, "%s: syncing Claude Code state... ", serverHost)
	if warmStart {
		claudeSession := fusesync.ClaudeSessionName(projectName)
		if err := mutagen.Resume(claudeSession); err != nil {
			if err := fusesync.CreateClaudeStateSync(mutagen, projectName, cfg.ProjectRoot, serverUser, serverHost, remotePath); err != nil {
				fmt.Fprintf(w, "failed\n")
				return fmt.Errorf("creating claude state sync: %w", err)
			}
		}
	} else {
		if err := fusesync.CreateClaudeStateSync(mutagen, projectName, cfg.ProjectRoot, serverUser, serverHost, remotePath); err != nil {
			fmt.Fprintf(w, "failed\n")
			return fmt.Errorf("creating claude state sync: %w", err)
		}
	}
	fmt.Fprintf(w, "ok\n")

	// 9. Copy global state (CLAUDE.md, settings, agents, skills)
	fmt.Fprintf(w, "%s: copying global state... ", serverHost)
	if err := fusesync.CopyGlobalState(sshClient, serverUser); err != nil {
		fmt.Fprintf(w, "failed\n")
		return fmt.Errorf("copying global state: %w", err)
	}
	fmt.Fprintf(w, "ok\n")

	// 10. Wait for initial source sync
	fmt.Fprintf(w, "%s: waiting for initial sync... ", serverHost)
	if err := mutagen.WaitForSync(srcSession, syncTimeout); err != nil {
		fmt.Fprintf(w, "warning: %v\n", err)
	} else {
		fmt.Fprintf(w, "ok\n")
	}

	// 11. Generate and distribute RPC secret
	secret, err := rpc.GenerateSecret()
	if err != nil {
		return fmt.Errorf("generating secret: %w", err)
	}

	// Write secret locally
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	secretDir := filepath.Join(home, ".fusebox", "run")
	if err := os.MkdirAll(secretDir, 0700); err != nil {
		return fmt.Errorf("creating secret directory: %w", err)
	}
	secretPath := filepath.Join(secretDir, projectName+".secret")
	if err := os.WriteFile(secretPath, []byte(secret), 0600); err != nil {
		return fmt.Errorf("writing local secret: %w", err)
	}

	// Write secret into container
	containerName := container.ContainerName(projectName)
	writeSecretCmd := fmt.Sprintf(
		"docker exec %s bash -c 'mkdir -p /root/.fusebox && echo -n '\"'\"'%s'\"'\"' > /root/.fusebox/secret && chmod 600 /root/.fusebox/secret'",
		containerName, secret,
	)
	_, stderr, exitCode, err := sshClient.RunCommand(writeSecretCmd)
	if err != nil {
		return fmt.Errorf("writing remote secret: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("writing remote secret failed (exit %d): %s", exitCode, stderr)
	}

	fmt.Fprintf(w, "%s: rpc secret distributed\n", serverHost)

	// 12. Reverse tunnel
	fmt.Fprintf(w, "%s: establishing reverse tunnel (port %d)... ", serverHost, rpcPort)
	tunnel, err := sshClient.ReverseTunnel(rpcPort, rpcPort)
	if err != nil {
		fmt.Fprintf(w, "failed\n")
		return fmt.Errorf("reverse tunnel: %w", err)
	}
	defer tunnel.Close()
	fmt.Fprintf(w, "ok\n")

	// 13. Start local daemon
	rpcAddr := fmt.Sprintf("127.0.0.1:%d", rpcPort)
	listener, err := net.Listen("tcp", rpcAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", rpcAddr, err)
	}

	syncWaiter := fusesync.NewSyncWaiter(mutagen)
	logger := log.New(w, "", 0)

	daemonSrv := daemon.NewServer(listener, daemon.ServerConfig{
		Config:      cfg.Project,
		Secret:      secret,
		ProjectDir:  cfg.ProjectRoot,
		Logger:      logger,
		SyncWaiter:  syncWaiter,
		SessionName: srcSession,
		SyncTimeout: 30 * time.Second,
	})

	// Start status socket
	sockPath, err := daemon.StatusSocketPath(projectName)
	if err != nil {
		return fmt.Errorf("getting status socket path: %w", err)
	}

	statusSrv, err := daemon.NewStatusServer(sockPath, func() daemon.StatusInfo {
		return daemon.StatusInfo{
			Project:       projectName,
			Server:        serverHost,
			Container:     containerName,
			ActionRunning: daemonSrv.IsActionRunning(),
			LastAction:    daemonSrv.GetLastAction(),
		}
	})
	if err != nil {
		return fmt.Errorf("creating status server: %w", err)
	}
	go statusSrv.Serve()
	defer statusSrv.Close()

	// Write PID file
	pidPath := filepath.Join(secretDir, projectName+".pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer os.Remove(pidPath)

	// 14-15. Ready
	fmt.Fprintf(w, "\nReady. Connect with: mosh --port=%d %s\n\n", moshPort, serverHost)

	// 16. Handle Ctrl-C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemonSrv.Serve()
	}()

	select {
	case sig := <-sigCh:
		fmt.Fprintf(w, "\n%s: received %s, shutting down...\n", serverHost, sig)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("daemon error: %w", err)
		}
	}

	// Graceful shutdown
	fmt.Fprintf(w, "%s: stopping daemon... ", serverHost)
	daemonSrv.Close()
	fmt.Fprintf(w, "ok\n")

	fmt.Fprintf(w, "%s: pausing sync... ", serverHost)
	_ = mutagen.Pause(srcSession)
	_ = mutagen.Pause(fusesync.ClaudeSessionName(projectName))
	fmt.Fprintf(w, "ok\n")

	// Tunnel and SSH closed by defers

	fmt.Fprintf(w, "%s: session paused (container still running)\n", serverHost)
	return nil
}
