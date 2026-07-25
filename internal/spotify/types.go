package spotify

import "strings"

// Artist is a slim artist reference.
type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Track is a slim track shape used for search and recommendations.
type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	Popularity int      `json:"popularity"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
	Album      struct {
		Name string `json:"name"`
	} `json:"album"`
}

// ArtistNames joins artist names with ", ".
func (t Track) ArtistNames() string {
	names := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

// Device is a Spotify Connect device. IsRestricted devices reject direct
// transport commands (play/queue/skip); use TransferPlayback for them.
type Device struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	IsActive      bool   `json:"is_active"`
	IsRestricted  bool   `json:"is_restricted"`
	VolumePercent int    `json:"volume_percent"`
}

// Playlist is a slim created-playlist result.
type Playlist struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URI          string `json:"uri"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

func trackURI(idOrURI string) string {
	if strings.HasPrefix(idOrURI, "spotify:") {
		return idOrURI
	}
	return "spotify:track:" + idOrURI
}

// ArtistURIs converts artist ids or URIs into spotify:artist: URIs.
func ArtistURIs(idsOrURIs []string) []string { return toURIs(idsOrURIs, "artist") }

func toURIs(idsOrURIs []string, kind string) []string {
	out := make([]string, 0, len(idsOrURIs))
	for _, s := range idsOrURIs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "spotify:") {
			out = append(out, s)
		} else {
			out = append(out, "spotify:"+kind+":"+s)
		}
	}
	return out
}
