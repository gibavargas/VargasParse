// Package pipeline provides the core PDF extraction pipeline:
// types, page processing, OCR fallback, worker pool, and result assembly.
package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/pbnjay/memory"

	"vargasparse/internal/llamacpp"
	"vargasparse/internal/ocr"
	"vargasparse/internal/progress"
	"vargasparse/internal/quality"
	"vargasparse/internal/renderer"
	"vargasparse/internal/table"
)

const (
	EngineDeterministic = "deterministic"
	EngineHybrid        = "hybrid"
	EngineLegacy        = "legacy"
)

const (
	ErrorCodeNone              = ""
	ErrorCodeTimeout           = "timeout"
	ErrorCodeDependencyMissing = "dependency_missing"
	ErrorCodeNativeFailed      = "native_failed"
	ErrorCodeOCRFailed         = "ocr_failed"
	ErrorCodeVLMFailed         = "vlm_failed"
	ErrorCodeNoText            = "no_text"
)

// Block represents a structural text block with bounding box info.
type Block struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"` // paragraph, heading, list, key_value, caption
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

// NativeExtractor extracts text directly from PDF text layer.
type NativeExtractor interface {
	Extract(ctx context.Context, pdfPath string, pageIndex int) (text string, source string, blocks []Block, tables []Table, err error)
}

// OCRExtractor extracts text from rasterized pages.
type OCRExtractor interface {
	Extract(ctx context.Context, pdfPath string, pageIndex int, langHint string) (text string, source string, err error)
}

// VLMRescueExtractor extracts text using a vision model for hard pages.
type VLMRescueExtractor interface {
	Extract(ctx context.Context, pdfPath string, pageIndex int) (text string, source string, err error)
	Close() error
}

// QualityDecider computes confidence/decision from extracted text.
type QualityDecider interface {
	Assess(text string, dict map[string]bool) quality.QualityResult
}

// Config holds the pipeline configuration.
type Config struct {
	PDFPath         string
	Dict            map[string]bool
	NumWorkers      int
	Profile         string // "accuracy" | "balanced"
	LangHint        string // "auto" or comma-separated languages
	PageTimeoutSec  int    // per-page timeout; 0 = no timeout
	ModelName       string // local model for VLM rescue
	EngineMode      string // deterministic | hybrid | legacy
	EnableVLMRescue bool

	NativeExtractor    NativeExtractor
	OCRExtractor       OCRExtractor
	VLMRescueExtractor VLMRescueExtractor
	QualityDecider     QualityDecider
}

// PageResult holds the extraction result for a single PDF page.
type PageResult struct {
	PageNum          int                `json:"page_num"`
	Text             string             `json:"text"`
	Method           string             `json:"method"`
	Confidence       float64            `json:"confidence"`
	Duration         time.Duration      `json:"duration_ms"`
	Warnings         []string           `json:"warnings,omitempty"`
	Blocks           []Block            `json:"blocks,omitempty"`
	Tables           []Table            `json:"tables,omitempty"`
	EngineTrace      []string           `json:"engine_trace,omitempty"`
	QualitySignals   map[string]float64 `json:"quality_signals,omitempty"`
	LanguageDetected string             `json:"language_detected,omitempty"`
	OCRApplied       bool               `json:"ocr_applied,omitempty"`
	ErrorCode        string             `json:"error_code,omitempty"`
}

type defaultQualityDecider struct{}

func (d defaultQualityDecider) Assess(text string, dict map[string]bool) quality.QualityResult {
	return quality.AssessQuality(text, dict)
}

type defaultNativeExtractor struct{}

func (e defaultNativeExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, []Block, []Table, error) {
	text, err := extractPdftotext(ctx, pdfPath, pageIndex)
	if err == nil && strings.TrimSpace(text) != "" {
		tables, _ := extractTablesFast(ctx, pdfPath, pageIndex)
		return text, "pdftotext", buildBlock(pageIndex+1, text, "pdftotext"), tables, nil
	}

	fallbackText, fallbackErr := extractTextFast(ctx, pdfPath, pageIndex)
	if fallbackErr == nil && strings.TrimSpace(fallbackText) != "" {
		tables, _ := extractTablesFast(ctx, pdfPath, pageIndex)
		return fallbackText, "golib_fallback", buildBlock(pageIndex+1, fallbackText, "golib_fallback"), tables, nil
	}

	if err != nil && fallbackErr != nil {
		return "", "", nil, nil, fmt.Errorf("pdftotext failed: %v; golib failed: %v", err, fallbackErr)
	}
	if err != nil {
		return "", "", nil, nil, err
	}
	if fallbackErr != nil {
		return "", "", nil, nil, fallbackErr
	}
	return "", "", nil, nil, nil
}

type tesseractOCRExtractor struct {
	engine *ocr.OCR
}

func (e *tesseractOCRExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, string, error) {
	text, _, err := e.engine.ProcessPage(ctx, pdfPath, pageIndex, langHint)
	if err != nil {
		return "", "", err
	}
	return text, "tesseract", nil
}

type ollamaVLMExtractor struct {
	engine *llamacpp.Engine
}

func (e *ollamaVLMExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, error) {
	if e.engine == nil {
		return "", "", fmt.Errorf("vlm engine not initialized")
	}

	raster, err := renderer.NewPDFRasterizer(pdfPath)
	if err != nil {
		return "", "", fmt.Errorf("create rasterizer: %w", err)
	}
	defer raster.Close()

	if pageIndex < 0 || pageIndex >= raster.NumPages() {
		return "", "", fmt.Errorf("page out of range")
	}

	img, err := raster.RenderPage(pageIndex, 150)
	if err != nil {
		return "", "", fmt.Errorf("render page: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return "", "", fmt.Errorf("encode png: %w", err)
	}

	imgBase64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	text, err := e.engine.ExtractMarkdownWithRetry(ctx, imgBase64, llamacpp.SystemPromptMarkdown, 2)
	if err != nil {
		return "", "", err
	}
	return text, "ollama_vlm", nil
}

func (e *ollamaVLMExtractor) Close() error {
	if e.engine == nil {
		return nil
	}
	return e.engine.Close()
}

// PageCount returns the number of pages in a PDF.
func PageCount(pdfPath string) (int, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return 0, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()
	return r.NumPage(), nil
}

func extractPdftotext(ctx context.Context, pdfPath string, pageIndex int) (string, error) {
	pageNum := pageIndex + 1
	cmd := exec.CommandContext(
		ctx,
		"pdftotext",
		"-f", fmt.Sprintf("%d", pageNum),
		"-l", fmt.Sprintf("%d", pageNum),
		"-layout",
		pdfPath,
		"-",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(out), nil
}

// extractTextFast extracts the text layer of a single PDF page.
// pageIndex is 0-based. Respects context cancellation.
func extractTextFast(ctx context.Context, pdfPath string, pageIndex int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	pageNum := pageIndex + 1
	if pageNum > r.NumPage() {
		return "", fmt.Errorf("page %d out of range (%d total)", pageNum, r.NumPage())
	}

	page := r.Page(pageNum)
	if page.V.IsNull() {
		return "", nil
	}

	var sb strings.Builder
	for _, t := range page.Content().Text {
		sb.WriteString(t.S)
	}
	return sb.String(), nil
}

func extractTablesFast(ctx context.Context, pdfPath string, pageIndex int) ([]Table, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pageNum := pageIndex + 1
	if pageNum < 1 || pageNum > r.NumPage() {
		return nil, fmt.Errorf("page out of range")
	}
	p := r.Page(pageNum)
	tables := table.ExtractTable(p)
	return convertExtractedTables(tables), nil
}

func convertExtractedTables(in []table.Table) []Table {
	if len(in) == 0 {
		return nil
	}
	out := make([]Table, 0, len(in))
	for i, t := range in {
		tt := Table{
			ID:   fmt.Sprintf("p_table_%d", i),
			Rows: t.Rows,
			Cols: t.Cols,
			BBox: &BBox{
				X: t.Box.X_min,
				Y: t.Box.Y_min,
				W: t.Box.X_max - t.Box.X_min,
				H: t.Box.Y_max - t.Box.Y_min,
			},
			Cells: make([]Cell, 0, len(t.Cells)),
		}
		for _, c := range t.Cells {
			tt.Cells = append(tt.Cells, Cell{
				Row:     c.Row,
				Col:     c.Col,
				RowSpan: c.RowSpan,
				ColSpan: c.ColSpan,
				Text:    c.Text,
			})
		}
		out = append(out, tt)
	}
	return out
}

func buildBlock(pageNum int, text, source string) []Block {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []Block{{
		ID:           fmt.Sprintf("p%d_%s", pageNum, source),
		Type:         "paragraph",
		Text:         text,
		ReadingOrder: 0,
		Confidence:   1.0,
		SourceMethod: source,
	}}
}

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

func processPage(ctx context.Context, cfg *Config, pageIndex int) PageResult {
	start := time.Now()
	pageNum := pageIndex + 1
	result := PageResult{PageNum: pageNum, ErrorCode: ErrorCodeNone}

	decider := cfg.QualityDecider
	if decider == nil {
		decider = defaultQualityDecider{}
	}

	// Legacy mode allows VLM as primary attempt, but deterministic path still handles failures.
	if cfg.EngineMode == EngineLegacy && cfg.VLMRescueExtractor != nil {
		result.EngineTrace = append(result.EngineTrace, "vlm_primary:start")
		vlmText, vlmSource, vlmErr := cfg.VLMRescueExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
		if vlmErr == nil && strings.TrimSpace(vlmText) != "" {
			q := decider.Assess(vlmText, cfg.Dict)
			if q.Confidence >= 0.70 {
				result.Text = vlmText
				result.Method = progress.MethodFast
				result.Confidence = q.Confidence
				result.Blocks = buildBlock(pageNum, vlmText, vlmSource)
				result.QualitySignals = qualitySignals(q)
				result.LanguageDetected = detectLanguage(vlmText, cfg.LangHint)
				result.EngineTrace = append(result.EngineTrace, "vlm_primary:ok")
				result.Duration = time.Since(start)
				return result
			}
		}
		if vlmErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("vlm primary failed: %v", vlmErr))
		}
		result.EngineTrace = append(result.EngineTrace, "vlm_primary:fallback")
	}

	nativeText, nativeSource, nativeBlocks, nativeTables, nativeErr := cfg.NativeExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
	if nativeErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("native extraction error: %v", nativeErr))
		result.EngineTrace = append(result.EngineTrace, "native:error")
		if isDependencyErr(nativeErr) {
			result.ErrorCode = ErrorCodeDependencyMissing
		} else {
			result.ErrorCode = ErrorCodeNativeFailed
		}
	} else {
		result.EngineTrace = append(result.EngineTrace, "native:"+nativeSource)
	}

	nativeQ := decider.Assess(nativeText, cfg.Dict)
	result.Confidence = nativeQ.Confidence
	result.QualitySignals = qualitySignals(nativeQ)
	result.LanguageDetected = detectLanguage(nativeText, cfg.LangHint)

	decision := profileDecision(cfg.Profile, nativeQ)
	if decision == quality.Accept && strings.TrimSpace(nativeText) != "" {
		result.Text = nativeText
		result.Method = progress.MethodFast
		result.Blocks = nativeBlocks
		result.Tables = nativeTables
		result.Duration = time.Since(start)
		return result
	}

	oText, oConf, oBlocks, oSource, oErrCode, oWarnings := runOCR(ctx, cfg, pageIndex)
	result.Warnings = append(result.Warnings, oWarnings...)
	if strings.TrimSpace(oText) != "" {
		result.OCRApplied = true
		result.EngineTrace = append(result.EngineTrace, "ocr:"+oSource)
	}

	switch decision {
	case quality.Compare:
		if strings.TrimSpace(oText) != "" && oConf > nativeQ.Confidence {
			result.Text = oText
			result.Confidence = oConf
			result.Blocks = oBlocks
		} else {
			result.Text = nativeText
			result.Blocks = nativeBlocks
			result.Tables = nativeTables
		}
		result.Method = progress.MethodCompare
	case quality.Reject:
		if strings.TrimSpace(oText) != "" {
			result.Text = oText
			result.Confidence = oConf
			result.Blocks = oBlocks
			result.Method = progress.MethodOCR
		} else {
			result.Method = progress.MethodOCRFail
			if oErrCode != "" {
				result.ErrorCode = oErrCode
			} else {
				result.ErrorCode = ErrorCodeNoText
			}
		}
	default:
		result.Text = nativeText
		result.Blocks = nativeBlocks
		result.Tables = nativeTables
		result.Method = progress.MethodFast
	}

	needsRescue := shouldUseVLM(cfg) && cfg.VLMRescueExtractor != nil &&
		(strings.TrimSpace(result.Text) == "" || result.Confidence < 0.55)
	if needsRescue {
		result.EngineTrace = append(result.EngineTrace, "vlm_rescue:start")
		vlmText, vlmConf, vlmBlocks, vlmErr := runVLMRescue(ctx, cfg, pageIndex)
		if vlmErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("vlm rescue failed: %v", vlmErr))
			if result.ErrorCode == "" {
				result.ErrorCode = ErrorCodeVLMFailed
			}
			result.EngineTrace = append(result.EngineTrace, "vlm_rescue:error")
		} else if strings.TrimSpace(vlmText) != "" {
			result.Text = vlmText
			if vlmConf > result.Confidence {
				result.Confidence = vlmConf
			}
			result.Blocks = vlmBlocks
			result.Method = progress.MethodCompare
			result.ErrorCode = ErrorCodeNone
			result.EngineTrace = append(result.EngineTrace, "vlm_rescue:ok")
		}
	}

	if strings.TrimSpace(result.Text) == "" {
		result.Method = progress.MethodOCRFail
		if result.ErrorCode == "" {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.ErrorCode = ErrorCodeTimeout
			} else {
				result.ErrorCode = ErrorCodeNoText
			}
		}
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.ErrorCode = ErrorCodeTimeout
		result.Warnings = append(result.Warnings, "page timed out")
	}

	result.Duration = time.Since(start)
	return result
}

func runOCR(ctx context.Context, cfg *Config, pageIndex int) (string, float64, []Block, string, string, []string) {
	pageNum := pageIndex + 1
	var warnings []string

	if cfg.OCRExtractor == nil {
		warnings = append(warnings, "OCR extractor unavailable")
		return "", 0, nil, "", ErrorCodeDependencyMissing, warnings
	}

	text, source, err := cfg.OCRExtractor.Extract(ctx, cfg.PDFPath, pageIndex, cfg.LangHint)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("OCR failed: %v", err))
		if isDependencyErr(err) {
			return "", 0, nil, source, ErrorCodeDependencyMissing, warnings
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", 0, nil, source, ErrorCodeTimeout, warnings
		}
		return "", 0, nil, source, ErrorCodeOCRFailed, warnings
	}

	if strings.TrimSpace(text) == "" {
		warnings = append(warnings, "OCR returned empty text")
		return "", 0, nil, source, ErrorCodeNoText, warnings
	}

	q := cfg.QualityDecider.Assess(text, cfg.Dict)
	blocks := []Block{{
		ID:           fmt.Sprintf("p%d_ocr", pageNum),
		Type:         "paragraph",
		Text:         text,
		ReadingOrder: 0,
		Confidence:   q.Confidence,
		SourceMethod: source,
	}}
	return text, q.Confidence, blocks, source, ErrorCodeNone, warnings
}

func runVLMRescue(ctx context.Context, cfg *Config, pageIndex int) (string, float64, []Block, error) {
	if cfg.VLMRescueExtractor == nil {
		return "", 0, nil, fmt.Errorf("vlm rescue extractor unavailable")
	}
	text, source, err := cfg.VLMRescueExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
	if err != nil {
		return "", 0, nil, err
	}
	if strings.TrimSpace(text) == "" {
		return "", 0, nil, nil
	}
	q := cfg.QualityDecider.Assess(text, cfg.Dict)
	blocks := []Block{{
		ID:           fmt.Sprintf("p%d_vlm", pageIndex+1),
		Type:         "paragraph",
		Text:         text,
		ReadingOrder: 0,
		Confidence:   q.Confidence,
		SourceMethod: source,
	}}
	return text, q.Confidence, blocks, nil
}

func isDependencyErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "executable file")
}

// ComputeWorkers returns the optimal number of goroutines.
func ComputeWorkers(override int) int {
	if override > 0 {
		return override
	}

	cpuCount := runtime.NumCPU()
	totalRAMGB := float64(memory.TotalMemory()) / (1024 * 1024 * 1024)

	ramBudget := int(math.Floor(totalRAMGB * 10))
	workers := cpuCount
	if ramBudget < workers {
		workers = ramBudget
	}
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}
	return workers
}

func initializeDependencies(cfg *Config) func() {
	if cfg.EngineMode == "" {
		cfg.EngineMode = EngineDeterministic
	}
	if cfg.QualityDecider == nil {
		cfg.QualityDecider = defaultQualityDecider{}
	}
	if cfg.NativeExtractor == nil {
		cfg.NativeExtractor = defaultNativeExtractor{}
	}
	if cfg.OCRExtractor == nil {
		engine, err := ocr.NewOCR()
		if err == nil {
			cfg.OCRExtractor = &tesseractOCRExtractor{engine: engine}
		}
	}

	cleanup := func() {}
	if shouldUseVLM(cfg) && cfg.VLMRescueExtractor == nil {
		model := cfg.ModelName
		if model == "" {
			model = "llava"
		}
		eng, err := llamacpp.NewEngine(model)
		if err == nil {
			ex := &ollamaVLMExtractor{engine: eng}
			cfg.VLMRescueExtractor = ex
			cleanup = func() { _ = ex.Close() }
		}
	}
	return cleanup
}

// Run launches the worker pool and returns sorted results.
func Run(cfg *Config, numPages int, progressCh chan<- progress.Event) []PageResult {
	cleanup := initializeDependencies(cfg)
	defer cleanup()

	jobs := make(chan int, numPages)
	results := make(chan PageResult, numPages)

	ctx := context.Background()

	var wg sync.WaitGroup
	for w := 0; w < cfg.NumWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageIndex := range jobs {
				var pageCtx context.Context
				var cancel context.CancelFunc
				if cfg.PageTimeoutSec > 0 {
					pageCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.PageTimeoutSec)*time.Second)
				} else {
					pageCtx, cancel = context.WithCancel(ctx)
				}

				r := processPage(pageCtx, cfg, pageIndex)
				if errors.Is(pageCtx.Err(), context.DeadlineExceeded) {
					r.ErrorCode = ErrorCodeTimeout
					if strings.TrimSpace(r.Text) == "" {
						r.Method = progress.MethodOCRFail
					}
				}

				results <- r
				progressCh <- progress.Event{
					PageNum:  r.PageNum,
					Method:   r.Method,
					Score:    r.Confidence,
					Duration: r.Duration,
					Warning:  strings.Join(r.Warnings, "; "),
				}
				cancel()
			}
		}()
	}

	for i := 0; i < numPages; i++ {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]PageResult, 0, numPages)
	for r := range results {
		all = append(all, r)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].PageNum < all[j].PageNum
	})
	return all
}
