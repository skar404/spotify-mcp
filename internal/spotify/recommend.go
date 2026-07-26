package spotify

import (
	"context"
	"encoding/json"
	"regexp"
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

var idPattern = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)

// RecommendBySeeds builds recommendations from explicit seeds instead of the
// user's overall taste: artist names/ids/URIs, track ids/URIs (their artists
// are used), and/or genre names. Each seed is searched and up to perArtist
// fresh tracks (not already saved) are taken from it.
func (c *Client) RecommendBySeeds(ctx context.Context, seedArtists, seedTracks, genres []string, limit, perArtist int) ([]Recommendation, error) {
	if limit <= 0 {
		limit = 30
	}
	if perArtist <= 0 {
		perArtist = 3
	}

	type seed struct{ query, label string }
	var seeds []seed
	addArtist := func(name string) {
		if name = strings.TrimSpace(name); name != "" {
			seeds = append(seeds, seed{query: "artist:" + name, label: name})
		}
	}

	for _, a := range seedArtists {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "spotify:") || strings.Contains(a, "open.spotify.com") || idPattern.MatchString(a) {
			if name := c.artistName(ctx, a); name != "" {
				addArtist(name)
				continue
			}
		}
		addArtist(a) // treat as a plain artist name
	}
	for _, t := range seedTracks {
		for _, n := range c.trackArtistNames(ctx, t) {
			addArtist(n)
		}
	}
	for _, g := range genres {
		if g = strings.TrimSpace(g); g != "" {
			seeds = append(seeds, seed{query: "genre:" + g, label: "genre:" + g})
		}
	}
	if len(seeds) == 0 {
		return nil, &APIError{Status: 0, Path: "recommend-by-seeds", Body: "no usable seeds (provide seed_artists, seed_tracks, or genres)"}
	}

	owned, _ := c.savedTrackIDs(ctx, 2000)
	seen := map[string]bool{}
	recs := []Recommendation{}
	for _, s := range seeds {
		if len(recs) >= limit {
			break
		}
		tracks, err := c.SearchTracks(ctx, s.query, 10, 0)
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
			seen[t.ID] = true
			recs = append(recs, Recommendation{ID: t.ID, URI: t.URI, Name: t.Name, Artists: t.ArtistNames(), Seed: s.label})
			taken++
		}
	}
	return recs, nil
}

// artistName resolves an artist id/URI/URL to its display name.
func (c *Client) artistName(ctx context.Context, idOrURI string) string {
	raw, err := c.GetArtist(ctx, idOrURI)
	if err != nil {
		return ""
	}
	var a struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &a)
	return a.Name
}

// trackArtistNames resolves a track id/URI/URL to the names of its artists.
func (c *Client) trackArtistNames(ctx context.Context, idOrURI string) []string {
	raw, err := c.GetTrack(ctx, idOrURI)
	if err != nil {
		return nil
	}
	var t Track
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil
	}
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
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
