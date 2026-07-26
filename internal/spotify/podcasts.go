package spotify

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// Podcasts (shows/episodes) and saved-library reads for shows, episodes and
// audiobooks. Single-object gets and the saved-list endpoints below were
// verified live against a Development-mode app post-Feb-2026. Saving/removing
// shows and episodes goes through the consolidated library_save/library_remove
// (PUT/DELETE /me/library) tools, which accept any Spotify URI.

// GetShow returns a single show's (podcast's) raw metadata.
func (c *Client) GetShow(ctx context.Context, id string) (json.RawMessage, error) {
	q := url.Values{}
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/shows/"+bareID(id), q)
}

// GetEpisode returns a single episode's raw metadata.
func (c *Client) GetEpisode(ctx context.Context, id string) (json.RawMessage, error) {
	q := url.Values{}
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/episodes/"+bareID(id), q)
}

// ShowEpisodes lists a show's episodes (paginated, newest first).
func (c *Client) ShowEpisodes(ctx context.Context, id string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/shows/"+bareID(id)+"/episodes", q)
}

// SavedShows lists the user's saved shows (podcasts).
func (c *Client) SavedShows(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	q.Set("offset", strconv.Itoa(offset))
	return c.getRaw(ctx, "/me/shows", q)
}

// SavedEpisodes lists the user's saved episodes.
func (c *Client) SavedEpisodes(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/me/episodes", q)
}

// SavedAudiobooks lists the user's saved audiobooks.
func (c *Client) SavedAudiobooks(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	q.Set("offset", strconv.Itoa(offset))
	return c.getRaw(ctx, "/me/audiobooks", q)
}
