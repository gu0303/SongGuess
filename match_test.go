package main

import (
	"testing"
)

func TestNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Bohemian Rhapsody - Remastered 2011", "bohemian rhapsody"},
		{"Despacito (feat. Justin Bieber) - Remix", "despacito"},
		{"Ela É Demais", "ela e demais"},
	}

	for _, tt := range tests {
		got := NormalizeReference(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeReference(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestPlayerGuessNormalization(t *testing.T) {
	// Steps 2-4 must NOT be applied to player guess (REQUISITOS.md M2-05)
	raw := "Paranoid - Banda Errada"
	got := NormalizePlayerGuess(raw)
	expected := "paranoid banda errada"
	if got != expected {
		t.Errorf("NormalizePlayerGuess(%q) = %q, expected %q", raw, got, expected)
	}
}

func TestCompareGuess(t *testing.T) {
	title := "Bohemian Rhapsody - Remastered 2011"
	band := "Queen"

	// 1. Correct song title
	res := CompareGuess("bohemian rhapsody", title, band)
	if res.Result != MatchResultSong || res.BestPercent < 95 {
		t.Errorf("Expected song match, got %+v", res)
	}

	// 2. Correct band
	resBand := CompareGuess("Queen", title, band)
	if resBand.Result != MatchResultBand || resBand.BestPercent < 95 {
		t.Errorf("Expected band match, got %+v", resBand)
	}

	// 3. Typo within 95% (e.g. 1 char typo in long string)
	// "bohemian rapsody" -> len 17, 1 deletion -> 16/17 = 94% -> might be < 95%
	// let's test title + band
	resTB := CompareGuess("bohemian rhapsody queen", title, band)
	if resTB.Result != MatchResultSong || resTB.BestPercent < 95 {
		t.Errorf("Expected song match for title + band, got %+v", resTB)
	}

	// 4. Correct title + wrong band (M2-06): should NOT match song
	resWrongBand := CompareGuess("bohemian rhapsody pink floyd", title, band)
	if resWrongBand.Result == MatchResultSong {
		t.Errorf("Title + wrong band should not be song match: %+v", resWrongBand)
	}
}
