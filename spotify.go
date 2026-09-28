package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	reNextData = regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">([^<]+)</script>`)
)

type SpotifyEmbedNextData struct {
	Props struct {
		PageProps struct {
			State struct {
				Data struct {
					Entity struct {
						Name           string `json:"name"`
						VisualIdentity struct {
							Image []struct {
								URL string `json:"url"`
							} `json:"image"`
						} `json:"visualIdentity"`
						TrackList []struct {
							Title        string `json:"title"`
							Subtitle     string `json:"subtitle"` // Artist name
							AudioPreview struct {
								URL string `json:"url"`
							} `json:"audioPreview"`
							UID string `json:"uid"`
						} `json:"trackList"`
					} `json:"entity"`
				} `json:"data"`
			} `json:"state"`
		} `json:"pageProps"`
	} `json:"props"`
}

// ExtractURL pulls any URL found inside text (e.g. from WhatsApp/Spotify share messages).
func ExtractURL(input string) string {
	input = strings.TrimSpace(input)
	re := regexp.MustCompile(`https?://[^\s"'<>]+`)
	matches := re.FindAllString(input, -1)
	for _, m := range matches {
		mLower := strings.ToLower(m)
		if strings.Contains(mLower, "spotify.com") || strings.Contains(mLower, "deezer.com") || strings.Contains(mLower, "spotify.link") || strings.Contains(mLower, "page.link") {
			return m
		}
	}
	if len(matches) > 0 {
		return matches[0]
	}

	words := strings.Fields(input)
	for _, w := range words {
		wLower := strings.ToLower(w)
		if strings.Contains(wLower, "spotify.com") || strings.Contains(wLower, "deezer.com") || strings.HasPrefix(wLower, "spotify:") || strings.HasPrefix(wLower, "spotify_") || strings.HasPrefix(wLower, "deezer_") {
			return w
		}
	}
	return input
}

// ExtractSpotifyEntityID parses any Spotify URL (playlist or album) and returns entityType and ID.
func ExtractSpotifyEntityID(input string) (entityType, id string) {
	urlCandidate := ExtractURL(input)
	urlCandidate = strings.TrimSpace(urlCandidate)
	if strings.Contains(strings.ToLower(urlCandidate), "deezer.com") || strings.HasPrefix(strings.ToLower(urlCandidate), "deezer_") {
		return "", ""
	}

	clean := urlCandidate
	if idx := strings.Index(clean, "?"); idx != -1 {
		clean = clean[:idx]
	}

	// 0. Matches internal cache ID format: spotify_playlist_<id> or spotify_album_<id>
	reInternal := regexp.MustCompile(`(?i)^spotify_(playlist|album)_([a-zA-Z0-9_\-]+)$`)
	if m := reInternal.FindStringSubmatch(clean); len(m) > 2 {
		return strings.ToLower(m[1]), m[2]
	}

	// 1. Matches playlist (e.g. /playlist/<id>, playlist:<id>, /user/<uid>/playlist/<id>)
	rePlaylist := regexp.MustCompile(`(?i)playlist[/:]+([a-zA-Z0-9_\-]+)`)
	if m := rePlaylist.FindStringSubmatch(clean); len(m) > 1 {
		return "playlist", m[1]
	}

	// 2. Matches album (e.g. /album/<id>, album:<id>)
	reAlbum := regexp.MustCompile(`(?i)album[/:]+([a-zA-Z0-9_\-]+)`)
	if m := reAlbum.FindStringSubmatch(clean); len(m) > 1 {
		return "album", m[1]
	}

	// 3. Fallback: 22-char raw alphanumeric Spotify ID
	cleanTrimmed := strings.Trim(clean, "/")
	if len(cleanTrimmed) == 22 && !strings.Contains(cleanTrimmed, "/") && !strings.Contains(cleanTrimmed, " ") {
		return "playlist", cleanTrimmed
	}

	return "", ""
}

// FetchPublicSpotifyPlaylist scrapes and decodes a public Spotify playlist or album without login.
func FetchPublicSpotifyPlaylist(entityType, entityID string) (*Playlist, error) {
	if entityType == "" {
		entityType = "playlist"
	}
	embedURL := fmt.Sprintf("https://open.spotify.com/embed/%s/%s", entityType, entityID)
	req, err := http.NewRequest(http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao Spotify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o Spotify retornou status %d. Verifique se a playlist/álbum é público", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	matches := reNextData.FindSubmatch(bodyBytes)
	if len(matches) < 2 {
		return nil, errors.New("não foi possível ler os dados da playlist. Verifique se o link está correto")
	}

	var nextData SpotifyEmbedNextData
	if err := json.Unmarshal(matches[1], &nextData); err != nil {
		return nil, fmt.Errorf("erro ao decodificar dados da playlist: %w", err)
	}

	entity := nextData.Props.PageProps.State.Data.Entity
	if len(entity.TrackList) == 0 {
		return nil, errors.New("a playlist está vazia ou não possui faixas acessíveis")
	}

	cover := ""
	if len(entity.VisualIdentity.Image) > 0 {
		cover = entity.VisualIdentity.Image[len(entity.VisualIdentity.Image)-1].URL
	}

	var validTracks []Track
	for i, t := range entity.TrackList {
		if strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Subtitle) == "" {
			continue
		}

		// Artist: RG-06 Banda = primeiro artista
		artist := t.Subtitle
		if commaIdx := strings.Index(artist, ","); commaIdx != -1 {
			artist = strings.TrimSpace(artist[:commaIdx])
		}

		previewURL := t.AudioPreview.URL
		// If preview URL is missing, try iTunes search as fallback
		if previewURL == "" {
			itTrack, err := FindTrackByiTunes(t.Title, artist)
			if err == nil && itTrack.PreviewURL != "" {
				previewURL = itTrack.PreviewURL
			}
		}

		if previewURL == "" {
			continue // RG-05: skip tracks without preview
		}

		validTracks = append(validTracks, Track{
			ID:         fmt.Sprintf("sp_%d", i+1),
			Title:      t.Title,
			Artist:     artist,
			Album:      entity.Name,
			CoverURL:   cover,
			PreviewURL: previewURL,
		})
	}

	if len(validTracks) < 4 {
		return nil, fmt.Errorf("a playlist possui apenas %d faixas com áudio disponível (mínimo necessário: 4)", len(validTracks))
	}

	typeLabel := "Playlist"
	if entityType == "album" {
		typeLabel = "Álbum"
	}

	pl := &Playlist{
		ID:          fmt.Sprintf("spotify_%s_%s", entityType, entityID),
		Name:        entity.Name,
		Description: fmt.Sprintf("%s pública do Spotify com %d músicas disponíveis", typeLabel, len(validTracks)),
		CoverURL:    cover,
		TotalTracks: len(validTracks),
		Source:      "spotify",
		Tracks:      validTracks,
	}
	CachePlaylist(pl)
	return pl, nil
}
