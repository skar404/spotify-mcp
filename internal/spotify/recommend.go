package spotify

import (
	"context"
	"sort"
	"strings"
)

// Recommendation is one suggested track.
type Recommendation struct {
	ID      string `json:"id"`
	URI     string `json:"uri"`
	Name    string `json:"name"`
	Artists string `json:"artists"`
	Seed    string `json:"seed_artist"`
}

// RecommendFromTaste builds recommendations without the deprecated
// /recommendations endpoint: it seeds from the user's top artists (and the
// artists of their top tracks), searches each seed for tracks, and returns
// fresh picks that are not already in the user's saved library.
//
// limit is the number of recommendations to return; perArtist caps how many
// tracks to take from each seed artist.
func (c *Client) RecommendFromTaste(ctx context.Context, limit, perArtist int) ([]Recommendation, error) {
	if limit <= 0 {
		limit = 30
	}
	if perArtist <= 0 {
		perArtist = 3
	}

	// Seed artists: top artists + artists behind top tracks (short+medium term).
	seeds := map[string]string{} // id -> name
	order := []string{}
	addSeed := func(a Artist) {
		if a.ID == "" || a.Name == "" {
			return
		}
		if _, ok := seeds[a.ID]; !ok {
			seeds[a.ID] = a.Name
			order = append(order, a.ID)
		}
	}
	for _, tr := range []string{"short_term", "medium_term"} {
		if arts, err := c.topArtists(ctx, tr, 50); err == nil {
			for _, a := range arts {
				addSeed(a)
			}
		}
		if trk, err := c.topTracks(ctx, tr, 50); err == nil {
			for _, t := range trk {
				for _, a := range t.Artists {
					addSeed(a)
				}
			}
		}
	}
	if len(order) == 0 {
		return nil, &APIError{Status: 0, Path: "top-items", Body: "no top artists/tracks available to seed recommendations"}
	}

	owned, _ := c.savedTrackIDs(ctx, 2000) // best-effort dedup set

	seen := map[string]bool{}
	recs := []Recommendation{}
	for _, id := range order {
		if len(recs) >= limit {
			break
		}
		name := seeds[id]
		tracks, err := c.SearchTracks(ctx, "artist:"+name, 10, 0)
		if err != nil {
			continue
		}
		taken := 0
		for _, t := range tracks {
			if taken >= perArtist || len(recs) >= limit {
				break
			}
			if t.ID == "" || owned[t.ID] || seen[t.ID] {
				continue
			}
			if !artistMatches(t, name) {
				continue
			}
			seen[t.ID] = true
			recs = append(recs, Recommendation{
				ID: t.ID, URI: t.URI, Name: t.Name, Artists: t.ArtistNames(), Seed: name,
			})
			taken++
		}
	}

	// Stable, popularity-agnostic order is fine; keep seed grouping but shuffle
	// deterministically by name to avoid front-loading one artist.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Seed < recs[j].Seed })
	return recs, nil
}

func artistMatches(t Track, name string) bool {
	n := strings.ToLower(name)
	for _, a := range t.Artists {
		if strings.Contains(strings.ToLower(a.Name), n) {
			return true
		}
	}
	return false
}
