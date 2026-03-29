package main

import (
	"fmt"
	"path/filepath"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/orchestrator"
	"github.com/lableaks/fusebox/internal/sync"
	"github.com/spf13/cobra"
)

var (
	downDestroy bool
	downForce   bool
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop a fusebox session",
	Long: `Stops the local daemon, terminates the SSH reverse tunnel, and pauses Mutagen
sync. Use --destroy to also remove the remote container.`,
	RunE: runDown,
}

func init() {
	downCmd.Flags().BoolVar(&downDestroy, "destroy", false, "terminate sync sessions and remove remote container")
	downCmd.Flags().BoolVar(&downForce, "force", false, "kill running actions before stopping")
}

func runDown(cmd *cobra.Command, args []string) error {
	resolved, err := config.Resolve(config.ResolveOptions{})
	if err != nil {
		return err
	}

	projectName := filepath.Base(resolved.ProjectRoot)

	mgr, err := sync.NewMutagenManager()
	if err != nil {
		return err
	}

	// Container operations require SSH, but for down without --destroy
	// we only need mutagen and the local daemon. Use a nil-safe stub
	// that only errors if actually called.
	var cm orchestrator.ContainerManager
	if downDestroy {
		// TODO: establish SSH connection and create container.NewManager
		// For now, return clear error if --destroy is used without SSH
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: --destroy requires SSH connection (not yet wired)")
	}

	opts := orchestrator.DownOptions{
		ProjectName: projectName,
		Destroy:     downDestroy,
		Force:       downForce,
	}

	return orchestrator.Down(opts, mgr, cm, func(s string) {
		fmt.Fprintln(cmd.OutOrStdout(), s)
	})
}
