package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"
)

// CalculateSpeedPoints implements P(t) = 1000 - 900 * (t / 120)
// t in seconds since round start (linear from 1000 at t=0 to 100 at t=120).
func CalculateSpeedPoints(elapsedSeconds float64) int {
	if elapsedSeconds < 0 {
		elapsedSeconds = 0
	}
	if elapsedSeconds > 120 {
		elapsedSeconds = 120
	}
	pts := 1000.0 - (900.0 * (elapsedSeconds / 120.0))
	return int(math.Round(pts))
}

type ActiveRound struct {
	RoundNumber    int
	TotalRounds    int
	ClipSeconds    int
	StartsAt       time.Time
	EndsAt         time.Time
	TargetTrack    Track
	InternalOpts   []InternalOption
	PublicOpts     []PublicOption
	AudioToken     string
	Finished       bool
	HasHitBand     bool
	BandPoints     int
	TotalPoints    int
	AttemptsCount  int
	History        []AttemptLog
	RoundAnswer    *RoundAnswer
}

type GameManager struct {
	mu           sync.Mutex
	Config       GameConfig
	TracksPool   []Track
	TargetTracks []Track
	CurrentIndex int
	CurrentRound *ActiveRound
	TotalScore   int
	CorrectSongs int
	PastAnswers  []RoundAnswer
	IsActive     bool
}

var GlobalGame = &GameManager{}

// StartNewGame initializes a game according to GameConfig.
func (gm *GameManager) StartNewGame(cfg GameConfig, tracks []Track) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if len(tracks) < 4 {
		return errors.New("a playlist precisa de pelo menos 4 músicas com prévia")
	}

	// Validate clip seconds (CF-01)
	validClips := map[int]bool{5: true, 10: true, 15: true, 20: true, 30: true}
	if !validClips[cfg.ClipSeconds] {
		cfg.ClipSeconds = 10
	}

	// Validate rounds (CF-02, CF-03)
	if cfg.Rounds < 10 {
		cfg.Rounds = 10
	}
	if cfg.Rounds > 50 {
		cfg.Rounds = 50
	}
	// CF-03: Limit rounds to available tracks
	if cfg.Rounds > len(tracks) {
		cfg.Rounds = len(tracks)
	}

	// RG-07: Shuffle tracks so no song repeats in the same match
	shuffled := make([]Track, len(tracks))
	copy(shuffled, tracks)
	shuffleTracks(shuffled)

	gm.Config = cfg
	gm.TracksPool = tracks
	gm.TargetTracks = shuffled[:cfg.Rounds]
	gm.CurrentIndex = 0
	gm.TotalScore = 0
	gm.CorrectSongs = 0
	gm.PastAnswers = make([]RoundAnswer, 0)
	gm.IsActive = true
	gm.CurrentRound = nil

	return nil
}

// NextRound prepares and starts the next round.
func (gm *GameManager) NextRound() (*RoundPublicState, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if !gm.IsActive {
		return nil, errors.New("jogo não está ativo")
	}

	if gm.CurrentIndex >= len(gm.TargetTracks) {
		gm.IsActive = false
		return nil, errors.New("todas as rodadas foram concluídas")
	}

	target := gm.TargetTracks[gm.CurrentIndex]

	// Cache audio if not already cached
	var audioToken string
	if target.AudioToken != "" {
		audioToken = target.AudioToken
	} else {
		// Download preview audio with fallback
		data, err := FetchAudioForTrack(target)
		if err != nil {
			logPrintf("Error downloading audio preview for %s (%s): %v", target.Title, target.PreviewURL, err)
		}
		audioToken = StoreAudio(data)
		target.AudioToken = audioToken
		gm.TargetTracks[gm.CurrentIndex].AudioToken = audioToken
	}

	// Pre-resolve individual track album cover & album name in background if needed
	go func(title, artist string) {
		ResolveTrackCoverAndAlbum(title, artist)
	}(target.Title, target.Artist)

	now := time.Now()
	// RD-01: 2 minutes duration
	endsAt := now.Add(120 * time.Second)

	round := &ActiveRound{
		RoundNumber:   gm.CurrentIndex + 1,
		TotalRounds:   len(gm.TargetTracks),
		ClipSeconds:   gm.Config.ClipSeconds,
		StartsAt:      now,
		EndsAt:        endsAt,
		TargetTrack:   target,
		AudioToken:    audioToken,
		Finished:      false,
		HasHitBand:    false,
		BandPoints:    0,
		TotalPoints:   0,
		AttemptsCount: 0,
		History:       make([]AttemptLog, 0),
	}

	// If Mode 1, generate multiple choice options (M1-01 to M1-03)
	if gm.Config.Mode == 1 {
		internalOpts, publicOpts := gm.generateMode1Options(target)
		round.InternalOpts = internalOpts
		round.PublicOpts = publicOpts
	}

	gm.CurrentRound = round
	gm.CurrentIndex++

	return gm.toPublicState(round), nil
}

// generateMode1Options satisfies M1-01, M1-02, M1-03:
// 4 options: 1 correct, 3 distractors from same playlist.
// Max ONE band repeated once (2 options of same band, different songs).
func (gm *GameManager) generateMode1Options(target Track) ([]InternalOption, []PublicOption) {
	correctOpt := InternalOption{
		Label:         fmt.Sprintf("%s — %s", target.Title, target.Artist),
		TrackID:       target.ID,
		IsCorrect:     true,
		IsCorrectBand: true,
	}

	// Candidate distractors (excluding target song)
	var candidates []Track
	for _, t := range gm.TracksPool {
		if t.ID != target.ID && t.Title != target.Title {
			candidates = append(candidates, t)
		}
	}
	shuffleTracks(candidates)

	// Pick 3 distractors adhering to M1-03:
	// "com no máximo uma banda repetida uma vez (2 opções da mesma banda, músicas diferentes)"
	pickedDistractors := make([]Track, 0, 3)
	bandCounts := make(map[string]int)
	bandCounts[NormalizeBand(target.Artist)] = 1

	for _, cand := range candidates {
		if len(pickedDistractors) == 3 {
			break
		}
		normB := NormalizeBand(cand.Artist)
		currCount := bandCounts[normB]

		// If this band would become count 2:
		// Check how many bands already have count 2
		if currCount == 1 {
			hasDuplicate := false
			for _, c := range bandCounts {
				if c >= 2 {
					hasDuplicate = true
					break
				}
			}
			if hasDuplicate {
				// Can't have more than one band repeated
				continue
			}
		} else if currCount >= 2 {
			// Never 3 of the same band
			continue
		}

		pickedDistractors = append(pickedDistractors, cand)
		bandCounts[normB]++
	}

	// Fallback if not enough distinct distractors were picked
	if len(pickedDistractors) < 3 {
		for _, cand := range candidates {
			if len(pickedDistractors) == 3 {
				break
			}
			alreadyIn := false
			for _, p := range pickedDistractors {
				if p.ID == cand.ID {
					alreadyIn = true
					break
				}
			}
			if !alreadyIn {
				pickedDistractors = append(pickedDistractors, cand)
			}
		}
	}

	allOpts := []InternalOption{correctOpt}
	targetNormBand := NormalizeBand(target.Artist)

	for _, d := range pickedDistractors {
		isSameBand := NormalizeBand(d.Artist) == targetNormBand
		allOpts = append(allOpts, InternalOption{
			Label:         fmt.Sprintf("%s — %s", d.Title, d.Artist),
			TrackID:       d.ID,
			IsCorrect:     false,
			IsCorrectBand: isSameBand,
		})
	}

	// Shuffle options
	for i := range allOpts {
		j := cryptoRandInt(len(allOpts))
		allOpts[i], allOpts[j] = allOpts[j], allOpts[i]
	}

	// Assign random IDs (e.g. o1, o2, o3, o4)
	publicOpts := make([]PublicOption, len(allOpts))
	for i := range allOpts {
		id := fmt.Sprintf("o%d", i+1)
		allOpts[i].ID = id
		publicOpts[i] = PublicOption{
			ID:       id,
			Label:    allOpts[i].Label,
			Disabled: false,
		}
	}

	return allOpts, publicOpts
}

// HandleChoose evaluates a Mode 1 pick (M1-04 to M1-08).
func (gm *GameManager) HandleChoose(optionID string) (*RoundPublicState, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	round := gm.CurrentRound
	if round == nil || round.Finished {
		return nil, errors.New("rodada já finalizada ou inválida")
	}

	var chosen *InternalOption
	var chosenIdx int
	for i := range round.InternalOpts {
		if round.InternalOpts[i].ID == optionID {
			chosen = &round.InternalOpts[i]
			chosenIdx = i
			break
		}
	}

	if chosen == nil {
		return nil, errors.New("opção não encontrada")
	}

	if round.PublicOpts[chosenIdx].Disabled {
		return nil, errors.New("opção já utilizada")
	}

	now := time.Now()
	elapsed := now.Sub(round.StartsAt).Seconds()
	pt := CalculateSpeedPoints(elapsed)

	if chosen.IsCorrect {
		// M1-04 & M1-06:
		// Acertar a música: se já acertou banda, max(bandPoints, P(t))
		total := pt
		if round.HasHitBand && round.BandPoints > total {
			total = round.BandPoints
		}
		round.TotalPoints = total
		round.Finished = true
		gm.TotalScore += round.TotalPoints
		gm.CorrectSongs++
		gm.recordAnswer(round)
	} else if chosen.IsCorrectBand {
		// M1-05: Acertar só a banda -> 40% * P(t). Opção bloqueada e pode tentar de novo!
		round.PublicOpts[chosenIdx].Disabled = true
		if !round.HasHitBand {
			round.HasHitBand = true
			round.BandPoints = int(math.Round(0.4 * float64(pt)))
			round.TotalPoints = round.BandPoints
		}
		// Does NOT finish round
	} else {
		// M1-08: Errar banda e música encerra a rodada com 0 pontos (ou retém pontos da banda se já tinha)
		round.Finished = true
		round.PublicOpts[chosenIdx].Disabled = true
		round.TotalPoints = round.BandPoints // 0 if no band hit earlier
		gm.TotalScore += round.TotalPoints
		gm.recordAnswer(round)
	}

	return gm.toPublicState(round), nil
}

// HandleGuess evaluates a Mode 2 guess (M2-01 to M2-09).
func (gm *GameManager) HandleGuess(rawGuess string) (*RoundPublicState, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	round := gm.CurrentRound
	if round == nil || round.Finished {
		return nil, errors.New("rodada já finalizada ou inválida")
	}

	round.AttemptsCount++
	now := time.Now()
	elapsed := now.Sub(round.StartsAt).Seconds()
	pt := CalculateSpeedPoints(elapsed)

	// Mode 2 speed & attempt penalty formula (M2-08, M2-09):
	// pontos_música = P(t) * max(0.5, 1 - 0.05 * (attempts - 1))
	attemptMult := math.Max(0.5, 1.0-(0.05*float64(round.AttemptsCount-1)))
	potentialSongPoints := int(math.Round(float64(pt) * attemptMult))

	comp := CompareGuess(rawGuess, round.TargetTrack.Title, round.TargetTrack.Artist)

	logItem := AttemptLog{
		AttemptNumber: round.AttemptsCount,
		Normalized:    comp.NormalizedGuess,
		Percent:       comp.BestPercent,
		Result:        string(comp.Result),
	}
	round.History = append(round.History, logItem)

	if comp.Result == MatchResultSong {
		// Hit song!
		total := potentialSongPoints
		if round.HasHitBand && round.BandPoints > total {
			total = round.BandPoints
		}
		round.TotalPoints = total
		round.Finished = true
		gm.TotalScore += round.TotalPoints
		gm.CorrectSongs++
		gm.recordAnswer(round)
	} else if comp.Result == MatchResultBand {
		// Hit band!
		if !round.HasHitBand {
			round.HasHitBand = true
			round.BandPoints = int(math.Round(0.4 * float64(potentialSongPoints)))
			round.TotalPoints = round.BandPoints
		}
	}

	return gm.toPublicState(round), nil
}

// HandleReveal handles giving up on the current round (RD-04, RD-05).
func (gm *GameManager) HandleReveal() (*RoundPublicState, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	round := gm.CurrentRound
	if round == nil || round.Finished {
		return nil, errors.New("rodada já finalizada")
	}

	round.Finished = true
	round.TotalPoints = round.BandPoints // keeps band points if already hit, otherwise 0
	gm.TotalScore += round.TotalPoints
	gm.recordAnswer(round)

	return gm.toPublicState(round), nil
}

// HandleTimeout finishes the round if time has expired (RD-08).
func (gm *GameManager) HandleTimeout() (*RoundPublicState, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	round := gm.CurrentRound
	if round == nil || round.Finished {
		return nil, errors.New("rodada já finalizada")
	}

	round.Finished = true
	round.TotalPoints = round.BandPoints
	gm.TotalScore += round.TotalPoints
	gm.recordAnswer(round)

	return gm.toPublicState(round), nil
}

func (gm *GameManager) recordAnswer(round *ActiveRound) {
	cover := round.TargetTrack.CoverURL
	album := round.TargetTrack.Album

	// Guarantee individual cover & real album: if missing or still equal to playlist name/fallback
	if cover == "" || album == "" || (gm.Config.PlaylistName != "" && album == gm.Config.PlaylistName) {
		resCover, resAlbum := ResolveTrackCoverAndAlbum(round.TargetTrack.Title, round.TargetTrack.Artist)
		if resCover != "" {
			cover = resCover
			round.TargetTrack.CoverURL = resCover
		}
		if resAlbum != "" {
			album = resAlbum
			round.TargetTrack.Album = resAlbum
		}
	}

	ans := &RoundAnswer{
		Title:      round.TargetTrack.Title,
		Artist:     round.TargetTrack.Artist,
		Album:      album,
		CoverURL:   cover,
		SpotifyURL: round.TargetTrack.SpotifyURL,
	}
	round.RoundAnswer = ans
	gm.PastAnswers = append(gm.PastAnswers, *ans)
}

func (gm *GameManager) toPublicState(round *ActiveRound) *RoundPublicState {
	st := &RoundPublicState{
		RoundNumber:    round.RoundNumber,
		TotalRounds:    round.TotalRounds,
		ClipSeconds:    round.ClipSeconds,
		AudioURL:       "/audio/" + round.AudioToken,
		Options:        round.PublicOpts,
		Finished:       round.Finished,
		HasHitBand:     round.HasHitBand,
		BandPoints:     round.BandPoints,
		RoundPoints:    round.TotalPoints,
		AttemptsCount:  round.AttemptsCount,
		History:        round.History,
		TotalScore:     gm.TotalScore,
		CorrectSongs:   gm.CorrectSongs,
		DurationMs:     120000,
		StartsAtUnixMs: round.StartsAt.UnixMilli(),
		EndsAtUnixMs:   round.EndsAt.UnixMilli(),
	}

	if round.Finished {
		st.Answer = round.RoundAnswer
	}

	return st
}

// GetSummary returns game completion stats and podium ranking.
func (gm *GameManager) GetSummary() *GameSummary {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	totalRounds := len(gm.TargetTracks)
	if totalRounds == 0 {
		totalRounds = 1
	}

	acc := int(math.Round(float64(gm.CorrectSongs) / float64(totalRounds) * 100))

	rank := "Ouvinte Casual 🎧"
	if acc >= 90 {
		rank = "Enciclopédia Musical 🏆"
	} else if acc >= 75 {
		rank = "Mestre do Ritmo 🎸"
	} else if acc >= 50 {
		rank = "Fã Conhecedor 🎵"
	} else if acc >= 25 {
		rank = "Bom de Ouvido 🎶"
	}

	pName := gm.Config.PlayerName
	if pName == "" {
		pName = "Jogador"
	}

	return &GameSummary{
		PlayerName:      pName,
		TotalScore:      gm.TotalScore,
		CorrectSongs:    gm.CorrectSongs,
		TotalRounds:     totalRounds,
		Mode:            gm.Config.Mode,
		TitleRank:       rank,
		AccuracyPercent: acc,
		RoundsSummary:   gm.PastAnswers,
	}
}

func shuffleTracks(tracks []Track) {
	for i := range tracks {
		j := cryptoRandInt(len(tracks))
		tracks[i], tracks[j] = tracks[j], tracks[i]
	}
}

func cryptoRandInt(n int) int {
	if n <= 1 {
		return 0
	}
	bi, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(bi.Int64())
}

func logPrintf(format string, v ...any) {
	// helper
	_ = format
	_ = v
}
