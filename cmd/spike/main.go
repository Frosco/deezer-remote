// Command spike proves the Deezer streaming pipeline end-to-end on Nils's
// account. Throwaway: deleted after Phase 1 starts.
//
//	$ deezer-remote-spike --track 3135556
//	playing: Daft Punk - Get Lucky (MP3_320)
//	got CDN url (TTL 1m48s), file size 8.4 MB
//	decrypted in 1.2s
//	wrote ./3135556.mp3
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
)

func main() {
	var trackID string
	flag.StringVar(&trackID, "track", "", "Deezer track ID to fetch (required)")
	flag.Parse()
	if trackID == "" {
		log.Fatal("--track is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, trackID); err != nil {
		log.Fatalf("spike: %v", err)
	}
}

func run(ctx context.Context, trackID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}

	fmt.Fprintln(os.Stderr, "→ fetching user data")
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return fmt.Errorf("getUserData: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  user_id=%d license_token=%s…\n", ud.UserID, truncate(ud.LicenseToken, 12))

	fmt.Fprintln(os.Stderr, "→ song.getData")
	td, err := gw.SongGetData(ctx, trackID)
	if err != nil {
		return fmt.Errorf("song.getData: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  %s — %s (sng_id=%s)\n", td.Artist, td.Title, td.SngID)

	fmt.Fprintln(os.Stderr, "→ media.getUrl (MP3_320 then MP3_128)")
	mc := media.NewClient(http.DefaultTransport)
	mr, err := mc.GetURL(ctx, media.URLRequest{
		LicenseToken: ud.LicenseToken,
		TrackToken:   td.TrackToken,
		Formats:      []media.Format{media.FormatMP3_320, media.FormatMP3_128},
	})
	if err != nil {
		return fmt.Errorf("media.getUrl: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  format=%s url=%s… exp=%d\n", mr.Format, truncate(mr.URL, 60), mr.Expiry)

	// Fetch encrypted bytes.
	fmt.Fprintln(os.Stderr, "→ GET CDN url")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mr.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cdn get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("cdn get: http %d", resp.StatusCode)
	}
	fmt.Fprintf(os.Stderr, "  content-length=%d\n", resp.ContentLength)

	// Save raw encrypted bytes as a debugging artefact.
	encPath := filepath.Clean(trackID + ".enc")
	encFile, err := os.Create(encPath)
	if err != nil {
		return err
	}
	defer encFile.Close()

	mp3Path := filepath.Clean(trackID + ".mp3")
	mp3File, err := os.Create(mp3Path)
	if err != nil {
		return err
	}
	defer mp3File.Close()

	// Pipe: HTTP body → tee → (encFile, decryptReader → mp3File).
	tee := io.TeeReader(resp.Body, encFile)

	key := media.KeyFromSNGID(td.SngID)
	dec := media.Decrypt(tee, key)

	start := time.Now()
	n, err := io.Copy(mp3File, dec)
	if err != nil {
		// Best-effort: keep the partial output.
		_ = os.Rename(mp3Path, mp3Path+".partial")
		return fmt.Errorf("decrypt copy: %w", err)
	}
	fmt.Fprintf(os.Stderr, "→ decrypted %d bytes in %s\n", n, time.Since(start).Round(time.Millisecond))

	// Sanity: very rough MP3-header check on first byte (0xFF) — purely informational.
	header := make([]byte, 4)
	if f, err := os.Open(mp3Path); err == nil {
		if _, err := io.ReadFull(f, header); err == nil {
			frameSync := binary.BigEndian.Uint16(header[0:2]) & 0xFFE0
			if frameSync == 0xFFE0 {
				fmt.Fprintln(os.Stderr, "  ✓ output starts with MP3 frame sync")
			} else {
				fmt.Fprintf(os.Stderr, "  ⚠ output does not start with MP3 frame sync (first bytes: % x)\n", header)
			}
		}
		f.Close()
	}

	// Drop the .enc on success — it's only useful for failures.
	_ = encFile.Close()
	_ = os.Remove(encPath)

	fmt.Fprintf(os.Stderr, "wrote %s\n", mp3Path)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
