package spotify

import (
	"encoding/json"
	"testing"
)

func TestTrackURI(t *testing.T) {
	cases := map[string]string{
		"3IkfGuitetzrJWUiBj0V7s":         "spotify:track:3IkfGuitetzrJWUiBj0V7s",
		"spotify:track:3IkfGuitetzrJWUi": "spotify:track:3IkfGuitetzrJWUi",
	}
	for in, want := range cases {
		if got := trackURI(in); got != want {
			t.Errorf("trackURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArtistURIs(t *testing.T) {
	got := ArtistURIs([]string{"abc", "spotify:artist:xyz", "  ", "def"})
	want := []string{"spotify:artist:abc", "spotify:artist:xyz", "spotify:artist:def"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSearchLimitClamp(t *testing.T) {
	for _, c := range []struct{ in, want int }{{0, 5}, {1, 1}, {10, 10}, {11, 10}, {50, 10}} {
		if got := searchLimit(c.in); got != c.want {
			t.Errorf("searchLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestTrackDecodeTolerant is the regression guard for the old server's
// KeyError 'tracks': a track object missing album/optional fields must decode
// without panicking and still yield id/name/artists.
func TestTrackDecodeTolerant(t *testing.T) {
	payload := `{"tracks":{"items":[
		{"id":"1","name":"A","uri":"spotify:track:1","artists":[{"id":"x","name":"Art"}]},
		{"id":"2","name":"B"}
	]}}`
	var out struct {
		Tracks struct {
			Items []Track `json:"items"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(out.Tracks.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(out.Tracks.Items))
	}
	if out.Tracks.Items[0].ArtistNames() != "Art" {
		t.Errorf("artist names = %q", out.Tracks.Items[0].ArtistNames())
	}
	if out.Tracks.Items[1].Name != "B" {
		t.Errorf("second track name = %q", out.Tracks.Items[1].Name)
	}
}
