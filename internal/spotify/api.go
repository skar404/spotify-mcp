package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---- Search ---------------------------------------------------------------

// searchLimit clamps to the Development-mode maximum of 10.
func searchLimit(limit int) int {
	if limit <= 0 {
		return 5
	}
	if limit > 10 {
		return 10
	}
	return limit
}

// Search runs a raw search for arbitrary types (track/album/artist/playlist).
func (c *Client) Search(ctx context.Context, query, qtype string, limit, offset int) (json.RawMessage, error) {
	if qtype == "" {
		qtype = "track"
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("type", qtype)
	q.Set("limit", strconv.Itoa(searchLimit(limit)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/search", q)
}

// SearchTracks returns slim tracks for a query.
func (c *Client) SearchTracks(ctx context.Context, query string, limit, offset int) ([]Track, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("type", "track")
	q.Set("limit", strconv.Itoa(searchLimit(limit)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	var out struct {
		Tracks struct {
			Items []Track `json:"items"`
		} `json:"tracks"`
	}
	if err := c.getJSON(ctx, "/search", q, &out); err != nil {
		return nil, err
	}
	return out.Tracks.Items, nil
}

// ---- Playback / player ----------------------------------------------------

// PlaybackState returns the raw current playback state (device, track, etc.).
func (c *Client) PlaybackState(ctx context.Context) (json.RawMessage, error) {
	data, err := c.do(ctx, http.MethodGet, "/me/player", nil, nil)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return json.RawMessage(`{"note":"no active playback"}`), nil
	}
	return json.RawMessage(data), nil
}

// Devices lists available Connect devices.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out struct {
		Devices []Device `json:"devices"`
	}
	if err := c.getJSON(ctx, "/me/player/devices", nil, &out); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

// TransferPlayback moves playback to a device. This is the only transport
// command that restricted devices (e.g. many speakers) accept.
func (c *Client) TransferPlayback(ctx context.Context, deviceID string, play bool) error {
	body := map[string]any{"device_ids": []string{deviceID}, "play": play}
	_, err := c.do(ctx, http.MethodPut, "/me/player", nil, body)
	return err
}

func deviceQuery(deviceID string) url.Values {
	if deviceID == "" {
		return nil
	}
	q := url.Values{}
	q.Set("device_id", deviceID)
	return q
}

// Play starts playback. Provide either contextURI (album/playlist/artist) or a
// list of track URIs. Both empty resumes current playback.
func (c *Client) Play(ctx context.Context, deviceID, contextURI string, uris []string, positionMs int) error {
	var body map[string]any
	switch {
	case contextURI != "":
		body = map[string]any{"context_uri": contextURI}
		if positionMs > 0 {
			body["position_ms"] = positionMs
		}
	case len(uris) > 0:
		norm := make([]string, len(uris))
		for i, u := range uris {
			norm[i] = trackURI(u)
		}
		body = map[string]any{"uris": norm}
		if positionMs > 0 {
			body["position_ms"] = positionMs
		}
	}
	_, err := c.do(ctx, http.MethodPut, "/me/player/play", deviceQuery(deviceID), body)
	return err
}

// Pause pauses playback.
func (c *Client) Pause(ctx context.Context, deviceID string) error {
	_, err := c.do(ctx, http.MethodPut, "/me/player/pause", deviceQuery(deviceID), nil)
	return err
}

// Next skips to the next track.
func (c *Client) Next(ctx context.Context, deviceID string) error {
	_, err := c.do(ctx, http.MethodPost, "/me/player/next", deviceQuery(deviceID), nil)
	return err
}

// Previous skips to the previous track.
func (c *Client) Previous(ctx context.Context, deviceID string) error {
	_, err := c.do(ctx, http.MethodPost, "/me/player/previous", deviceQuery(deviceID), nil)
	return err
}

// Seek jumps to a position in the current track.
func (c *Client) Seek(ctx context.Context, positionMs int, deviceID string) error {
	q := deviceQuery(deviceID)
	if q == nil {
		q = url.Values{}
	}
	q.Set("position_ms", strconv.Itoa(positionMs))
	_, err := c.do(ctx, http.MethodPut, "/me/player/seek", q, nil)
	return err
}

// SetVolume sets playback volume (0-100).
func (c *Client) SetVolume(ctx context.Context, percent int, deviceID string) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	q := deviceQuery(deviceID)
	if q == nil {
		q = url.Values{}
	}
	q.Set("volume_percent", strconv.Itoa(percent))
	_, err := c.do(ctx, http.MethodPut, "/me/player/volume", q, nil)
	return err
}

// QueueAdd appends a track to the queue.
func (c *Client) QueueAdd(ctx context.Context, idOrURI, deviceID string) error {
	q := deviceQuery(deviceID)
	if q == nil {
		q = url.Values{}
	}
	q.Set("uri", trackURI(idOrURI))
	_, err := c.do(ctx, http.MethodPost, "/me/player/queue", q, nil)
	return err
}

// QueueGet returns the raw upcoming queue.
func (c *Client) QueueGet(ctx context.Context) (json.RawMessage, error) {
	return c.getRaw(ctx, "/me/player/queue", nil)
}

// RecentlyPlayed returns the raw recently-played history.
func (c *Client) RecentlyPlayed(ctx context.Context, limit int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	return c.getRaw(ctx, "/me/player/recently-played", q)
}

// ---- Playlists (Feb-2026 paths) ------------------------------------------

// Playlists lists the current user's playlists.
func (c *Client) Playlists(ctx context.Context, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 50)))
	q.Set("offset", strconv.Itoa(offset))
	return c.getRaw(ctx, "/me/playlists", q)
}

// PlaylistItems returns items of a playlist (new /items path).
func (c *Client) PlaylistItems(ctx context.Context, id string, limit, offset int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 100, 100)))
	q.Set("offset", strconv.Itoa(offset))
	if m := c.Market(ctx); m != "" {
		q.Set("market", m)
	}
	return c.getRaw(ctx, "/playlists/"+id+"/items", q)
}

// CreatePlaylist creates a playlist for the current user (POST /me/playlists).
func (c *Client) CreatePlaylist(ctx context.Context, name, description string, public bool) (*Playlist, error) {
	body := map[string]any{"name": name, "public": public}
	if description != "" {
		body["description"] = description
	}
	data, err := c.do(ctx, http.MethodPost, "/me/playlists", nil, body)
	if err != nil {
		return nil, err
	}
	var pl Playlist
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

// PlaylistAddItems appends tracks (POST /playlists/{id}/items).
func (c *Client) PlaylistAddItems(ctx context.Context, id string, idsOrURIs []string) error {
	uris := make([]string, len(idsOrURIs))
	for i, s := range idsOrURIs {
		uris[i] = trackURI(s)
	}
	// Spotify caps at 100 URIs per request.
	for start := 0; start < len(uris); start += 100 {
		end := start + 100
		if end > len(uris) {
			end = len(uris)
		}
		if _, err := c.do(ctx, http.MethodPost, "/playlists/"+id+"/items", nil,
			map[string]any{"uris": uris[start:end]}); err != nil {
			return err
		}
	}
	return nil
}

// PlaylistRemoveItems removes tracks (DELETE /playlists/{id}/items).
func (c *Client) PlaylistRemoveItems(ctx context.Context, id string, idsOrURIs []string) error {
	items := make([]map[string]string, 0, len(idsOrURIs))
	for _, s := range idsOrURIs {
		items = append(items, map[string]string{"uri": trackURI(s)})
	}
	_, err := c.do(ctx, http.MethodDelete, "/playlists/"+id+"/items", nil, map[string]any{"items": items})
	return err
}

// ChangePlaylistDetails updates a playlist's name/description/public flag.
func (c *Client) ChangePlaylistDetails(ctx context.Context, id, name, description string, public *bool) error {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if description != "" {
		body["description"] = description
	}
	if public != nil {
		body["public"] = *public
	}
	if len(body) == 0 {
		return fmt.Errorf("nothing to change")
	}
	_, err := c.do(ctx, http.MethodPut, "/playlists/"+id, nil, body)
	return err
}

// UploadCover sets a playlist cover from base64-encoded JPEG bytes.
func (c *Client) UploadCover(ctx context.Context, id, jpegBase64 string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		apiBase+"/playlists/"+id+"/images", strings.NewReader(jpegBase64))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "image/jpeg")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Path: "PUT /playlists/" + id + "/images"}
	}
	return nil
}

// ---- Library / follow (consolidated /me/library) -------------------------

// LibrarySave saves/follows items by URI (tracks, albums, artists, ...).
func (c *Client) LibrarySave(ctx context.Context, uris []string) error {
	_, err := c.do(ctx, http.MethodPut, "/me/library", nil, map[string]any{"uris": uris})
	return err
}

// LibraryRemove removes/unfollows items by URI.
func (c *Client) LibraryRemove(ctx context.Context, uris []string) error {
	_, err := c.do(ctx, http.MethodDelete, "/me/library", nil, map[string]any{"uris": uris})
	return err
}

// LibraryContains checks saved status for the given URIs.
func (c *Client) LibraryContains(ctx context.Context, uris []string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("uris", strings.Join(uris, ","))
	return c.getRaw(ctx, "/me/library/contains", q)
}

// FollowingList returns followed artists.
func (c *Client) FollowingList(ctx context.Context, limit int) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("type", "artist")
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 50)))
	return c.getRaw(ctx, "/me/following", q)
}

// ---- Top items / saved (taste signals) -----------------------------------

// TopItems returns the user's top artists or tracks. kind: "artists"|"tracks".
func (c *Client) TopItems(ctx context.Context, kind, timeRange string, limit int) (json.RawMessage, error) {
	if kind != "artists" {
		kind = "tracks"
	}
	if timeRange == "" {
		timeRange = "medium_term"
	}
	q := url.Values{}
	q.Set("time_range", timeRange)
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 20)))
	return c.getRaw(ctx, "/me/top/"+kind, q)
}

// topTracks returns slim top tracks (used by recommender).
func (c *Client) topTracks(ctx context.Context, timeRange string, limit int) ([]Track, error) {
	q := url.Values{}
	q.Set("time_range", timeRange)
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 50)))
	var out struct {
		Items []Track `json:"items"`
	}
	if err := c.getJSON(ctx, "/me/top/tracks", q, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// topArtists returns slim top artists (used by recommender).
func (c *Client) topArtists(ctx context.Context, timeRange string, limit int) ([]Artist, error) {
	q := url.Values{}
	q.Set("time_range", timeRange)
	q.Set("limit", strconv.Itoa(clamp(limit, 1, 50, 50)))
	var out struct {
		Items []Artist `json:"items"`
	}
	if err := c.getJSON(ctx, "/me/top/artists", q, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// savedTrackIDs returns the IDs of the user's saved tracks (for dedup), up to max.
func (c *Client) savedTrackIDs(ctx context.Context, max int) (map[string]bool, error) {
	ids := make(map[string]bool)
	for offset := 0; offset < max; offset += 50 {
		q := url.Values{}
		q.Set("limit", "50")
		q.Set("offset", strconv.Itoa(offset))
		var out struct {
			Items []struct {
				Track Track `json:"track"`
			} `json:"items"`
		}
		if err := c.getJSON(ctx, "/me/tracks", q, &out); err != nil {
			return ids, err
		}
		if len(out.Items) == 0 {
			break
		}
		for _, it := range out.Items {
			if it.Track.ID != "" {
				ids[it.Track.ID] = true
			}
		}
		if len(out.Items) < 50 {
			break
		}
	}
	return ids, nil
}

func clamp(v, lo, hi, def int) int {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
