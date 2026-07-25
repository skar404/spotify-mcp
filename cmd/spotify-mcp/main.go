// Command spotify-mcp is a Model Context Protocol server for Spotify.
//
// Usage:
//
//	spotify-mcp auth     # one-time interactive login (opens a browser)
//	spotify-mcp serve    # run the MCP server over stdio (default)
//
// All diagnostic output goes to stderr; stdout is reserved for the MCP
// protocol. Credentials come from the environment:
//
//	SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, SPOTIFY_REDIRECT_URI,
//	SPOTIFY_TOKEN_PATH (defaults to ~/.config/spotify-mcp/token.json)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/skar404/spotify-mcp/internal/auth"
	"github.com/skar404/spotify-mcp/internal/mcpserver"
	"github.com/skar404/spotify-mcp/internal/spotify"
)

const version = "0.1.0"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	cfg, err := auth.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	switch cmd {
	case "auth", "login":
		runAuth(cfg)
	case "serve", "":
		runServe(cfg)
	case "-h", "--help", "help":
		fmt.Fprintln(os.Stderr, "usage: spotify-mcp [auth|serve]")
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (use: auth | serve)\n", cmd)
		os.Exit(2)
	}
}

func runAuth(cfg auth.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tok, err := cfg.Login(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth failed:", err)
		os.Exit(1)
	}

	// Verify by fetching the profile.
	hc, err := cfg.HTTPClient(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth saved but client init failed:", err)
		os.Exit(1)
	}
	_ = tok
	if raw, err := spotify.New(hc).Me(ctx); err == nil {
		fmt.Fprintf(os.Stderr, "Authorized. Token saved to %s\n%s\n", cfg.TokenPath, string(raw))
	} else {
		fmt.Fprintf(os.Stderr, "Authorized and token saved to %s (profile check failed: %v)\n", cfg.TokenPath, err)
	}
}

func runServe(cfg auth.Config) {
	ctx := context.Background()
	hc, err := cfg.HTTPClient(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot start: ", err)
		os.Exit(1)
	}
	sp := spotify.New(hc)
	s := mcpserver.New(sp, version)

	fmt.Fprintln(os.Stderr, "spotify-mcp serving on stdio")
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
