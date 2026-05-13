// Command deezer-remote is the on-LAN remote-control music app:
//
//	deezer-remote serve     # start the HTTP + WS service
//	deezer-remote pair      # print pairing URLs + QR (no rotation)
//	deezer-remote pair --reset    # rotate token then print
//	deezer-remote doctor    # validate setup
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is overridable at link time: -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

func main() {
	root := &cobra.Command{
		Use:           "deezer-remote",
		Short:         "On-LAN remote-control music app for Deezer.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(serveCmd())
	root.AddCommand(pairCmd())
	root.AddCommand(doctorCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
