package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAlbum_ReturnsHeaderAndTracks(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		if strings.Contains(url, "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		if strings.Contains(url, "method=album.getData") {
			return mkResp(200, `{"error":[],"results":{
				"ALB_ID":"10","ALB_TITLE":"RAM","ART_NAME":"Daft Punk","ALB_PICTURE":"pic1","NUMBER_TRACK":"13"
			}}`), nil
		}
		if strings.Contains(url, "method=song.getListByAlbum") {
			return mkResp(200, `{"error":[],"results":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"Give Life Back to Music","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"275"},
				{"SNG_ID":"2","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"248"}
			]}}`), nil
		}
		t.Fatalf("unexpected URL: %s", url)
		return nil, nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	alb, err := c.Album(context.Background(), "10")
	if err != nil {
		t.Fatal(err)
	}
	if alb.Header.ID != "10" || alb.Header.Title != "RAM" {
		t.Errorf("Header = %+v", alb.Header)
	}
	if len(alb.Tracks) != 2 || alb.Tracks[0].ID != "1" || alb.Tracks[1].ID != "2" {
		t.Errorf("Tracks = %+v", alb.Tracks)
	}
}
