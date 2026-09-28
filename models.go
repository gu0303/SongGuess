package main

// Track represents a music track in the game.
// Banda = main artist (first item in artists list). Guest artists are ignored (RG-06).
type Track struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"` // Main artist (RG-06)
	Album      string `json:"album"`
	CoverURL   string `json:"coverUrl"`
	ISRC       string `json:"isrc,omitempty"`
	SpotifyURL string `json:"spotifyUrl,omitempty"`
	PreviewURL string `json:"previewUrl,omitempty"`
	AudioToken string `json:"audioToken,omitempty"` // Obfuscated token for /audio/{token} (RG-04)
}

// Playlist represents a playlist selectable by the user.
type Playlist struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	CoverURL    string  `json:"coverUrl,omitempty"`
	TotalTracks int     `json:"totalTracks"`
	Source      string  `json:"source"` // "spotify" or "deezer" or "demo"
	Tracks      []Track `json:"-"`
}

// GameConfig defines match options set by the host (CF-01, CF-02, CF-03).
type GameConfig struct {
	Mode         int    `json:"mode"`        // 1 = Múltipla Escolha, 2 = Digitação
	ClipSeconds  int    `json:"clipSeconds"` // 5, 10, 15, 20, 30
	Rounds       int    `json:"rounds"`      // 10 to 50
	PlaylistID   string `json:"playlistId"`
	PlaylistName string `json:"playlistName"`
	PlayerName   string `json:"playerName"`
}

// PublicOption represents one of 4 multiple choice options in Mode 1 (M1-01).
// Random ID per round prevents cheating (PROTOCOLO.md 1.5).
type PublicOption struct {
	ID       string `json:"id"`       // e.g. "o1", "o2"
	Label    string `json:"label"`    // "Música — Banda"
	Disabled bool   `json:"disabled"` // True if already picked (e.g. partial hit)
}

// InternalOption holds server-side evaluation flags for Mode 1.
type InternalOption struct {
	ID            string
	Label         string
	TrackID       string
	IsCorrect     bool
	IsCorrectBand bool
}

// RoundResultType defines how the round ended for the player.
type RoundResultType string

const (
	ResultTypeSong   RoundResultType = "song"
	ResultTypeBand   RoundResultType = "band"
	ResultTypeWrong  RoundResultType = "wrong"
	ResultTypeGaveUp RoundResultType = "gave_up"
	ResultTypeTime   RoundResultType = "timeout"
)

// AttemptLog records an attempt in Mode 2.
type AttemptLog struct {
	AttemptNumber int    `json:"attemptNumber"`
	Normalized    string `json:"normalized"`
	Percent       int    `json:"percent"`
	Result        string `json:"result"` // "song", "band", "none"
}

// RoundAnswer is revealed only after the round ends (RG-04, RD-07, RD-08).
type RoundAnswer struct {
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	CoverURL   string `json:"coverUrl"`
	SpotifyURL string `json:"spotifyUrl"`
}

// RoundPublicState is sent to the client during a round.
type RoundPublicState struct {
	RoundNumber      int            `json:"roundNumber"`
	TotalRounds      int            `json:"totalRounds"`
	ClipSeconds      int            `json:"clipSeconds"`
	AudioURL         string         `json:"audioUrl"` // /audio/{token}
	Options          []PublicOption `json:"options,omitempty"`
	Finished         bool           `json:"finished"`
	HasHitBand       bool           `json:"hasHitBand"`
	BandPoints       int            `json:"bandPoints"`
	RoundPoints      int            `json:"roundPoints"`
	AttemptsCount    int            `json:"attemptsCount"`
	History          []AttemptLog   `json:"history,omitempty"`
	Answer           *RoundAnswer   `json:"answer,omitempty"` // populated only when Finished == true
	TotalScore       int            `json:"totalScore"`
	CorrectSongs     int            `json:"correctSongs"`
	DurationMs       int64          `json:"durationMs"`
	StartsAtUnixMs   int64          `json:"startsAtUnixMs"`
	EndsAtUnixMs     int64          `json:"endsAtUnixMs"`
}

// GameSummary represents the final results / podium (PL-03, PL-05, PL-06).
type GameSummary struct {
	PlayerName       string        `json:"playerName"`
	TotalScore       int           `json:"totalScore"`
	CorrectSongs     int           `json:"correctSongs"`
	TotalRounds      int           `json:"totalRounds"`
	Mode             int           `json:"mode"`
	TitleRank        string        `json:"titleRank"`
	AccuracyPercent  int           `json:"accuracyPercent"`
	RoundsSummary    []RoundAnswer `json:"roundsSummary"`
}
