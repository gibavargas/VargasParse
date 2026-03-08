// Package pipeline provides the core PDF extraction pipeline:
// types, page processing, OCR fallback, worker pool, and result assembly.
package pipeline

import (
	"context"
	"time"

	"vargasparse/internal/quality"
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
	RenderDPI       float64 // DPI for rasterizing PDF pages (0 = default 150)

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

// NewDefaultQualityDecider returns the default quality scorer implementation.
func NewDefaultQualityDecider() QualityDecider {
	return defaultQualityDecider{}
}
