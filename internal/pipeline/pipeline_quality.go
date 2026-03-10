package pipeline

import (
	"strings"

	"vargasparse/internal/quality"
)

func parseLangs(hint string) []string {
	if hint == "" || hint == "auto" {
		return []string{"por", "eng"}
	}
	parts := strings.Split(hint, ",")
	langs := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			langs = append(langs, p)
		}
	}
	if len(langs) == 0 {
		return []string{"por", "eng"}
	}
	return langs
}

func detectLanguage(text, langHint string) string {
	if langHint != "" && langHint != "auto" {
		langs := parseLangs(langHint)
		if len(langs) > 0 {
			return langs[0]
		}
	}
	lower := strings.ToLower(text)
	if strings.ContainsAny(lower, "ãõçáàâéêíóôú") {
		return "por"
	}
	if strings.TrimSpace(lower) == "" {
		return "unknown"
	}
	return "eng"
}

func shouldUseVLM(cfg *Config) bool {
	if cfg.EngineMode == EngineLegacy || cfg.EngineMode == EngineHybrid {
		return true
	}
	return cfg.EnableVLMRescue
}

func qualitySignals(q quality.QualityResult) map[string]float64 {
	return map[string]float64{
		"printable_ratio":    q.PrintableRatio,
		"lexicon_score":      q.LexiconScore,
		"garbage_score":      q.GarbageScore,
		"token_entropy":      q.TokenEntropy,
		"line_consistency":   q.LineConsistency,
		"symbol_ratio":       q.SymbolRatio,
		"repetition_penalty": q.RepetitionPenalty,
	}
}

func profileDecision(profile string, q quality.QualityResult) quality.Decision {
	decision := q.Decision
	if profile == "accuracy" && decision == quality.Accept && q.Confidence < 0.95 {
		decision = quality.Compare
	}
	return decision
}

func isDependencyErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "executable file")
}
