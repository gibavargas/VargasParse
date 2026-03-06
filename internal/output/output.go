// Package output formats extraction results as .txt, .md, or .json documents.
package output

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Block represents a structural text block with bounding box info.
type Block struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Text         string  `json:"text"`
	BBox         *BBox   `json:"bbox,omitempty"`
	ReadingOrder int     `json:"reading_order"`
	Confidence   float64 `json:"confidence"`
	SourceMethod string  `json:"source_method"`
}

// BBox is a bounding box in PDF points.
type BBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Table represents an extracted table.
type Table struct {
	ID    string `json:"id"`
	BBox  *BBox  `json:"bbox,omitempty"`
	Rows  int    `json:"rows"`
	Cols  int    `json:"cols"`
	Cells []Cell `json:"cells"`
}

// Cell is a single table cell.
type Cell struct {
	Row     int    `json:"row"`
	Col     int    `json:"col"`
	RowSpan int    `json:"row_span"`
	ColSpan int    `json:"col_span"`
	Text    string `json:"text"`
}

// PageData is the page info carried through to formatters.
type PageData struct {
	PageNum          int                `json:"page_num"`
	Text             string             `json:"text"`
	Method           string             `json:"method"`
	Confidence       float64            `json:"confidence"`
	DurationMs       int64              `json:"duration_ms"`
	Warnings         []string           `json:"warnings,omitempty"`
	Blocks           []Block            `json:"blocks,omitempty"`
	Tables           []Table            `json:"tables,omitempty"`
	EngineTrace      []string           `json:"engine_trace,omitempty"`
	QualitySignals   map[string]float64 `json:"quality_signals,omitempty"`
	LanguageDetected string             `json:"language_detected,omitempty"`
	OCRApplied       bool               `json:"ocr_applied,omitempty"`
	ErrorCode        string             `json:"error_code,omitempty"`
}

// Document is the top-level JSON output structure.
type Document struct {
	SourceFile    string     `json:"source_file"`
	TotalPages    int        `json:"total_pages"`
	PagesWithText int        `json:"pages_with_text"`
	ProcessingMs  int64      `json:"processing_ms"`
	Profile       string     `json:"profile"`
	Pages         []PageData `json:"pages"`
	Metrics       DocMetrics `json:"metrics"`
}

// DocMetrics contains aggregate stats for the document.
type DocMetrics struct {
	FastPages    int `json:"fast_pages"`
	OCRPages     int `json:"ocr_pages"`
	ComparePages int `json:"compare_pages"`
	FailedPages  int `json:"failed_pages"`
	SkippedPages int `json:"skipped_pages"`
}

// FormatTxt assembles pages into a plain-text document.
func FormatTxt(pages []PageData) string {
	var sb strings.Builder
	first := true
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		if !first {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString(text)
		first = false
	}
	return sb.String()
}

// FormatMd assembles pages into a Markdown document with headings.
func FormatMd(pages []PageData) string {
	var sb strings.Builder
	for _, p := range pages {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&sb, "## Página %d\n\n%s\n\n---\n\n", p.PageNum, text)
	}
	return sb.String()
}

// FormatJSON assembles the full document as indented JSON.
func FormatJSON(doc Document) (string, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}
	return string(data), nil
}

// Format dispatches to the correct formatter based on extension.
func Format(pages []PageData, ext string) string {
	switch ext {
	case ".md":
		return FormatMd(pages)
	case ".json":
		return FormatTxt(pages)
	default:
		return FormatTxt(pages)
	}
}

// Report is a structured per-page extraction report.
type Report struct {
	SourceFile     string         `json:"source_file"`
	Timestamp      string         `json:"timestamp"`
	TotalPages     int            `json:"total_pages"`
	PassRate       float64        `json:"pass_rate_pct"`
	FailRate       float64        `json:"fail_rate_pct"`
	CERMedian      float64        `json:"cer_median,omitempty"`
	CERP95         float64        `json:"cer_p95,omitempty"`
	WERMedian      float64        `json:"wer_median,omitempty"`
	WERP95         float64        `json:"wer_p95,omitempty"`
	FailureClasses map[string]int `json:"failure_classes,omitempty"`
	Pages          []PageReport   `json:"pages"`
}

// PageReport describes extraction outcome for one page.
type PageReport struct {
	PageNum    int      `json:"page_num"`
	Pass       bool     `json:"pass"`
	Method     string   `json:"method"`
	Confidence float64  `json:"confidence"`
	DurationMs int64    `json:"duration_ms"`
	Warnings   []string `json:"warnings,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	ErrorCode  string   `json:"error_code,omitempty"`
}

// BenchmarkReport captures benchmark metrics and gate result.
type BenchmarkReport struct {
	SourceFile    string  `json:"source_file"`
	TruthFile     string  `json:"truth_file,omitempty"`
	TruthPresent  bool    `json:"truth_present"`
	Timestamp     string  `json:"timestamp"`
	CER           float64 `json:"cer"`
	WER           float64 `json:"wer"`
	PassRate      float64 `json:"pass_rate_pct"`
	FailRate      float64 `json:"fail_rate_pct"`
	MinPassRate   float64 `json:"min_pass_rate_pct"`
	MaxFailRate   float64 `json:"max_fail_rate_pct"`
	Passed        bool    `json:"passed"`
	FailureReason string  `json:"failure_reason,omitempty"`
}

// BuildReport builds a structured report from page results.
func BuildReport(sourceFile string, pages []PageData, wallDuration time.Duration) Report {
	_ = wallDuration
	r := Report{
		SourceFile:     sourceFile,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		TotalPages:     len(pages),
		FailureClasses: map[string]int{},
	}

	passed := 0
	for _, p := range pages {
		pr := PageReport{
			PageNum:    p.PageNum,
			Method:     p.Method,
			Confidence: p.Confidence,
			DurationMs: p.DurationMs,
			Warnings:   p.Warnings,
			ErrorCode:  p.ErrorCode,
		}

		if strings.TrimSpace(p.Text) != "" && p.Confidence >= 0.55 {
			pr.Pass = true
			passed++
		} else {
			pr.Pass = false
			if strings.TrimSpace(p.Text) == "" {
				pr.Reason = "no text extracted"
			} else {
				pr.Reason = fmt.Sprintf("low confidence: %.2f", p.Confidence)
			}
			if p.ErrorCode != "" {
				r.FailureClasses[p.ErrorCode]++
			} else {
				r.FailureClasses["unknown"]++
			}
		}
		r.Pages = append(r.Pages, pr)
	}

	if len(pages) > 0 {
		r.PassRate = float64(passed) / float64(len(pages)) * 100
		r.FailRate = 100 - r.PassRate
	}
	return r
}

// BuildBenchmarkReport computes CER/WER and threshold gates.
func BuildBenchmarkReport(sourceFile, truthFile, extractedText, truthText string, report Report, minPassRate, maxFailRate float64) BenchmarkReport {
	truthPresent := strings.TrimSpace(truthText) != ""
	cer, wer := 0.0, 0.0
	if truthPresent {
		cer, wer = CERAndWER(extractedText, truthText)
	}
	b := BenchmarkReport{
		SourceFile:   sourceFile,
		TruthFile:    truthFile,
		TruthPresent: truthPresent,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		CER:          cer,
		WER:          wer,
		PassRate:     report.PassRate,
		FailRate:     report.FailRate,
		MinPassRate:  minPassRate,
		MaxFailRate:  maxFailRate,
	}

	if !truthPresent {
		b.FailureReason = "missing or empty ground-truth text"
	}
	if report.PassRate < minPassRate {
		if b.FailureReason != "" {
			b.FailureReason += "; "
		}
		b.FailureReason += fmt.Sprintf("pass rate %.2f < minimum %.2f", report.PassRate, minPassRate)
	}
	if report.FailRate > maxFailRate {
		if b.FailureReason != "" {
			b.FailureReason += "; "
		}
		b.FailureReason += fmt.Sprintf("fail rate %.2f > maximum %.2f", report.FailRate, maxFailRate)
	}
	b.Passed = b.FailureReason == ""
	return b
}

// CERAndWER computes character and word error rates (0-1).
func CERAndWER(extracted, truth string) (float64, float64) {
	truthChars := []rune(normalizeForMetric(truth))
	extChars := []rune(normalizeForMetric(extracted))

	cer := 0.0
	if len(truthChars) > 0 {
		cer = float64(levenshteinRunes(truthChars, extChars)) / float64(len(truthChars))
	}

	truthWords := strings.Fields(normalizeForMetric(truth))
	extWords := strings.Fields(normalizeForMetric(extracted))
	wer := 0.0
	if len(truthWords) > 0 {
		wer = float64(levenshteinStrings(truthWords, extWords)) / float64(len(truthWords))
	}

	return cer, wer
}

func normalizeForMetric(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func levenshteinRunes(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	dp := make([]int, len(b)+1)
	for j := range dp {
		dp[j] = j
	}

	for i := 1; i <= len(a); i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= len(b); j++ {
			tmp := dp[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[j] = min3(dp[j]+1, dp[j-1]+1, prev+cost)
			prev = tmp
		}
	}
	return dp[len(b)]
}

func levenshteinStrings(a, b []string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	dp := make([]int, len(b)+1)
	for j := range dp {
		dp[j] = j
	}

	for i := 1; i <= len(a); i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= len(b); j++ {
			tmp := dp[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[j] = min3(dp[j]+1, dp[j-1]+1, prev+cost)
			prev = tmp
		}
	}
	return dp[len(b)]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

// AttachErrorMetrics sets CER/WER summary fields from page-level vectors.
func AttachErrorMetrics(r *Report, pageCER, pageWER []float64) {
	if len(pageCER) == 0 || len(pageWER) == 0 {
		return
	}
	r.CERMedian = percentile(pageCER, 50)
	r.CERP95 = percentile(pageCER, 95)
	r.WERMedian = percentile(pageWER, 50)
	r.WERP95 = percentile(pageWER, 95)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := (p / 100.0) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	weight := rank - float64(lo)
	return sorted[lo]*(1-weight) + sorted[hi]*weight
}

// FormatReport serializes a report to indented JSON.
func FormatReport(r Report) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("report json: %w", err)
	}
	return string(data), nil
}

// FormatBenchmarkReport serializes benchmark results to indented JSON.
func FormatBenchmarkReport(r BenchmarkReport) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("benchmark report json: %w", err)
	}
	return string(data), nil
}
