package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/qrterm"
)

func pairCmd() *cobra.Command {
	var reset bool
	var port int
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Print pairing URLs (player + phone QR). Use --reset to rotate the token.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPair(reset, port)
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false, "rotate the bearer token (invalidates paired devices)")
	cmd.Flags().IntVar(&port, "port", 8080, "service port (matches `serve --port`)")
	return cmd
}

func runPair(reset bool, port int) error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		return err
	}
	var tok string
	if reset {
		tok, err = config.RotateToken(path, cfg)
	} else {
		tok, err = config.EnsureToken(path, cfg)
	}
	if err != nil {
		return err
	}

	addrs, err := network.LANAddrs()
	if err != nil {
		return err
	}

	playerURL := "http://localhost:" + strconv.Itoa(port) + "/?t=" + tok
	fmt.Println("Player URL (open on the laptop):")
	fmt.Println("  " + playerURL)
	fmt.Println()

	if len(addrs) == 0 {
		fmt.Println("⚠ No private (RFC1918) LAN address found. Phone pairing won't work.")
		return nil
	}
	fmt.Println("Phone URL (scan QR with phone camera):")
	for _, a := range addrs {
		phoneURL := buildURL(a, port, tok)
		fmt.Println("  " + phoneURL)
		qrterm.Render(os.Stdout, phoneURL)
		fmt.Println()
	}
	return nil
}

func buildURL(ip net.IP, port int, tok string) string {
	return "http://" + ip.String() + ":" + strconv.Itoa(port) + "/?t=" + tok
}
