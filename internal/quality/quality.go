// Package quality provides text quality assessment:
// dictionary-based token scoring, printable-ratio analysis,
// garbage detection, and ensemble confidence scoring.
package quality

import (
	"bufio"
	_ "embed"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed dicts/en_us.txt
var enDictRaw string

//go:embed dicts/pt_br.txt
var ptDictRaw string

// EmbeddedDictionary loads the built-in EN+PT word lists.
func EmbeddedDictionary() map[string]bool {
	return LoadDictionary(enDictRaw, ptDictRaw)
}

// LoadDictionary builds a combined map from newline-delimited word lists.
func LoadDictionary(sources ...string) map[string]bool {
	dict := make(map[string]bool, 600_000)
	for _, raw := range sources {
		scanner := bufio.NewScanner(strings.NewReader(raw))
		for scanner.Scan() {
			w := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if w != "" {
				dict[w] = true
			}
		}
	}
	return dict
}

var reNonAlpha = regexp.MustCompile(`[^a-záàâãäéèêëíìîïóòôõöúùûüýÿçñA-Z\p{L}]+`)

// CleanAndTokenize removes numbers/punctuation, lowercases, and splits into tokens.
func CleanAndTokenize(text string) []string {
	lower := strings.ToLower(text)
	cleaned := reNonAlpha.ReplaceAllString(lower, " ")
	parts := strings.Fields(cleaned)

	tokens := parts[:0]
	for _, p := range parts {
		if utf8.RuneCountInString(p) > 1 {
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// QualityResult holds the result of an ensemble quality assessment.
type QualityResult struct {
	PrintableRatio    float64 // fraction of printable runes
	LexiconScore      float64 // fraction of tokens found in dictionary
	GarbageScore      float64 // 0.0 = clean, 1.0 = full garbage
	TokenEntropy      float64 // normalized token entropy (0-1)
	LineConsistency   float64 // normalized line regularity (0-1)
	SymbolRatio       float64 // ratio of symbols to total runes (0-1)
	RepetitionPenalty float64 // repeated-run penalty (0-1)

	Confidence float64
	Decision   Decision
}

// Decision indicates what the pipeline should do with this text.
type Decision int

const (
	Accept  Decision = iota // confidence >= 0.80 — use native text
	Compare                 // 0.55 <= confidence < 0.80 — run OCR and compare
	Reject                  // confidence < 0.55 — force OCR
)

func (d Decision) String() string {
	switch d {
	case Accept:
		return "accept"
	case Compare:
		return "compare"
	case Reject:
		return "reject"
	default:
		return "unknown"
	}
}

var reGarbage = regexp.MustCompile(`(?i)(cid\(|\(cid|[\x00-\x08\x0e-\x1f\x7f-\x9f]|!{2,}|#{2,}|\?{4,})`)

// AssessQuality runs the ensemble scorer on the given text.
// dict may be nil (lexicon score will be 0).
func AssessQuality(text string, dict map[string]bool) QualityResult {
	var r QualityResult

	if strings.TrimSpace(text) == "" {
		r.Decision = Reject
		return r
	}

	r.PrintableRatio = printableRatio(text)
	tokens := CleanAndTokenize(text)

	if len(tokens) > 0 && dict != nil {
		found := 0
		for _, t := range tokens {
			if dict[t] {
				found++
			}
		}
		r.LexiconScore = float64(found) / float64(len(tokens))
	}

	garbageMatches := reGarbage.FindAllString(text, -1)
	totalRunes := utf8.RuneCountInString(text)
	if totalRunes > 0 {
		garbageRunes := 0
		for _, m := range garbageMatches {
			garbageRunes += utf8.RuneCountInString(m)
		}
		r.GarbageScore = math.Min(1.0, float64(garbageRunes)/float64(totalRunes)*10)
	}

	r.TokenEntropy = tokenEntropy(tokens)
	r.LineConsistency = lineConsistency(text)
	r.SymbolRatio = symbolRatio(text)
	r.RepetitionPenalty = repetitionPenalty(text)

	base := r.PrintableRatio*0.18 +
		r.LexiconScore*0.42 +
		(1.0-r.GarbageScore)*0.20 +
		r.TokenEntropy*0.10 +
		r.LineConsistency*0.10

	penalty := r.SymbolRatio*0.15 + r.RepetitionPenalty*0.15
	r.Confidence = clamp01(base - penalty)

	if len(tokens) < 5 && r.GarbageScore < 0.1 {
		r.Confidence = math.Max(r.Confidence, 0.80)
	}

	switch {
	case r.Confidence >= 0.80:
		r.Decision = Accept
	case r.Confidence >= 0.55:
		r.Decision = Compare
	default:
		r.Decision = Reject
	}
	return r
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func printableRatio(text string) float64 {
	total := 0
	printable := 0
	for _, r := range text {
		total++
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			printable++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(printable) / float64(total)
}

func tokenEntropy(tokens []string) float64 {
	if len(tokens) < 2 {
		return 1
	}
	freq := make(map[string]int, len(tokens))
	for _, t := range tokens {
		freq[t]++
	}
	var entropy float64
	total := float64(len(tokens))
	for _, c := range freq {
		p := float64(c) / total
		entropy -= p * math.Log2(p)
	}
	maxEntropy := math.Log2(float64(len(freq)))
	if maxEntropy <= 0 {
		return 1
	}
	return clamp01(entropy / maxEntropy)
}

func lineConsistency(text string) float64 {
	lines := strings.Split(text, "\n")
	counts := make([]float64, 0, len(lines))
	for _, line := range lines {
		toks := strings.Fields(line)
		if len(toks) == 0 {
			continue
		}
		counts = append(counts, float64(len(toks)))
	}
	if len(counts) <= 1 {
		return 1
	}
	mean := 0.0
	for _, c := range counts {
		mean += c
	}
	mean /= float64(len(counts))
	if mean == 0 {
		return 1
	}
	var variance float64
	for _, c := range counts {
		d := c - mean
		variance += d * d
	}
	variance /= float64(len(counts))
	cv := math.Sqrt(variance) / mean
	return clamp01(1.0 / (1.0 + cv))
}

func symbolRatio(text string) float64 {
	total := 0
	symbol := 0
	for _, r := range text {
		total++
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			continue
		}
		symbol++
	}
	if total == 0 {
		return 0
	}
	return float64(symbol) / float64(total)
}

func repetitionPenalty(text string) float64 {
	runes := []rune(text)
	if len(runes) < 4 {
		return 0
	}
	maxRun := 1
	run := 1
	for i := 1; i < len(runes); i++ {
		if runes[i] == runes[i-1] {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 1
		}
	}
	if maxRun <= 3 {
		return 0
	}
	return clamp01(float64(maxRun-3) / 20.0)
}
