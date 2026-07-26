package spotify

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// Catalog object lookups. Spotify's Feb-2026 migration removed the *batch* gets
// (GET /tracks?ids=…, /albums?ids=…) and audio-features for Development-mode
// apps, but the single-object GETs below still work. All accept a bare id, a
// spotify:kind:id URI, or an open.spotify.com URL.

// GetTrack returns a single track's raw metadata.
func (c *Client) GetTrack(ctx context.Context, id string) (json.RawMessage, error) {
	q := url.Values{}
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/tracks/"+bareID(id), q)
}

// GetAlbum returns a single album's raw metadata (includes its track list).
func (c *Client) GetAlbum(ctx context.Context, id string) (json.RawMessage, error) {
	q := url.Values{}
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/albums/"+bareID(id), q)
}

// GetArtist returns a single artist's raw metadata.
func (c *Client) GetArtist(ctx context.Context, id string) (json.RawMessage, error) {
	return c.getRaw(ctx, "/artists/"+bareID(id), nil)
}

// AlbumTracks lists the tracks on an album (paginated).
func (c *Client) AlbumTracks(ctx context.Context, id string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 50)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/albums/"+bareID(id)+"/tracks", q)
}

// ArtistAlbums lists an artist's albums. includeGroups is an optional comma-
// separated filter of album,single,appears_on,compilation.
func (c *Client) ArtistAlbums(ctx context.Context, id, includeGroups string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	q.Set("offset", strconv.Itoa(offset))
	if includeGroups != "" {
		q.Set("include_groups", includeGroups)
	}
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/artists/"+bareID(id)+"/albums", q)
}
