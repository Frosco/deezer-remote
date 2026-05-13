//go:build linux

package media

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
)

// TestIntegration_GetMediaURL runs only when DEEZER_INTEGRATION=1.
// Verifies the live pipeline (arl auth → song.getData → media.getUrl)
// resolves a stable public track and that the returned URL is reachable
// (HEAD only — does not download audio bytes from the CDN).
func TestIntegration_GetMediaURL(t *testing.T) {
	if os.Getenv("DEEZER_INTEGRATION") != "1" {
		t.Skip("set DEEZER_INTEGRATION=1 to run")
	}
	const trackID = "3135556" // Daft Punk – Harder, Better, Faster, Stronger

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	path, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		t.Fatal(err)
	}
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		t.Fatalf("getUserData: %v", err)
	}
	td, err := gw.SongGetData(ctx, trackID)
	if err != nil {
		t.Fatalf("song.getData: %v", err)
	}

	mc := NewClient(http.DefaultTransport)
	res, err := mc.GetURL(ctx, URLRequest{
		LicenseToken: ud.LicenseToken,
		TrackToken:   td.TrackToken,
		Formats:      []Format{FormatMP3_320, FormatMP3_128},
	})
	if err != nil {
		t.Fatalf("media.getUrl: %v", err)
	}
	if res.URL == "" {
		t.Fatal("empty media URL")
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, res.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD CDN URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("CDN HEAD status = %d (URL may have changed semantics)", resp.StatusCode)
	}
	t.Logf("format=%s url=%.60s… exp=%d", res.Format, res.URL, res.Expiry)
}
