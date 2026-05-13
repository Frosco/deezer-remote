package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestPlaylist_ReturnsHeaderAndTracks(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		if strings.Contains(url, "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		if strings.Contains(url, "method=playlist.getData") {
			return mkResp(200, `{"error":[],"results":{
				"PLAYLIST_ID":"100","TITLE":"My Mix","PARENT_USERNAME":"nils","PLAYLIST_PICTURE":"pic","NB_SONG":"2"
			}}`), nil
		}
		if strings.Contains(url, "method=playlist.getSongs") {
			return mkResp(200, `{"error":[],"results":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"A","ART_NAME":"X","ALB_TITLE":"Z","ALB_PICTURE":"p","DURATION":"180"},
				{"SNG_ID":"2","SNG_TITLE":"B","ART_NAME":"X","ALB_TITLE":"Z","ALB_PICTURE":"p","DURATION":"200"}
			]}}`), nil
		}
		t.Fatalf("unexpected URL: %s", url)
		return nil, nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	p, err := c.Playlist(context.Background(), "100")
	if err != nil {
		t.Fatal(err)
	}
	if p.Header.ID != "100" || p.Header.Owner != "nils" {
		t.Errorf("Header = %+v", p.Header)
	}
	if len(p.Tracks) != 2 || p.Tracks[1].ID != "2" {
		t.Errorf("Tracks = %+v", p.Tracks)
	}
}
