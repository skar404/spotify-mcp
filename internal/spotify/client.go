// Package spotify is a small typed client for the Spotify Web API using the
// endpoint paths introduced by the February 2026 migration (writes go to
// /me/playlists, /me/library and /playlists/{id}/items). Write failures are
// always returned as errors, never swallowed.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxRetries bounds how many times do() retries a 429 (rate-limited) response.
const maxRetries = 3

const apiBase = "https://api.spotify.com/v1"

// Client talks to the Spotify Web API. The http.Client carries the OAuth token.
type Client struct {
	http   *http.Client
	market string
}

// New creates a Client from an authenticated *http.Client.
func New(hc *http.Client) *Client { return &Client{http: hc} }

// APIError surfaces a non-2xx Spotify response verbatim.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("spotify API %d on %q: %s", e.Status, e.Path, e.Body)
}

// do performs a request and returns the raw body, or an *APIError on non-2xx.
// On HTTP 429 it honours the Retry-After header and retries up to maxRetries.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	u := apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}

	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			wait := retryAfter(resp, attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &APIError{Status: resp.StatusCode, Path: method + " " + path, Body: strings.TrimSpace(string(data))}
		}
		return data, nil
	}
}

// retryAfter derives how long to wait before retrying a 429. It prefers the
// Retry-After header (seconds); otherwise it backs off exponentially.
func retryAfter(resp *http.Response, attempt int) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
			return time.Duration(secs)*time.Second + 250*time.Millisecond
		}
	}
	return time.Duration(1<<attempt) * time.Second
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	data, err := c.do(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) getRaw(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	data, err := c.do(ctx, http.MethodGet, path, q, nil)
	return json.RawMessage(data), err
}

// Market returns and caches the user's country code for market-correct results.
func (c *Client) Market(ctx context.Context) string {
	if c.market != "" {
		return c.market
	}
	var me struct {
		Country string `json:"country"`
	}
	if err := c.getJSON(ctx, "/me", nil, &me); err == nil {
		c.market = me.Country
	}
	return c.market
}

// Me returns the raw current-user profile.
func (c *Client) Me(ctx context.Context) (json.RawMessage, error) {
	return c.getRaw(ctx, "/me", nil)
}
