package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os/exec"
	"strings"

	"github.com/ledongthuc/pdf"

	"vargasparse/internal/llamacpp"
	"vargasparse/internal/ocr"
	"vargasparse/internal/renderer"
	"vargasparse/internal/table"
)

var (
	commandContext = exec.CommandContext
	newRasterizer  = func(pdfPath string) (pageRasterizer, error) { return renderer.NewPDFRasterizer(pdfPath) }
	pngEncode      = png.Encode
)

type ocrProcessor interface {
	ProcessPage(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, float64, error)
}

type vlmEngine interface {
	ExtractMarkdownWithRetry(ctx context.Context, imgBase64, systemPrompt string, attempts int) (string, error)
	Close() error
}

type pageRasterizer interface {
	NumPages() int
	RenderPage(pageIndex int, dpi float64) (image.Image, error)
	Close() error
}

type defaultNativeExtractor struct{}

// NewDefaultNativeExtractor returns the native text-layer extractor.
func NewDefaultNativeExtractor() NativeExtractor {
	return defaultNativeExtractor{}
}

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
	engine ocrProcessor
}

// NewTesseractOCRExtractor returns a pipeline OCR adapter backed by a local OCR engine.
func NewTesseractOCRExtractor(engine *ocr.OCR) OCRExtractor {
	return &tesseractOCRExtractor{engine: engine}
}

func (e *tesseractOCRExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, string, error) {
	text, _, err := e.engine.ProcessPage(ctx, pdfPath, pageIndex, langHint)
	if err != nil {
		return "", "", err
	}
	return text, "tesseract", nil
}

type ollamaVLMExtractor struct {
	engine vlmEngine
	dpi    float64
}

// NewOllamaVLMRescueExtractor returns a rescue extractor backed by a local VLM engine.
func NewOllamaVLMRescueExtractor(engine *llamacpp.Engine, dpi float64) VLMRescueExtractor {
	if dpi <= 0 {
		dpi = renderer.DefaultDPI
	}
	return &ollamaVLMExtractor{engine: engine, dpi: dpi}
}

func (e *ollamaVLMExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, error) {
	if e.engine == nil {
		return "", "", fmt.Errorf("vlm engine not initialized")
	}

	raster, err := newRasterizer(pdfPath)
	if err != nil {
		return "", "", fmt.Errorf("create rasterizer: %w", err)
	}
	defer raster.Close()

	if pageIndex < 0 || pageIndex >= raster.NumPages() {
		return "", "", fmt.Errorf("page out of range")
	}

	img, err := raster.RenderPage(pageIndex, e.dpi)
	if err != nil {
		return "", "", fmt.Errorf("render page: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := pngEncode(buf, img); err != nil {
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

func extractPdftotext(ctx context.Context, pdfPath string, pageIndex int) (string, error) {
	pageNum := pageIndex + 1
	cmd := commandContext(
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
