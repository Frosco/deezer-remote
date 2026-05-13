package transport

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/web"
)

// ServerInput holds the dependencies NewServer wires together.
type ServerInput struct {
	BindAddr     string
	BearerToken  string
	LicenseToken string
	Gateway      *gateway.Client
	URLClient    media.TrackURLAPI
	Cache        *media.Cache
	Sess         *session.Session
}

// NewServer composes API + /stream + /ws + static SPA over a single
// mux and returns an *http.Server plus the live hub (so cmd code can
// introspect connections for logging).
//
// The caller is responsible for: loading config, calling
// gw.GetUserData (to seed CSRF + license token), and shutting the
// server down.
func NewServer(in ServerInput) (*http.Server, *Hub) {
	resolver := media.NewResolver(
		&fetcherAdapter{c: in.Gateway, u: in.URLClient},
		in.LicenseToken,
		[]media.Format{media.FormatMP3_320, media.FormatMP3_128},
		in.Cache,
	)

	apiH := NewAPIHandlers(in.Gateway)
	streamH := NewStreamHandler(resolver, &http.Client{Timeout: 0})

	mux := http.NewServeMux()
	// Public, token-protected:
	apiMux := http.NewServeMux()
	apiH.Register(apiMux)
	streamH.Register(apiMux)
	mux.Handle("/api/", RequireToken(in.BearerToken)(apiMux))
	mux.Handle("/stream/", RequireToken(in.BearerToken)(apiMux))

	// WS endpoint. The router needs the hub for SendToPlayer / Broadcast,
	// but the hub needs the router for OnCmd / OnPlayback — break the
	// cycle by constructing the router with a nil hub and SetHub after.
	router := NewRouter(in.Sess, in.Gateway, resolver, nil, in.BearerToken)
	hub := NewHub(router)
	router.SetHub(hub)
	mux.Handle("/ws", RequireToken(in.BearerToken)(hub))

	// Static SPA (no token gating; the page reads `?t=` from the URL on
	// first load and stores it in localStorage; subsequent /api / /ws
	// calls carry it):
	mux.Handle("/", http.FileServerFS(web.FS()))

	return &http.Server{
		Addr:              in.BindAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, hub
}

// fetcherAdapter satisfies media.TrackFetcher over a gateway.Client +
// media.TrackURLAPI.
type fetcherAdapter struct {
	c *gateway.Client
	u media.TrackURLAPI
}

func (a *fetcherAdapter) SongGetData(ctx context.Context, trackID string) (media.TrackInfo, error) {
	td, err := a.c.SongGetData(ctx, trackID)
	if err != nil {
		return media.TrackInfo{}, err
	}
	return media.TrackInfo{
		SngID:           td.SngID,
		TrackToken:      td.TrackToken,
		FileSizeMP3_320: td.FileSizeMP3_320,
		FileSizeMP3_128: td.FileSizeMP3_128,
	}, nil
}

func (a *fetcherAdapter) GetURL(ctx context.Context, req media.URLRequest) (*media.URLResult, error) {
	return a.u.GetURL(ctx, req)
}

// Compile-time touchpoint for the embedded FS.
var _ fs.FS = web.FS()
