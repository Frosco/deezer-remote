package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/qrterm"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/transport"
)

func serveCmd() *cobra.Command {
	var (
		bindHost string
		port     int
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP + WebSocket service.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(bindHost, port)
		},
	}
	cmd.Flags().StringVar(&bindHost, "bind", "0.0.0.0", "host to bind (0.0.0.0 = all interfaces)")
	cmd.Flags().IntVar(&port, "port", 8080, "port to listen on")
	return cmd
}

func runServe(bindHost string, port int) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tok, err := config.EnsureToken(path, cfg)
	if err != nil {
		return err
	}

	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		return err
	}
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return fmt.Errorf("authenticate with Deezer (arl): %w", err)
	}
	fmt.Fprintf(os.Stderr, "Authenticated as user %d\n", ud.UserID)

	mc := media.NewClient(http.DefaultTransport)
	cache := media.NewCache(30 * time.Minute)
	sess := session.New()

	srv, _ := transport.NewServer(transport.ServerInput{
		BindAddr:     bindHost + ":" + strconv.Itoa(port),
		BearerToken:  tok,
		LicenseToken: ud.LicenseToken,
		Gateway:      gw,
		URLClient:    mc,
		Cache:        cache,
		Sess:         sess,
	})

	printURLs(port, tok)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "Listening on %s\n", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func printURLs(port int, tok string) {
	fmt.Println("Player URL (open on the laptop):")
	fmt.Println("  http://localhost:" + strconv.Itoa(port) + "/?t=" + tok)
	fmt.Println()
	addrs, _ := network.LANAddrs()
	if len(addrs) == 0 {
		fmt.Println("⚠ No private LAN address detected. Phone pairing won't work.")
		return
	}
	fmt.Println("Phone URL (scan QR with phone camera):")
	for _, a := range addrs {
		url := "http://" + a.String() + ":" + strconv.Itoa(port) + "/?t=" + tok
		fmt.Println("  " + url)
		qrterm.Render(os.Stdout, url)
		fmt.Println()
	}
}
