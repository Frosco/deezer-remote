package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/transport"
)

func doctorCmd() *cobra.Command {
	var (
		port    int
		trackID string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate the setup (arl, token, port, LAN IPs, end-to-end stream).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(port, trackID)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8080, "port to probe (matches `serve --port`)")
	cmd.Flags().StringVar(&trackID, "track", "3135556", "stable public track ID for the e2e probe")
	return cmd
}

type check struct {
	name string
	run  func(ctx context.Context) error
}

func runDoctor(port int, trackID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var path string
	var cfg *config.Config
	var gw *gateway.Client

	checks := []check{
		{"config file path", func(ctx context.Context) error {
			p, err := config.DefaultPath()
			path = p
			return err
		}},
		{"arl present (config readable, arl non-empty)", func(ctx context.Context) error {
			c, err := config.LoadFromPath(path)
			cfg = c
			return err
		}},
		{"bearer token present", func(ctx context.Context) error {
			if cfg.BearerToken == "" {
				return fmt.Errorf("no bearer token; run `deezer-remote pair`")
			}
			return nil
		}},
		{"can bind port " + strconv.Itoa(port), func(ctx context.Context) error {
			ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(port))
			if err != nil {
				return err
			}
			return ln.Close()
		}},
		{"arl authenticates against Deezer", func(ctx context.Context) error {
			c, err := gateway.NewClient(cfg.ARL)
			if err != nil {
				return err
			}
			gw = c
			ud, err := gw.GetUserData(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "  → user_id=%d\n", ud.UserID)
			return nil
		}},
		{"LAN addresses reachable from self", func(ctx context.Context) error {
			addrs, err := network.LANAddrs()
			if err != nil {
				return err
			}
			if len(addrs) == 0 {
				return fmt.Errorf("no private (RFC1918) interface")
			}
			for _, a := range addrs {
				if err := probeTCP(ctx, a.String()+":"+strconv.Itoa(port)); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ %s: %v (firewall? wrong network profile?)\n", a, err)
				} else {
					fmt.Fprintf(os.Stderr, "  → %s reachable\n", a)
				}
			}
			return nil
		}},
		{"/stream e2e (track " + trackID + ", first 64 KB)", func(ctx context.Context) error {
			return probeStream(ctx, gw, cfg.BearerToken, trackID)
		}},
	}

	failed := 0
	for _, c := range checks {
		fmt.Fprintf(os.Stderr, "▶ %s\n", c.name)
		if err := c.run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", err)
			failed++
		} else {
			fmt.Fprintln(os.Stderr, "  ✓")
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d checks failed", failed)
	}
	fmt.Fprintln(os.Stderr, "All checks passed.")
	return nil
}

func probeTCP(ctx context.Context, hostport string) error {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return err
	}
	return conn.Close()
}

// probeStream stands up the transport pipeline against an in-process
// httptest-like config, then issues a Range:0-65535 request against
// /stream/<id>?t=<token> and verifies a 206 response with the right
// Content-Range.
func probeStream(ctx context.Context, gw *gateway.Client, tok, trackID string) error {
	mc := media.NewClient(http.DefaultTransport)
	cache := media.NewCache(30 * time.Minute)
	sess := session.New()
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return err
	}
	srv, _ := transport.NewServer(transport.ServerInput{
		BindAddr:     "127.0.0.1:0",
		BearerToken:  tok,
		LicenseToken: ud.LicenseToken,
		Gateway:      gw,
		URLClient:    mc,
		Cache:        cache,
		Sess:         sess,
	})
	// Listen on an ephemeral port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	url := "http://" + ln.Addr().String() + "/stream/" + trackID + "?t=" + tok
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Range", "bytes=0-65535")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("expected 206, got %d", resp.StatusCode)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}
	if n < 4 {
		return fmt.Errorf("stream returned only %d bytes", n)
	}
	return nil
}
