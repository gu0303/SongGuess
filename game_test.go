package main

import (
	"testing"
	"time"
)

func TestCalculateSpeedPoints(t *testing.T) {
	// P(t) = 1000 - 900 * (t / 120)
	// at t=0s -> 1000
	if pts := CalculateSpeedPoints(0); pts != 1000 {
		t.Errorf("At 0s expected 1000, got %d", pts)
	}

	// at t=10s -> 1000 - 900 * (10/120) = 1000 - 75 = 925 (conforme REQUISITOS.md tabela)
	if pts := CalculateSpeedPoints(10); pts != 925 {
		t.Errorf("At 10s expected 925, got %d", pts)
	}

	// at t=60s -> 1000 - 900 * 0.5 = 550 (conforme REQUISITOS.md tabela)
	if pts := CalculateSpeedPoints(60); pts != 550 {
		t.Errorf("At 60s expected 550, got %d", pts)
	}

	// at t=120s -> 100
	if pts := CalculateSpeedPoints(120); pts != 100 {
		t.Errorf("At 120s expected 100, got %d", pts)
	}
}

func TestMode1OptionGeneration(t *testing.T) {
	tracks := []Track{
		{ID: "1", Title: "Song 1", Artist: "Band A"},
		{ID: "2", Title: "Song 2", Artist: "Band A"},
		{ID: "3", Title: "Song 3", Artist: "Band B"},
		{ID: "4", Title: "Song 4", Artist: "Band C"},
		{ID: "5", Title: "Song 5", Artist: "Band D"},
	}

	gm := &GameManager{TracksPool: tracks}
	internal, public := gm.generateMode1Options(tracks[0])

	if len(public) != 4 || len(internal) != 4 {
		t.Fatalf("Expected 4 options, got %d public and %d internal", len(public), len(internal))
	}

	hasCorrect := false
	bandCounts := make(map[string]int)

	for _, opt := range internal {
		if opt.IsCorrect {
			hasCorrect = true
		}
		// Extract artist
		for _, tr := range tracks {
			if tr.ID == opt.TrackID {
				bandCounts[tr.Artist]++
				break
			}
		}
	}

	if !hasCorrect {
		t.Errorf("Mode 1 options missing correct option")
	}

	// Check M1-03: at most ONE band repeated once (max count 2, and at most one such band)
	repeatCount := 0
	for _, count := range bandCounts {
		if count > 2 {
			t.Errorf("Band repeated more than once: %d", count)
		}
		if count == 2 {
			repeatCount++
		}
	}
	if repeatCount > 1 {
		t.Errorf("More than one band repeated: %d", repeatCount)
	}
}

func TestGameFlow(t *testing.T) {
	tracks := []Track{
		{ID: "1", Title: "Song A", Artist: "Artist 1", PreviewURL: "mock"},
		{ID: "2", Title: "Song B", Artist: "Artist 2", PreviewURL: "mock"},
		{ID: "3", Title: "Song C", Artist: "Artist 3", PreviewURL: "mock"},
		{ID: "4", Title: "Song D", Artist: "Artist 4", PreviewURL: "mock"},
	}

	cfg := GameConfig{
		Mode:        1,
		ClipSeconds: 10,
		Rounds:      10, // should adjust to 4 (len(tracks))
		PlayerName:  "Tester",
	}

	gm := &GameManager{}
	err := gm.StartNewGame(cfg, tracks)
	if err != nil {
		t.Fatalf("Failed starting game: %v", err)
	}

	if len(gm.TargetTracks) != 4 {
		t.Errorf("Expected 4 rounds (capped to track count), got %d", len(gm.TargetTracks))
	}

	round, err := gm.NextRound()
	if err != nil {
		t.Fatalf("Failed next round: %v", err)
	}
	if round.RoundNumber != 1 {
		t.Errorf("Expected round 1, got %d", round.RoundNumber)
	}

	// Find the correct option in internal options to test choose
	correctID := ""
	for _, opt := range gm.CurrentRound.InternalOpts {
		if opt.IsCorrect {
			correctID = opt.ID
			break
		}
	}

	gm.CurrentRound.StartsAt = time.Now().Add(-10 * time.Second) // simulate 10s elapsed
	res, err := gm.HandleChoose(correctID)
	if err != nil {
		t.Fatalf("Choose failed: %v", err)
	}
	if !res.Finished {
		t.Errorf("Round should be finished on correct song pick")
	}
	if res.RoundPoints != 925 {
		t.Errorf("Expected 925 points at 10s, got %d", res.RoundPoints)
	}
	if gm.TotalScore != 925 || gm.CorrectSongs != 1 {
		t.Errorf("Game score mismatch: score=%d, correct=%d", gm.TotalScore, gm.CorrectSongs)
	}
}
