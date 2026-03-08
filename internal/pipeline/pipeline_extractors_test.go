package pipeline

import (
	"context"
	"errors"
	"image"
	"image/color"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vargasparse/internal/llamacpp"
	"vargasparse/internal/ocr"
)

type mockOCRProcessor struct {
	text string
	err  error
}

func (m mockOCRProcessor) ProcessPage(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, float64, error) {
	if m.err != nil {
		return "", 0, m.err
	}
	return m.text, 0, nil
}

type mockVLMEngine struct {
	text   string
	err    error
	closed bool
}

func (m *mockVLMEngine) ExtractMarkdownWithRetry(ctx context.Context, imgBase64, systemPrompt string, attempts int) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.text, nil
}

func (m *mockVLMEngine) Close() error {
	m.closed = true
	return nil
}

type mockRasterizer struct {
	pages int
	img   image.Image
	err   error
}

func (m mockRasterizer) NumPages() int { return m.pages }
func (m mockRasterizer) RenderPage(pageIndex int, dpi float64) (image.Image, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.img, nil
}
func (m mockRasterizer) Close() error { return nil }

func TestExtractPdftotextCommandSeam(t *testing.T) {
	orig := commandContext
	defer func() { commandContext = orig }()

	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf 'native text'")
	}

	text, err := extractPdftotext(context.Background(), "ignored.pdf", 0)
	if err != nil {
		t.Fatalf("extractPdftotext error: %v", err)
	}
	if text != "native text" {
		t.Fatalf("text=%q", text)
	}
}

func TestExtractTextFastAndPageCountFixture(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "test_pdfs", "attention.pdf")
	count, err := PageCount(pdfPath)
	if err != nil {
		t.Fatalf("PageCount error: %v", err)
	}
	if count < 1 {
		t.Fatalf("count=%d want >=1", count)
	}

	text, err := extractTextFast(context.Background(), pdfPath, 0)
	if err != nil {
		t.Fatalf("extractTextFast error: %v", err)
	}
	if text == "" {
		t.Fatal("expected extracted text, got empty")
	}
}

func TestExtractTablesFastOutOfRange(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "test_pdfs", "attention.pdf")
	_, err := extractTablesFast(context.Background(), pdfPath, 9999)
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestDefaultNativeExtractorFallsBackToGoLib(t *testing.T) {
	orig := commandContext
	defer func() { commandContext = orig }()

	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}

	pdfPath := filepath.Join("..", "..", "test_pdfs", "attention.pdf")
	text, source, blocks, _, err := NewDefaultNativeExtractor().Extract(context.Background(), pdfPath, 0)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if source != "golib_fallback" {
		t.Fatalf("source=%q", source)
	}
	if strings.TrimSpace(text) == "" || len(blocks) == 0 {
		t.Fatalf("expected fallback text+blocks")
	}
}

func TestTesseractOCRExtractorPassThrough(t *testing.T) {
	ex := &tesseractOCRExtractor{engine: mockOCRProcessor{text: "ocr text"}}
	text, source, err := ex.Extract(context.Background(), "doc.pdf", 0, "eng")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if source != "tesseract" || text != "ocr text" {
		t.Fatalf("unexpected output source=%q text=%q", source, text)
	}
}

func TestTesseractOCRExtractorError(t *testing.T) {
	ex := &tesseractOCRExtractor{engine: mockOCRProcessor{err: errors.New("ocr failed")}}
	_, _, err := ex.Extract(context.Background(), "doc.pdf", 0, "eng")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOllamaVLMExtractorSuccess(t *testing.T) {
	origRasterizer := newRasterizer
	defer func() { newRasterizer = origRasterizer }()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	newRasterizer = func(pdfPath string) (pageRasterizer, error) {
		return mockRasterizer{pages: 1, img: img}, nil
	}

	engine := &mockVLMEngine{text: "vlm text"}
	ex := &ollamaVLMExtractor{engine: engine, dpi: 150}
	text, source, err := ex.Extract(context.Background(), "doc.pdf", 0)
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if text != "vlm text" || source != "ollama_vlm" {
		t.Fatalf("unexpected output source=%q text=%q", source, text)
	}

	if err := ex.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !engine.closed {
		t.Fatal("expected engine close")
	}
}

func TestOllamaVLMExtractorErrors(t *testing.T) {
	ex := &ollamaVLMExtractor{}
	_, _, err := ex.Extract(context.Background(), "doc.pdf", 0)
	if err == nil {
		t.Fatal("expected nil-engine error")
	}

	origRasterizer := newRasterizer
	defer func() { newRasterizer = origRasterizer }()
	newRasterizer = func(pdfPath string) (pageRasterizer, error) {
		return mockRasterizer{pages: 1, img: image.NewRGBA(image.Rect(0, 0, 1, 1))}, nil
	}

	ex = &ollamaVLMExtractor{engine: &mockVLMEngine{text: "x"}}
	_, _, err = ex.Extract(context.Background(), "doc.pdf", 3)
	if err == nil || !strings.Contains(err.Error(), "page out of range") {
		t.Fatalf("unexpected out-of-range error: %v", err)
	}

	newRasterizer = func(pdfPath string) (pageRasterizer, error) {
		return mockRasterizer{pages: 1, err: errors.New("render failed")}, nil
	}
	_, _, err = ex.Extract(context.Background(), "doc.pdf", 0)
	if err == nil || !strings.Contains(err.Error(), "render page") {
		t.Fatalf("unexpected render error: %v", err)
	}
}

func TestConstructors(t *testing.T) {
	if NewDefaultNativeExtractor() == nil {
		t.Fatal("expected native extractor")
	}
	if NewTesseractOCRExtractor(&ocr.OCR{}) == nil {
		t.Fatal("expected OCR extractor")
	}
	if NewOllamaVLMRescueExtractor(&llamacpp.Engine{}, 0) == nil {
		t.Fatal("expected VLM extractor")
	}
	if NewDefaultQualityDecider() == nil {
		t.Fatal("expected quality decider")
	}
}
