package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSearch_ParsesTracksAlbumsPlaylists(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		body := `{"error":[],"results":{
			"TRACK":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"248"}
			],"count":1},
			"ALBUM":{"data":[
				{"ALB_ID":"10","ALB_TITLE":"RAM","ART_NAME":"Daft Punk","ALB_PICTURE":"pic1","NUMBER_TRACK":"13"}
			],"count":1},
			"PLAYLIST":{"data":[
				{"PLAYLIST_ID":"100","TITLE":"Daft Punk Essentials","PARENT_USERNAME":"deezer","PLAYLIST_PICTURE":"pic2","NB_SONG":"42"}
			],"count":1}
		}}`
		return mkResp(200, body), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	res, err := c.Search(context.Background(), "daft punk", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks) != 1 || res.Tracks[0].ID != "1" || res.Tracks[0].Artist != "Daft Punk" {
		t.Errorf("Tracks = %+v", res.Tracks)
	}
	if len(res.Albums) != 1 || res.Albums[0].ID != "10" || res.Albums[0].TrackCount != 13 {
		t.Errorf("Albums = %+v", res.Albums)
	}
	if len(res.Playlists) != 1 || res.Playlists[0].ID != "100" || res.Playlists[0].TrackCount != 42 {
		t.Errorf("Playlists = %+v", res.Playlists)
	}
}

func TestSearch_EmptyQueryErrors(t *testing.T) {
	c, _ := newClientWithTransport("ARL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport should not be called")
		return nil, nil
	}))
	if _, err := c.Search(context.Background(), "  ", 10); err == nil {
		t.Error("expected error for empty query")
	}
}
