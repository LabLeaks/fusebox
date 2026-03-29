package main

import (
	"os"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/orchestrator"
	"github.com/spf13/cobra"
)

var upServerFlag string
var upBinaryFlag string

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start a fusebox session",
	Long: `Provisions a Sysbox container on the remote server, starts Mutagen sync,
establishes the SSH reverse tunnel for RPC, and starts the local daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return orchestrator.Up(orchestrator.UpConfig{
			ResolveOpts: config.ResolveOptions{
				ServerOverride: upServerFlag,
			},
			FuseboxBinary: upBinaryFlag,
			Log:           os.Stdout,
		})
	},
}

func init() {
	upCmd.Flags().StringVar(&upServerFlag, "server", "", "override remote server host")
	upCmd.Flags().StringVar(&upBinaryFlag, "binary", "", "path to fusebox-remote binary to copy into container")
}
