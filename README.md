# spotify-mcp

A first-party [Model Context Protocol](https://modelcontextprotocol.io) server
for Spotify, written in Go. Built to replace `varunneal/spotify-mcp`, which
broke after Spotify's **February 2026 Web API migration** (playlist/library
writes moved paths and started returning `403` on the old endpoints).

## Why this exists

The Feb-2026 migration changed the write endpoints for Development-mode apps:

| Operation | Old (→ 403) | New |
|---|---|---|
| Create playlist | `POST /users/{id}/playlists` | `POST /me/playlists` |
| Add/remove playlist items | `…/playlists/{id}/tracks` | `…/playlists/{id}/items` |
| Save / follow | `PUT /me/tracks`, `PUT /me/following` | `PUT /me/library` (URIs) |
| Check saved | `GET /me/tracks/contains` | `GET /me/library/contains` |
| Search `limit` | ≤ 50 | ≤ 10 |

Removed for Dev mode (this server avoids them): `GET /recommendations`,
`/artists/{id}/top-tracks`, related-artists, audio-features, `/browse/*`,
batch gets, `GET /users/{id}`. Recommendations are rebuilt from
`GET /me/top/*` + search instead.

## Install

```sh
go install github.com/skar404/spotify-mcp/cmd/spotify-mcp@latest
```

…or build from source (see below).

## Setup

Create a Spotify app in the [Developer Dashboard](https://developer.spotify.com/dashboard),
grab its client id/secret, and register the redirect URI. Then:

```sh
git clone https://github.com/skar404/spotify-mcp
cd spotify-mcp
cp .env.example .env       # fill in client id/secret
make build                 # -> bin/spotify-mcp
set -a; . ./.env; set +a   # export the vars
bin/spotify-mcp auth       # one-time browser login, caches the token
```

The redirect URI must be registered on your Spotify app and must be a loopback
IP literal (`http://127.0.0.1:8080/callback`, not `localhost`).

## Register with Claude Code

`~/.claude.json` → `mcpServers`:

```json
"spotify": {
  "command": "/absolute/path/to/spotify-mcp",
  "args": ["serve"],
  "env": {
    "SPOTIFY_CLIENT_ID": "…",
    "SPOTIFY_CLIENT_SECRET": "…",
    "SPOTIFY_REDIRECT_URI": "http://127.0.0.1:8080/callback",
    "SPOTIFY_TOKEN_PATH": "/absolute/path/to/token.json"
  }
}
```

`SPOTIFY_TOKEN_PATH` is optional; it defaults to
`~/.config/spotify-mcp/token.json`.

## Tools

Profile: `me`. Search: `search`, `search_tracks` (limit capped at 10).
Playback: `playback_state`, `devices_list`, `transfer_playback`, `play`,
`pause`, `next`, `previous`, `seek`, `set_volume`. Queue: `queue_add`,
`queue_get`, `recently_played`. Playlists: `playlists_list`, `playlist_items`,
`playlist_create`, `playlist_add_items`, `playlist_remove_items`,
`playlist_change_details`, `playlist_upload_cover`. Library/follow:
`library_save`, `library_remove`, `library_contains`, `follow_artists`,
`unfollow_artists`, `following_list`. Taste: `top_items`, `recommend`.

### Restricted devices (speakers)

Devices with `is_restricted: true` (e.g. a Bose SoundLink Roam 2) reject direct
`play`/`queue`/`next`. Wake the device, then use `transfer_playback` to move the
session onto it.

## Notes

- Auth: Authorization Code + client secret; token cached at `SPOTIFY_TOKEN_PATH`
  and auto-refreshed (access token ~1h). Spotify's refresh token has a 180-day
  ceiling; on `invalid_grant`, re-run `bin/spotify-mcp auth`.
- All diagnostics go to stderr; stdout carries the MCP protocol only.

## License

[MIT](LICENSE)
