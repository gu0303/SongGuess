package main

import (
	"testing"
)

func TestExtractSpotifyEntityID(t *testing.T) {
	cases := []struct {
		input        string
		expectedType string
		expectedID   string
	}{
		{"https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M?si=123", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/intl-pt/playlist/37i9dQZF1DXcBWIGoYBM5M?si=123", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/intl-es/playlist/37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/intl-en/playlist/37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/user/123/playlist/37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/embed/playlist/37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"Confira minha playlist no Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M?si=abc", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"https://open.spotify.com/album/4aawyAB9vmqN3uQ7FjRGTy", "album", "4aawyAB9vmqN3uQ7FjRGTy"},
		{"spotify:playlist:37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
		{"37i9dQZF1DXcBWIGoYBM5M", "playlist", "37i9dQZF1DXcBWIGoYBM5M"},
	}

	for _, c := range cases {
		eType, id := ExtractSpotifyEntityID(c.input)
		if eType != c.expectedType || id != c.expectedID {
			t.Errorf("ExtractSpotifyEntityID(%q) = (%q, %q), expected (%q, %q)", c.input, eType, id, c.expectedType, c.expectedID)
		}
	}
}

func TestFetchPublicSpotifyPlaylist(t *testing.T) {
	pl, err := FetchPublicSpotifyPlaylist("playlist", "37i9dQZF1DXcBWIGoYBM5M")
	if err != nil {
		t.Fatalf("Failed to fetch public spotify playlist: %v", err)
	}

	if pl.Name == "" {
		t.Errorf("Expected non-empty playlist name")
	}

	if len(pl.Tracks) < 4 {
		t.Errorf("Expected at least 4 tracks, got %d", len(pl.Tracks))
	}
}

func TestResolveTrackCoverAndAlbum(t *testing.T) {
	cover, album := ResolveTrackCoverAndAlbum("Nicole Kidman", "ADÉLA")
	if cover == "" {
		t.Errorf("Expected non-empty cover URL for track")
	}
	if album == "" {
		t.Errorf("Expected non-empty album name for track")
	}
	t.Logf("Resolved Cover: %s, Album: %s", cover, album)
}

func TestExtractDeezerEntityID(t *testing.T) {
	cases := []struct {
		input        string
		expectedType string
		expectedID   string
	}{
		{"https://www.deezer.com/playlist/1306931615", "playlist", "1306931615"},
		{"https://www.deezer.com/br/playlist/1306931615", "playlist", "1306931615"},
		{"https://www.deezer.com/en/playlist/1306931615", "playlist", "1306931615"},
		{"https://www.deezer.com/pt-br/playlist/1306931615?utm_source=deezer", "playlist", "1306931615"},
		{"https://www.deezer.com/album/302127", "album", "302127"},
		{"https://www.deezer.com/br/album/302127?app_id=123", "album", "302127"},
		{"deezer.com/playlist/1306931615", "playlist", "1306931615"},
		{"1306931615", "playlist", "1306931615"},
	}

	for _, c := range cases {
		eType, id := ExtractDeezerEntityID(c.input)
		if eType != c.expectedType || id != c.expectedID {
			t.Errorf("ExtractDeezerEntityID(%q) = (%q, %q), expected (%q, %q)", c.input, eType, id, c.expectedType, c.expectedID)
		}
	}
}

func TestLoadPlaylistFromInput(t *testing.T) {
	// Test Deezer URL loading
	plDeezer, err := LoadPlaylistFromInput("https://www.deezer.com/br/playlist/1306931615")
	if err != nil {
		t.Fatalf("Failed to load Deezer playlist: %v", err)
	}
	if len(plDeezer.Tracks) < 4 {
		t.Errorf("Expected at least 4 tracks from Deezer, got %d", len(plDeezer.Tracks))
	}

	// Test Spotify URL loading
	plSpotify, err := LoadPlaylistFromInput("https://open.spotify.com/intl-pt/playlist/37i9dQZF1DXcBWIGoYBM5M")
	if err != nil {
		t.Fatalf("Failed to load Spotify playlist: %v", err)
	}
	if len(plSpotify.Tracks) < 4 {
		t.Errorf("Expected at least 4 tracks from Spotify, got %d", len(plSpotify.Tracks))
	}

	// Test loading by ID (the exact string sent by frontend on startGame!)
	plById, err := LoadPlaylistFromInput(plSpotify.ID)
	if err != nil {
		t.Fatalf("Failed to load Spotify playlist by ID (%s): %v", plSpotify.ID, err)
	}
	if plById.Name != plSpotify.Name {
		t.Errorf("Expected name %s, got %s", plSpotify.Name, plById.Name)
	}

	plDeezerById, err := LoadPlaylistFromInput(plDeezer.ID)
	if err != nil {
		t.Fatalf("Failed to load Deezer playlist by ID (%s): %v", plDeezer.ID, err)
	}
	if plDeezerById.Name != plDeezer.Name {
		t.Errorf("Expected name %s, got %s", plDeezer.Name, plDeezerById.Name)
	}
}
