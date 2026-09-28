package main

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

var (
	reParens  = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]`)
	reDash    = regexp.MustCompile(`\s+-\s+.*$`)
	reFeat    = regexp.MustCompile(`(?i)\b(feat\.|ft\.|featuring)\b.*$`)
	rePunct   = regexp.MustCompile(`[^\p{L}\p{N}\s]`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// stripAccents replaces accented unicode characters with their ASCII equivalents.
func stripAccents(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ã', 'â', 'ä', 'å', 'ā':
			b.WriteRune('a')
		case 'é', 'è', 'ê', 'ë', 'ē':
			b.WriteRune('e')
		case 'í', 'ì', 'î', 'ï', 'ī':
			b.WriteRune('i')
		case 'ó', 'ò', 'õ', 'ô', 'ö', 'ø', 'ō':
			b.WriteRune('o')
		case 'ú', 'ù', 'û', 'ü', 'ū':
			b.WriteRune('u')
		case 'ç', 'ć', 'č':
			b.WriteRune('c')
		case 'ñ', 'ń':
			b.WriteRune('n')
		case 'ý', 'ÿ':
			b.WriteRune('y')
		case 'ß':
			b.WriteString("ss")
		case 'æ':
			b.WriteString("ae")
		default:
			// General check for non-spacing mark if decomposed
			if unicode.Is(unicode.Mn, r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizePlayerGuess applies steps 1 and 5 (M2-05):
// 1. Minúsculas e remoção de acentos
// 5. Remover pontuação e espaços duplicados
func NormalizePlayerGuess(s string) string {
	s = strings.ToLower(s)
	s = stripAccents(s)
	s = rePunct.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// NormalizeReference applies steps 1 through 5 (M2-05) for target song titles:
// 1. Minúsculas e remoção de acentos
// 2. Remover (...) e [...]
// 3. Remover tudo após " - " (traço com espaços)
// 4. Remover feat., ft., featuring e o que vier depois
// 5. Remover pontuação e espaços duplicados
func NormalizeReference(s string) string {
	s = strings.ToLower(s)
	s = stripAccents(s)
	s = reParens.ReplaceAllString(s, "")
	s = reDash.ReplaceAllString(s, "")
	s = reFeat.ReplaceAllString(s, "")
	s = rePunct.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// NormalizeBand applies steps 1 and 5 to the main artist name.
func NormalizeBand(s string) string {
	return NormalizePlayerGuess(s)
}

// LevenshteinDistance calculates the Levenshtein distance between two rune slices.
func LevenshteinDistance(a, b []rune) int {
	lenA := len(a)
	lenB := len(b)
	if lenA == 0 {
		return lenB
	}
	if lenB == 0 {
		return lenA
	}

	prev := make([]int, lenB+1)
	curr := make([]int, lenB+1)

	for j := 0; j <= lenB; j++ {
		prev[j] = j
	}

	for i := 1; i <= lenA; i++ {
		curr[0] = i
		for j := 1; j <= lenB; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost

			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		copy(prev, curr)
	}

	return prev[lenB]
}

// CalculateSimilarity computes 1 - distance / max_len as a percentage (0 to 100).
func CalculateSimilarity(guess, target string) int {
	rGuess := []rune(guess)
	rTarget := []rune(target)

	maxLen := len(rGuess)
	if len(rTarget) > maxLen {
		maxLen = len(rTarget)
	}

	if maxLen == 0 {
		return 100
	}

	dist := LevenshteinDistance(rGuess, rTarget)
	ratio := 1.0 - (float64(dist) / float64(maxLen))
	if ratio < 0 {
		ratio = 0
	}
	pct := int(math.Round(ratio * 100))
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

type MatchResult string

const (
	MatchResultNone MatchResult = "none"
	MatchResultBand MatchResult = "band"
	MatchResultSong MatchResult = "song"
)

type GuessComparison struct {
	NormalizedGuess string
	BestPercent     int
	Result          MatchResult
	MatchedTarget   string
}

// CompareGuess evaluates the player's raw guess against the target song and main artist.
// Conforms to M2-01, M2-04, M2-05, M2-06:
// References:
// 1. Normalized title
// 2. Full title (only steps 1 and 5, for titles with essential parentheses like "(I Can't Get No) Satisfaction")
// 3. Title + Band / Band + Title
// 4. Band
// Match threshold: >= 95%
func CompareGuess(rawGuess, rawTitle, rawBand string) GuessComparison {
	normGuess := NormalizePlayerGuess(rawGuess)
	if normGuess == "" {
		return GuessComparison{
			NormalizedGuess: "",
			BestPercent:     0,
			Result:          MatchResultNone,
		}
	}

	refTitle := NormalizeReference(rawTitle)
	refFullTitle := NormalizePlayerGuess(rawTitle)
	refBand := NormalizeBand(rawBand)

	refTitleBand := NormalizePlayerGuess(refTitle + " " + refBand)
	refBandTitle := NormalizePlayerGuess(refBand + " " + refTitle)

	// Check comparisons
	type candidate struct {
		target string
		kind   MatchResult
	}

	candidates := []candidate{
		{target: refTitle, kind: MatchResultSong},
		{target: refFullTitle, kind: MatchResultSong},
		{target: refTitleBand, kind: MatchResultSong},
		{target: refBandTitle, kind: MatchResultSong},
		{target: refBand, kind: MatchResultBand},
	}

	bestPct := 0
	bestResult := MatchResultNone
	bestMatched := ""

	// Prefer Song match over Band match if both meet threshold or if higher %
	for _, c := range candidates {
		if c.target == "" {
			continue
		}
		pct := CalculateSimilarity(normGuess, c.target)
		if pct > bestPct {
			bestPct = pct
			bestMatched = c.target
			if pct >= 95 {
				bestResult = c.kind
			} else {
				bestResult = MatchResultNone
			}
		} else if pct == bestPct && pct >= 95 && bestResult != MatchResultSong && c.kind == MatchResultSong {
			// If tied at >= 95, song takes precedence over band
			bestResult = MatchResultSong
			bestMatched = c.target
		}
	}

	return GuessComparison{
		NormalizedGuess: normGuess,
		BestPercent:     bestPct,
		Result:          bestResult,
		MatchedTarget:   bestMatched,
	}
}
