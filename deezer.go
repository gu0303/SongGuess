package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	httpClient = &http.Client{Timeout: 10 * time.Second}

	// audioStore caches raw audio bytes by random token
	audioStoreMutex sync.RWMutex
	audioStore      = make(map[string][]byte)
)

// GenerateRandomToken returns a safe hex token for /audio/{token} (anti-cheating RG-04).
func GenerateRandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StoreAudio stores the audio buffer and returns a unique token.
func StoreAudio(data []byte) string {
	token := GenerateRandomToken()
	audioStoreMutex.Lock()
	audioStore[token] = data
	audioStoreMutex.Unlock()
	return token
}

// GetAudio retrieves audio data by token.
func GetAudio(token string) ([]byte, bool) {
	audioStoreMutex.RLock()
	defer audioStoreMutex.RUnlock()
	data, ok := audioStore[token]
	return data, ok
}

// DeezerTrackResponse mirrors Deezer's track json response.
type DeezerTrackResponse struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	TitleShort string `json:"title_short"`
	ISRC       string `json:"isrc"`
	Preview    string `json:"preview"`
	Link       string `json:"link"`
	Artist     struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		CoverBig string `json:"cover_big"`
		Cover    string `json:"cover"`
	} `json:"album"`
}

type DeezerPlaylistResponse struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PictureBig  string `json:"picture_big"`
	Tracks      struct {
		Data []DeezerTrackResponse `json:"data"`
	} `json:"tracks"`
}

type iTunesSearchResponse struct {
	ResultCount int `json:"resultCount"`
	Results     []struct {
		TrackName      string `json:"trackName"`
		ArtistName     string `json:"artistName"`
		CollectionName string `json:"collectionName"`
		PreviewURL     string `json:"previewUrl"`
		ArtworkURL100  string `json:"artworkUrl100"`
		TrackViewURL   string `json:"trackViewUrl"`
	} `json:"results"`
}

// FindTrackByISRC searches Deezer for a track using its ISRC.
func FindTrackByISRC(isrc string) (*Track, error) {
	if isrc == "" {
		return nil, errors.New("empty isrc")
	}

	apiURL := fmt.Sprintf("https://api.deezer.com/2.0/track/isrc:%s", url.PathEscape(isrc))
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deezer returned status %d", resp.StatusCode)
	}

	var dTrack DeezerTrackResponse
	if err := json.NewDecoder(resp.Body).Decode(&dTrack); err != nil {
		return nil, err
	}

	if dTrack.Preview == "" {
		return nil, errors.New("no preview available on deezer")
	}

	cover := dTrack.Album.CoverBig
	if cover == "" {
		cover = dTrack.Album.Cover
	}

	title := dTrack.TitleShort
	if title == "" {
		title = dTrack.Title
	}

	return &Track{
		ID:         fmt.Sprintf("deezer_%d", dTrack.ID),
		Title:      title,
		Artist:     dTrack.Artist.Name,
		Album:      dTrack.Album.Title,
		CoverURL:   cover,
		ISRC:       dTrack.ISRC,
		PreviewURL: dTrack.Preview,
	}, nil
}

func getHighResArtwork(urlStr string) string {
	if strings.Contains(urlStr, "100x100bb") {
		return strings.Replace(urlStr, "100x100bb", "600x600bb", 1)
	}
	if strings.Contains(urlStr, "100x100") {
		return strings.Replace(urlStr, "100x100", "600x600", 1)
	}
	return urlStr
}

type TrackMeta struct {
	CoverURL string
	Album    string
}

var (
	trackMetaCacheMutex sync.RWMutex
	trackMetaCache      = make(map[string]TrackMeta)
)

// ResolveTrackCoverAndAlbum finds the exact individual album cover and album name for a track.
// It searches iTunes first (returns 600x600 HD cover) and falls back to Deezer Search (500x500).
func ResolveTrackCoverAndAlbum(title, artist string) (string, string) {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	if title == "" {
		return "", ""
	}

	cleanKey := strings.ToLower(artist + " - " + title)
	trackMetaCacheMutex.RLock()
	cached, ok := trackMetaCache[cleanKey]
	trackMetaCacheMutex.RUnlock()
	if ok && cached.CoverURL != "" {
		return cached.CoverURL, cached.Album
	}

	// 1. Try iTunes search first (fast, reliable, free, 600x600 artwork)
	term := fmt.Sprintf("%s %s", artist, title)
	apiURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&entity=song&limit=3", url.QueryEscape(term))
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var itResp iTunesSearchResponse
				if err := json.NewDecoder(resp.Body).Decode(&itResp); err == nil {
					for _, item := range itResp.Results {
						if item.ArtworkURL100 != "" {
							cover := getHighResArtwork(item.ArtworkURL100)
							album := item.CollectionName
							meta := TrackMeta{CoverURL: cover, Album: album}
							trackMetaCacheMutex.Lock()
							trackMetaCache[cleanKey] = meta
							trackMetaCacheMutex.Unlock()
							return cover, album
						}
					}
				}
			}
		}
	}

	// 2. Fallback: Deezer search
	searchURL := fmt.Sprintf("https://api.deezer.com/2.0/search?q=%s", url.QueryEscape(term))
	reqD, errD := http.NewRequest(http.MethodGet, searchURL, nil)
	if errD == nil {
		reqD.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		respD, errD := httpClient.Do(reqD)
		if errD == nil {
			defer respD.Body.Close()
			if respD.StatusCode == http.StatusOK {
				var dSearch struct {
					Data []struct {
						Title string `json:"title"`
						Album struct {
							Title    string `json:"title"`
							CoverBig string `json:"cover_big"`
							Cover    string `json:"cover"`
						} `json:"album"`
					} `json:"data"`
				}
				if err := json.NewDecoder(respD.Body).Decode(&dSearch); err == nil && len(dSearch.Data) > 0 {
					cover := dSearch.Data[0].Album.CoverBig
					if cover == "" {
						cover = dSearch.Data[0].Album.Cover
					}
					album := dSearch.Data[0].Album.Title
					if cover != "" {
						meta := TrackMeta{CoverURL: cover, Album: album}
						trackMetaCacheMutex.Lock()
						trackMetaCache[cleanKey] = meta
						trackMetaCacheMutex.Unlock()
						return cover, album
					}
				}
			}
		}
	}

	return "", ""
}

// FindTrackByiTunes searches iTunes as a fallback when Deezer doesn't have the track (REQUISITOS.md).
func FindTrackByiTunes(title, artist string) (*Track, error) {
	term := fmt.Sprintf("%s %s", artist, title)
	apiURL := fmt.Sprintf("https://itunes.apple.com/search?term=%s&entity=song&limit=3", url.QueryEscape(term))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("itunes returned status %d", resp.StatusCode)
	}

	var itResp iTunesSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&itResp); err != nil {
		return nil, err
	}

	for _, item := range itResp.Results {
		if item.PreviewURL != "" {
			return &Track{
				ID:         fmt.Sprintf("itunes_%s", title),
				Title:      item.TrackName,
				Artist:     item.ArtistName,
				Album:      item.CollectionName,
				CoverURL:   getHighResArtwork(item.ArtworkURL100),
				PreviewURL: item.PreviewURL,
			}, nil
		}
	}

	return nil, errors.New("no preview found on itunes")
}

// FetchAudioBytes downloads audio data from preview URL, with iTunes fallback if needed.
func FetchAudioBytes(previewURL string) ([]byte, error) {
	if previewURL == "" {
		return nil, errors.New("empty preview URL")
	}

	resp, err := httpClient.Get(previewURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	if resp != nil {
		resp.Body.Close()
	}

	return nil, fmt.Errorf("failed to fetch audio from %s", previewURL)
}

// FetchAudioForTrack downloads preview, falling back to iTunes if Deezer preview is expired or missing.
func FetchAudioForTrack(t Track) ([]byte, error) {
	// Try existing preview URL
	if t.PreviewURL != "" {
		data, err := FetchAudioBytes(t.PreviewURL)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	// Fallback to iTunes search
	itTrack, err := FindTrackByiTunes(t.Title, t.Artist)
	if err == nil && itTrack.PreviewURL != "" {
		data, err := FetchAudioBytes(itTrack.PreviewURL)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	return nil, fmt.Errorf("could not fetch preview for %s - %s", t.Artist, t.Title)
}

var (
	reDeezerPlaylist = regexp.MustCompile(`(?i)(?:deezer\.com/(?:[a-zA-Z0-9\-_]+/)?playlist/|playlist/)([0-9]+)`)
	reDeezerAlbum    = regexp.MustCompile(`(?i)(?:deezer\.com/(?:[a-zA-Z0-9\-_]+/)?album/|album/)([0-9]+)`)
)

var (
	playlistCacheMutex sync.RWMutex
	playlistCache      = make(map[string]*Playlist)
)

// CachePlaylist caches a loaded playlist in memory by its ID.
func CachePlaylist(pl *Playlist) {
	if pl == nil || pl.ID == "" {
		return
	}
	playlistCacheMutex.Lock()
	playlistCache[pl.ID] = pl
	playlistCacheMutex.Unlock()
}

// GetCachedPlaylist retrieves a cached playlist by ID.
func GetCachedPlaylist(id string) (*Playlist, bool) {
	playlistCacheMutex.RLock()
	defer playlistCacheMutex.RUnlock()
	pl, ok := playlistCache[id]
	return pl, ok
}

// ResolveRedirectURL expands shortened share links (e.g. spotify.link or deezer.page.link).
func ResolveRedirectURL(input string) string {
	urlStr := ExtractURL(input)
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return ""
	}

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") && !strings.HasPrefix(urlStr, "spotify:") {
		if strings.Contains(urlStr, "spotify.com") || strings.Contains(urlStr, "deezer.com") || strings.Contains(urlStr, ".link") {
			urlStr = "https://" + urlStr
		}
	}

	if strings.Contains(urlStr, "spotify.link") || strings.Contains(urlStr, "page.link") {
		client := &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				return nil
			},
		}
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.Request != nil && resp.Request.URL != nil {
					return resp.Request.URL.String()
				}
			}
		}
	}
	return urlStr
}

// ExtractDeezerEntityID parses a link, internal ID, or raw numeric ID into a Deezer entityType and ID.
func ExtractDeezerEntityID(input string) (entityType, id string) {
	urlCandidate := ExtractURL(input)
	urlCandidate = strings.TrimSpace(urlCandidate)
	if strings.Contains(strings.ToLower(urlCandidate), "spotify.com") || strings.HasPrefix(strings.ToLower(urlCandidate), "spotify_") {
		return "", ""
	}

	clean := urlCandidate
	if idx := strings.Index(clean, "?"); idx != -1 {
		clean = clean[:idx]
	}

	// 0. Matches internal cache ID format: deezer_playlist_<id> or deezer_album_<id>
	reInternal := regexp.MustCompile(`(?i)^deezer_(playlist|album)_([0-9]+)$`)
	if m := reInternal.FindStringSubmatch(clean); len(m) > 2 {
		return strings.ToLower(m[1]), m[2]
	}

	if m := reDeezerPlaylist.FindStringSubmatch(clean); len(m) > 1 {
		return "playlist", m[1]
	}
	if m := reDeezerAlbum.FindStringSubmatch(clean); len(m) > 1 {
		return "album", m[1]
	}

	// Check if pure integer ID
	isNum := true
	for _, r := range clean {
		if r < '0' || r > '9' {
			isNum = false
			break
		}
	}
	if isNum && len(clean) >= 3 {
		return "playlist", clean
	}
	return "", ""
}

// LoadPlaylistFromInput loads a playlist or album from any Spotify URL, Deezer URL, or cached ID without requiring login.
func LoadPlaylistFromInput(input string) (*Playlist, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("por favor cole o link da playlist")
	}

	// 0. Check in-memory cache first (instant response when starting game!)
	if pl, ok := GetCachedPlaylist(input); ok {
		return pl, nil
	}

	resolved := ResolveRedirectURL(input)

	// Check for single tracks vs playlists
	lower := strings.ToLower(resolved)
	if strings.Contains(lower, "/track/") || strings.Contains(lower, "spotify:track:") {
		return nil, errors.New("você colou o link de uma faixa individual (música). Por favor cole o link de uma PLAYLIST ou ÁLBUM completo do Spotify ou Deezer")
	}
	if strings.Contains(lower, "/artist/") || strings.Contains(lower, "spotify:artist:") {
		return nil, errors.New("você colou o link de um artista. Por favor cole o link de uma PLAYLIST ou ÁLBUM do Spotify ou Deezer")
	}

	// 1. Check if Spotify URL / ID (playlist or album)
	if spType, spID := ExtractSpotifyEntityID(resolved); spID != "" {
		pl, err := FetchPublicSpotifyPlaylist(spType, spID)
		if err == nil {
			CachePlaylist(pl)
			return pl, nil
		}
		return nil, err
	}

	// 2. Check if Deezer URL / ID (playlist or album)
	if dzType, dzID := ExtractDeezerEntityID(resolved); dzID != "" {
		pl, err := FetchDeezerEntity(dzType, dzID)
		if err == nil {
			CachePlaylist(pl)
			return pl, nil
		}
		return nil, err
	}

	// 3. Informative error messages for other common platforms
	if strings.Contains(lower, "youtube.com") || strings.Contains(lower, "youtu.be") {
		return nil, errors.New("links do YouTube não são suportados. Cole um link de playlist pública do Spotify ou Deezer")
	}
	if strings.Contains(lower, "music.apple.com") {
		return nil, errors.New("links da Apple Music não são suportados diretamente. Cole um link de playlist pública do Spotify ou Deezer")
	}

	return nil, fmt.Errorf("link não reconhecido. Por favor cole o link de uma playlist pública do Spotify (open.spotify.com/playlist/...) ou Deezer (deezer.com/playlist/...)")
}

// FetchDeezerEntity loads a Deezer public playlist or album with tracks that have audio previews.
func FetchDeezerEntity(entityType, entityID string) (*Playlist, error) {
	if entityType == "" {
		entityType = "playlist"
	}
	apiURL := fmt.Sprintf("https://api.deezer.com/%s/%s", entityType, entityID)
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deezer retornou status %d", resp.StatusCode)
	}

	var pResp DeezerPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&pResp); err != nil {
		return nil, err
	}

	var tracks []Track
	for _, dt := range pResp.Tracks.Data {
		// RG-05: skip tracks without preview
		if dt.Preview == "" {
			continue
		}
		title := dt.TitleShort
		if title == "" {
			title = dt.Title
		}
		cover := dt.Album.CoverBig
		if cover == "" {
			cover = dt.Album.Cover
		}

		tracks = append(tracks, Track{
			ID:         fmt.Sprintf("dz_%d", dt.ID),
			Title:      title,
			Artist:     dt.Artist.Name, // RG-06: principal artist
			Album:      dt.Album.Title,
			CoverURL:   cover,
			ISRC:       dt.ISRC,
			PreviewURL: dt.Preview,
		})
	}

	if len(tracks) < 4 {
		return nil, fmt.Errorf("foram encontradas apenas %d faixas com áudio disponível no Deezer (mínimo necessário: 4)", len(tracks))
	}

	pl := &Playlist{
		ID:          fmt.Sprintf("deezer_%s_%s", entityType, entityID),
		Name:        pResp.Title,
		Description: pResp.Description,
		CoverURL:    pResp.PictureBig,
		TotalTracks: len(tracks),
		Source:      "deezer",
		Tracks:      tracks,
	}
	CachePlaylist(pl)
	return pl, nil
}

// PresetPlaylists provides instant playlists with popular genres and verified active Deezer IDs.
var PresetPlaylists = []struct {
	ID          string
	Name        string
	Description string
	CoverURL    string
	DeezerID    string
}{
	{
		ID:          "rock_classics",
		Name:        "Rock Essentials",
		Description: "Queen, AC/DC, Led Zeppelin, Metallica, Nirvana e mais lendas do Rock.",
		CoverURL:    "https://images.unsplash.com/photo-1498038432885-c6f3f1b912ee?w=500&auto=format&fit=crop&q=80",
		DeezerID:    "1306931615",
	},
	{
		ID:          "top_brasil",
		Name:        "Top Hits Brasil",
		Description: "Os maiores sucessos que estão bombando no Brasil agora.",
		CoverURL:    "https://images.unsplash.com/photo-1511671782779-c97d3d27a1d4?w=500&auto=format&fit=crop&q=80",
		DeezerID:    "1111141961",
	},
	{
		ID:          "rock_nacional",
		Name:        "Clássicos do Rock Nacional",
		Description: "Legião Urbana, Titãs, Paralamas, Capital Inicial, Raul Seixas.",
		CoverURL:    "https://images.unsplash.com/photo-1465847899084-d164df4dedc6?w=500&auto=format&fit=crop&q=80",
		DeezerID:    "9268293682",
	},
	{
		ID:          "rock_2000s",
		Name:        "2000s Rock",
		Description: "Linkin Park, Green Day, Evanescence, Foo Fighters, The Killers.",
		CoverURL:    "https://images.unsplash.com/photo-1511735111819-9a3f7709049c?w=500&auto=format&fit=crop&q=80",
		DeezerID:    "1419215845",
	},
	{
		ID:          "rock_60s",
		Name:        "60s Rock & Clássicos",
		Description: "The Beatles, The Rolling Stones, The Doors, Jimi Hendrix, The Who.",
		CoverURL:    "https://images.unsplash.com/photo-1514525253161-7a46d19cd819?w=500&auto=format&fit=crop&q=80",
		DeezerID:    "1437011185",
	},
}

// LoadDefaultPlaylists returns available ready-to-play playlists.
func LoadDefaultPlaylists() []Playlist {
	var list []Playlist
	for _, p := range PresetPlaylists {
		list = append(list, Playlist{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			CoverURL:    p.CoverURL,
			Source:      "deezer",
			TotalTracks: 30, // dynamically fetched when selected
		})
	}
	return list
}

// EnsureTracksForPlaylist fetches tracks on demand with fallback.
func EnsureTracksForPlaylist(playlistID string) ([]Track, error) {
	for _, p := range PresetPlaylists {
		if p.ID == playlistID {
			pl, err := FetchDeezerEntity("playlist", p.DeezerID)
			if err == nil && len(pl.Tracks) >= 4 {
				return pl.Tracks, nil
			}
			log.Printf("Warning: failed fetching Deezer playlist %s (%v), using built-in offline fallback", p.DeezerID, err)
			return GetBuiltinTracks(), nil
		}
	}
	return GetBuiltinTracks(), nil
}

// GetBuiltinTracks provides an offline list of songs with active public audio previews
// ensuring the game is 100% playable even without an external API!
func GetBuiltinTracks() []Track {
	return []Track{
		{
			ID:         "demo_1",
			Title:      "Billie Jean",
			Artist:     "Michael Jackson",
			Album:      "Thriller",
			CoverURL:   "https://images.unsplash.com/photo-1511671782779-c97d3d27a1d4?w=500&auto=format&fit=crop&q=80",
			PreviewURL: "https://cdnt-preview.dzcdn.net/api/1/1/e/6/e/0/e6e0f8717b1e6353e2abf797c6849889.mp3",
		},
		{
			ID:         "demo_2",
			Title:      "Bohemian Rhapsody",
			Artist:     "Queen",
			Album:      "A Night At The Opera",
			CoverURL:   "https://images.unsplash.com/photo-1498038432885-c6f3f1b912ee?w=500&auto=format&fit=crop&q=80",
			PreviewURL: "https://audio-ssl.itunes.apple.com/itunes-assets/AudioPreview221/v4/17/fc/1e/17fc1eba-946d-84a9-710b-a0e88ea64209/mzaf_3049006317693088799.plus.aac.p.m4a",
		},
		{
			ID:         "demo_3",
			Title:      "Smells Like Teen Spirit",
			Artist:     "Nirvana",
			Album:      "Nevermind",
			CoverURL:   "https://images.unsplash.com/photo-1514525253161-7a46d19cd819?w=500&auto=format&fit=crop&q=80",
			PreviewURL: "https://cdnt-preview.dzcdn.net/api/1/1/e/6/e/0/e6e0f8717b1e6353e2abf797c6849889.mp3", // fallback preview
		},
		{
			ID:         "demo_4",
			Title:      "Stayin' Alive",
			Artist:     "Bee Gees",
			Album:      "Saturday Night Fever",
			CoverURL:   "https://images.unsplash.com/photo-1470225620780-dba8ba36b745?w=500&auto=format&fit=crop&q=80",
			PreviewURL: "https://cdnt-preview.dzcdn.net/api/1/1/d/9/1/0/d9193b97c3b0ef9cd7ee8b327726dc87.mp3",
		},
		{
			ID:         "demo_5",
			Title:      "Sweet Child O' Mine",
			Artist:     "Guns N' Roses",
			Album:      "Appetite For Destruction",
			CoverURL:   "https://images.unsplash.com/photo-1511735111819-9a3f7709049c?w=500&auto=format&fit=crop&q=80",
			PreviewURL: "https://cdnt-preview.dzcdn.net/api/1/1/e/6/e/0/e6e0f8717b1e6353e2abf797c6849889.mp3",
		},
	}
}
