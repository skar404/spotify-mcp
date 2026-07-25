// Package auth handles Spotify OAuth2 (Authorization Code flow with client
// secret), token persistence and automatic refresh. All human-facing output
// goes to stderr so it never corrupts the stdio MCP channel on stdout.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"
)

// Scopes is the full set of permissions the server requests.
var Scopes = []string{
	"user-read-private", "user-read-email",
	"user-library-read", "user-library-modify",
	"user-read-playback-state", "user-modify-playback-state", "user-read-currently-playing",
	"playlist-read-private", "playlist-read-collaborative",
	"playlist-modify-private", "playlist-modify-public",
	"user-top-read", "user-read-recently-played",
	"user-follow-read", "user-follow-modify",
	"streaming", "ugc-image-upload", "app-remote-control",
}

const (
	authURL  = "https://accounts.spotify.com/authorize"
	tokenURL = "https://accounts.spotify.com/api/token"
)

// Config holds credentials and file locations.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	TokenPath    string
}

// FromEnv builds a Config from environment variables, applying defaults.
func FromEnv() (Config, error) {
	c := Config{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("SPOTIFY_REDIRECT_URI"),
		TokenPath:    os.Getenv("SPOTIFY_TOKEN_PATH"),
	}
	if c.RedirectURI == "" {
		c.RedirectURI = "http://127.0.0.1:8080/callback"
	}
	if c.TokenPath == "" {
		home, _ := os.UserHomeDir()
		c.TokenPath = filepath.Join(home, ".config", "spotify-mcp", "token.json")
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return c, fmt.Errorf("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
	}
	return c, nil
}

func (c Config) oauth() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURI,
		Scopes:       Scopes,
		Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
	}
}

// HTTPClient returns an *http.Client that transparently refreshes and persists
// the token. It fails if no token is cached yet (run `spotify-mcp auth`).
// The same base transport is used for token refresh and API calls, so setting
// SPOTIFY_FORCE_IPV4 (for networks with broken IPv6) applies to both.
func (c Config) HTTPClient(ctx context.Context) (*http.Client, error) {
	tok, err := loadToken(c.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("no usable token at %s (run `spotify-mcp auth`): %w", c.TokenPath, err)
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: baseTransport(), Timeout: 30 * time.Second})
	src := &persistSource{base: c.oauth().TokenSource(ctx, tok), path: c.TokenPath, last: tok}
	return oauth2.NewClient(ctx, src), nil
}

// baseTransport builds the underlying HTTP transport, optionally forcing IPv4.
func baseTransport() http.RoundTripper {
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	dial := d.DialContext
	if os.Getenv("SPOTIFY_FORCE_IPV4") != "" {
		dial = func(ctx context.Context, _, addr string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp4", addr)
		}
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// Login runs the interactive Authorization Code flow and stores the token.
func (c Config) Login(ctx context.Context) (*oauth2.Token, error) {
	conf := c.oauth()
	addr, cbPath, err := listenAddr(c.RedirectURI)
	if err != nil {
		return nil, err
	}

	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(cbPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if e := q.Get("error"); e != "" {
			_, _ = fmt.Fprintf(w, "<h2>Authorization failed: %s</h2>", e)
			errCh <- fmt.Errorf("authorization denied: %s", e)
			return
		}
		if q.Get("state") != state {
			_, _ = w.Write([]byte("<h2>State mismatch</h2>"))
			errCh <- fmt.Errorf("state mismatch")
			return
		}
		_, _ = w.Write([]byte("<h2>Spotify authorized ✓ — you can close this tab.</h2>"))
		codeCh <- q.Get("code")
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot bind %s (another process using the port?): %w", addr, err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	authCodeURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline)
	fmt.Fprintf(os.Stderr, "Opening browser for Spotify authorization.\nIf it does not open, visit:\n\n%s\n\n", authCodeURL)
	_ = openBrowser(authCodeURL)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case code := <-codeCh:
		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("token exchange failed: %w", err)
		}
		if err := saveToken(c.TokenPath, tok); err != nil {
			return nil, err
		}
		return tok, nil
	case <-time.After(3 * time.Minute):
		return nil, fmt.Errorf("timed out waiting for authorization")
	}
}

// persistSource wraps a refreshing TokenSource and writes the token to disk
// whenever it changes (access tokens live ~1h and rotate on refresh).
type persistSource struct {
	base oauth2.TokenSource
	path string
	last *oauth2.Token
}

func (p *persistSource) Token() (*oauth2.Token, error) {
	t, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if p.last == nil || t.AccessToken != p.last.AccessToken || t.RefreshToken != p.last.RefreshToken {
		if err := saveToken(p.path, t); err != nil {
			fmt.Fprintf(os.Stderr, "spotify-mcp: failed to persist token: %v\n", err)
		}
		p.last = t
	}
	return t, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	if t.AccessToken == "" && t.RefreshToken == "" {
		return nil, fmt.Errorf("cached token is empty")
	}
	return &t, nil
}

func saveToken(path string, t *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func listenAddr(redirect string) (addr, path string, err error) {
	u, err := url.Parse(redirect)
	if err != nil {
		return "", "", err
	}
	port := u.Port()
	if port == "" {
		port = "8080"
	}
	path = u.Path
	if path == "" {
		path = "/callback"
	}
	return net.JoinHostPort(u.Hostname(), port), path, nil
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

func randomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
