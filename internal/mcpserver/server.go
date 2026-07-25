// Package mcpserver registers the Spotify tools on an MCP server. Handlers are
// thin wrappers over internal/spotify; every error is surfaced to the caller.
package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/skar404/spotify-mcp/internal/spotify"
)

// New builds an MCP server with all Spotify tools registered.
func New(sp *spotify.Client, version string) *server.MCPServer {
	s := server.NewMCPServer("spotify-mcp", version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	registerProfile(s, sp)
	registerSearch(s, sp)
	registerPlayback(s, sp)
	registerQueue(s, sp)
	registerPlaylists(s, sp)
	registerLibrary(s, sp)
	registerTaste(s, sp)
	return s
}

// ---- helpers --------------------------------------------------------------

func jsonText(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("marshal error: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func rawText(v json.RawMessage, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(v)), nil
}

func okText(err error, msg string) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func hasArg(req mcp.CallToolRequest, key string) bool {
	_, ok := req.GetArguments()[key]
	return ok
}

// ---- profile --------------------------------------------------------------

func registerProfile(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("me",
		mcp.WithDescription("Get the current user's Spotify profile (id, country/market, product).")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.Me(ctx))
		})
}

// ---- search ---------------------------------------------------------------

func registerSearch(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Search Spotify. limit is capped at 10 (Feb-2026 API). Use offset to paginate."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query. Supports field filters e.g. artist:, track:, album:")),
		mcp.WithString("type", mcp.Description("track, album, artist, or playlist (comma-separated ok)"), mcp.DefaultString("track")),
		mcp.WithNumber("limit", mcp.Description("Max results, 1-10"), mcp.DefaultNumber(10)),
		mcp.WithNumber("offset", mcp.Description("Pagination offset"), mcp.DefaultNumber(0)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return rawText(sp.Search(ctx,
			req.GetString("query", ""),
			req.GetString("type", "track"),
			req.GetInt("limit", 10),
			req.GetInt("offset", 0)))
	})

	s.AddTool(mcp.NewTool("search_tracks",
		mcp.WithDescription("Search tracks and return a slim list (id, uri, name, artists). limit capped at 10."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query; supports artist:/track: filters")),
		mcp.WithNumber("limit", mcp.Description("Max results, 1-10"), mcp.DefaultNumber(10)),
		mcp.WithNumber("offset", mcp.Description("Pagination offset"), mcp.DefaultNumber(0)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tracks, err := sp.SearchTracks(ctx, req.GetString("query", ""), req.GetInt("limit", 10), req.GetInt("offset", 0))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonText(tracks)
	})
}

// ---- playback -------------------------------------------------------------

func registerPlayback(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("playback_state",
		mcp.WithDescription("Get current playback state: active device (with is_restricted), track, progress.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.PlaybackState(ctx))
		})

	s.AddTool(mcp.NewTool("devices_list",
		mcp.WithDescription("List Spotify Connect devices. is_restricted=true devices (e.g. speakers) only accept transfer_playback, not direct play/queue/skip.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			devs, err := sp.Devices(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return jsonText(devs)
		})

	s.AddTool(mcp.NewTool("transfer_playback",
		mcp.WithDescription("Move playback to a device by id. The correct way to drive restricted devices (speakers). Device must be online/awake."),
		mcp.WithString("device_id", mcp.Required(), mcp.Description("Target device id (from devices_list)")),
		mcp.WithBoolean("play", mcp.Description("Start playing after transfer"), mcp.DefaultBool(true)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("device_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return okText(sp.TransferPlayback(ctx, id, req.GetBool("play", true)), "Playback transferred.")
	})

	s.AddTool(mcp.NewTool("play",
		mcp.WithDescription("Start/resume playback. Provide context_uri (album/playlist/artist) OR uris (tracks). Both empty = resume."),
		mcp.WithString("context_uri", mcp.Description("Album/playlist/artist URI to play")),
		mcp.WithArray("uris", mcp.Description("Track ids or URIs to play in order"), mcp.WithStringItems()),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)")),
		mcp.WithNumber("position_ms", mcp.Description("Start position in ms"), mcp.DefaultNumber(0)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return okText(sp.Play(ctx,
			req.GetString("device_id", ""),
			req.GetString("context_uri", ""),
			req.GetStringSlice("uris", nil),
			req.GetInt("position_ms", 0)), "Playback started.")
	})

	s.AddTool(mcp.NewTool("pause", mcp.WithDescription("Pause playback."),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return okText(sp.Pause(ctx, req.GetString("device_id", "")), "Paused.")
		})

	s.AddTool(mcp.NewTool("next", mcp.WithDescription("Skip to next track (fails on restricted devices)."),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return okText(sp.Next(ctx, req.GetString("device_id", "")), "Skipped.")
		})

	s.AddTool(mcp.NewTool("previous", mcp.WithDescription("Skip to previous track."),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return okText(sp.Previous(ctx, req.GetString("device_id", "")), "Back to previous.")
		})

	s.AddTool(mcp.NewTool("seek", mcp.WithDescription("Seek to a position in the current track."),
		mcp.WithNumber("position_ms", mcp.Required(), mcp.Description("Position in milliseconds")),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return okText(sp.Seek(ctx, req.GetInt("position_ms", 0), req.GetString("device_id", "")), "Seeked.")
		})

	s.AddTool(mcp.NewTool("set_volume", mcp.WithDescription("Set playback volume (0-100)."),
		mcp.WithNumber("percent", mcp.Required(), mcp.Description("Volume 0-100")),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return okText(sp.SetVolume(ctx, req.GetInt("percent", 50), req.GetString("device_id", "")), "Volume set.")
		})
}

// ---- queue ----------------------------------------------------------------

func registerQueue(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("queue_add",
		mcp.WithDescription("Add a track to the playback queue. Requires a non-restricted active device."),
		mcp.WithString("track", mcp.Required(), mcp.Description("Track id or spotify:track: URI")),
		mcp.WithString("device_id", mcp.Description("Target device id (optional)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t, err := req.RequireString("track")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return okText(sp.QueueAdd(ctx, t, req.GetString("device_id", "")), "Queued.")
	})

	s.AddTool(mcp.NewTool("queue_get", mcp.WithDescription("Get the current playback queue.")),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.QueueGet(ctx))
		})

	s.AddTool(mcp.NewTool("recently_played", mcp.WithDescription("Get recently played tracks."),
		mcp.WithNumber("limit", mcp.Description("Max items, 1-50"), mcp.DefaultNumber(20))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.RecentlyPlayed(ctx, req.GetInt("limit", 20)))
		})
}

// ---- playlists ------------------------------------------------------------

func registerPlaylists(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("playlists_list", mcp.WithDescription("List the current user's playlists."),
		mcp.WithNumber("limit", mcp.Description("Max, 1-50"), mcp.DefaultNumber(50)),
		mcp.WithNumber("offset", mcp.Description("Pagination offset"), mcp.DefaultNumber(0))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.Playlists(ctx, req.GetInt("limit", 50), req.GetInt("offset", 0)))
		})

	s.AddTool(mcp.NewTool("playlist_items", mcp.WithDescription("Get items of a playlist (new /items endpoint)."),
		mcp.WithString("playlist_id", mcp.Required(), mcp.Description("Playlist id")),
		mcp.WithNumber("limit", mcp.Description("Max, 1-100"), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description("Pagination offset"), mcp.DefaultNumber(0))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := req.RequireString("playlist_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return rawText(sp.PlaylistItems(ctx, id, req.GetInt("limit", 100), req.GetInt("offset", 0)))
		})

	s.AddTool(mcp.NewTool("playlist_create",
		mcp.WithDescription("Create a playlist for the current user (POST /me/playlists, the Feb-2026 path)."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Playlist name")),
		mcp.WithString("description", mcp.Description("Playlist description")),
		mcp.WithBoolean("public", mcp.Description("Public playlist"), mcp.DefaultBool(false)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		pl, err := sp.CreatePlaylist(ctx, name, req.GetString("description", ""), req.GetBool("public", false))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonText(pl)
	})

	s.AddTool(mcp.NewTool("playlist_add_items",
		mcp.WithDescription("Add tracks to a playlist (POST /playlists/{id}/items). Auto-batches per 100."),
		mcp.WithString("playlist_id", mcp.Required(), mcp.Description("Playlist id")),
		mcp.WithArray("tracks", mcp.Required(), mcp.Description("Track ids or URIs"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("playlist_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return okText(sp.PlaylistAddItems(ctx, id, req.GetStringSlice("tracks", nil)), "Items added.")
	})

	s.AddTool(mcp.NewTool("playlist_remove_items",
		mcp.WithDescription("Remove tracks from a playlist (DELETE /playlists/{id}/items)."),
		mcp.WithString("playlist_id", mcp.Required(), mcp.Description("Playlist id")),
		mcp.WithArray("tracks", mcp.Required(), mcp.Description("Track ids or URIs to remove"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("playlist_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return okText(sp.PlaylistRemoveItems(ctx, id, req.GetStringSlice("tracks", nil)), "Items removed.")
	})

	s.AddTool(mcp.NewTool("playlist_change_details",
		mcp.WithDescription("Change a playlist's name/description/public flag."),
		mcp.WithString("playlist_id", mcp.Required(), mcp.Description("Playlist id")),
		mcp.WithString("name", mcp.Description("New name")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithBoolean("public", mcp.Description("New public flag")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("playlist_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var public *bool
		if hasArg(req, "public") {
			v := req.GetBool("public", false)
			public = &v
		}
		return okText(sp.ChangePlaylistDetails(ctx, id, req.GetString("name", ""), req.GetString("description", ""), public), "Playlist updated.")
	})

	s.AddTool(mcp.NewTool("playlist_upload_cover",
		mcp.WithDescription("Set a playlist cover image (base64-encoded JPEG, max 256KB)."),
		mcp.WithString("playlist_id", mcp.Required(), mcp.Description("Playlist id")),
		mcp.WithString("image_base64", mcp.Required(), mcp.Description("Base64 JPEG bytes (no data: prefix)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("playlist_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		img, err := req.RequireString("image_base64")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return okText(sp.UploadCover(ctx, id, img), "Cover uploaded.")
	})
}

// ---- library / follow -----------------------------------------------------

func registerLibrary(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("library_save",
		mcp.WithDescription("Save/like items to library (PUT /me/library). Accepts any Spotify URIs (tracks, albums, ...)."),
		mcp.WithArray("uris", mcp.Required(), mcp.Description("Spotify URIs, e.g. spotify:track:..."), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return okText(sp.LibrarySave(ctx, req.GetStringSlice("uris", nil)), "Saved to library.")
	})

	s.AddTool(mcp.NewTool("library_remove",
		mcp.WithDescription("Remove/unsave items from library (DELETE /me/library)."),
		mcp.WithArray("uris", mcp.Required(), mcp.Description("Spotify URIs to remove"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return okText(sp.LibraryRemove(ctx, req.GetStringSlice("uris", nil)), "Removed from library.")
	})

	s.AddTool(mcp.NewTool("library_contains",
		mcp.WithDescription("Check whether URIs are saved (GET /me/library/contains)."),
		mcp.WithArray("uris", mcp.Required(), mcp.Description("Spotify URIs to check"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return rawText(sp.LibraryContains(ctx, req.GetStringSlice("uris", nil)))
	})

	s.AddTool(mcp.NewTool("follow_artists",
		mcp.WithDescription("Follow artists (PUT /me/library with artist URIs)."),
		mcp.WithArray("artists", mcp.Required(), mcp.Description("Artist ids or spotify:artist: URIs"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := spotify.ArtistURIs(req.GetStringSlice("artists", nil))
		return okText(sp.LibrarySave(ctx, uris), "Followed.")
	})

	s.AddTool(mcp.NewTool("unfollow_artists",
		mcp.WithDescription("Unfollow artists (DELETE /me/library with artist URIs)."),
		mcp.WithArray("artists", mcp.Required(), mcp.Description("Artist ids or spotify:artist: URIs"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		uris := spotify.ArtistURIs(req.GetStringSlice("artists", nil))
		return okText(sp.LibraryRemove(ctx, uris), "Unfollowed.")
	})

	s.AddTool(mcp.NewTool("following_list", mcp.WithDescription("List followed artists."),
		mcp.WithNumber("limit", mcp.Description("Max, 1-50"), mcp.DefaultNumber(50))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.FollowingList(ctx, req.GetInt("limit", 50)))
		})
}

// ---- taste / recommendations ---------------------------------------------

func registerTaste(s *server.MCPServer, sp *spotify.Client) {
	s.AddTool(mcp.NewTool("top_items", mcp.WithDescription("Get the user's top artists or tracks."),
		mcp.WithString("kind", mcp.Description("artists or tracks"), mcp.DefaultString("tracks"), mcp.Enum("artists", "tracks")),
		mcp.WithString("time_range", mcp.Description("short_term, medium_term, or long_term"), mcp.DefaultString("medium_term"), mcp.Enum("short_term", "medium_term", "long_term")),
		mcp.WithNumber("limit", mcp.Description("Max, 1-50"), mcp.DefaultNumber(20))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return rawText(sp.TopItems(ctx, req.GetString("kind", "tracks"), req.GetString("time_range", "medium_term"), req.GetInt("limit", 20)))
		})

	s.AddTool(mcp.NewTool("recommend",
		mcp.WithDescription("Taste-based recommendations (replaces the deprecated /recommendations). Seeds from your top artists/tracks, returns fresh tracks not already in your library."),
		mcp.WithNumber("limit", mcp.Description("How many tracks to return"), mcp.DefaultNumber(30)),
		mcp.WithNumber("per_artist", mcp.Description("Max tracks per seed artist"), mcp.DefaultNumber(3)),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		recs, err := sp.RecommendFromTaste(ctx, req.GetInt("limit", 30), req.GetInt("per_artist", 3))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonText(recs)
	})
}
