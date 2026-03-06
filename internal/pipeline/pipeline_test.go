package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vargasparse/internal/progress"
	"vargasparse/internal/quality"
	"vargasparse/internal/table"
)

type mockNativeExtractor struct {
	text   string
	source string
	err    error
}

func (m mockNativeExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, []Block, []Table, error) {
	if m.err != nil {
		return "", "", nil, nil, m.err
	}
	return m.text, m.source, buildBlock(pageIndex+1, m.text, m.source), nil, nil
}

type mockOCRExtractor struct {
	text   string
	source string
	err    error
}

func (m mockOCRExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	return m.text, m.source, nil
}

type mockVLMExtractor struct {
	text   string
	source string
	err    error
}

func (m mockVLMExtractor) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	return m.text, m.source, nil
}

func (m mockVLMExtractor) Close() error { return nil }

type timeoutNative struct{}

func (t timeoutNative) Extract(ctx context.Context, pdfPath string, pageIndex int) (string, string, []Block, []Table, error) {
	<-ctx.Done()
	return "", "", nil, nil, ctx.Err()
}

var mockDict = map[string]bool{
	"hello": true,
	"world": true,
	"clean": true,
	"text":  true,
	"from":  true,
	"ocr":   true,
	"good":  true,
}

func baseConfig() *Config {
	return &Config{
		PDFPath:        "dummy.pdf",
		Dict:           mockDict,
		NumWorkers:     2,
		Profile:        "balanced",
		LangHint:       "auto",
		PageTimeoutSec: 1,
		EngineMode:     EngineDeterministic,
		QualityDecider: defaultQualityDecider{},
	}
}

func TestParseLangs(t *testing.T) {
	tests := []struct {
		hint     string
		expected []string
	}{
		{"", []string{"por", "eng"}},
		{"auto", []string{"por", "eng"}},
		{"eng", []string{"eng"}},
		{"por,eng,spa", []string{"por", "eng", "spa"}},
	}

	for _, tc := range tests {
		t.Run(tc.hint, func(t *testing.T) {
			result := parseLangs(tc.hint)
			if len(result) != len(tc.expected) {
				t.Fatalf("expected %d langs, got %d", len(tc.expected), len(result))
			}
			for i, lang := range result {
				if lang != tc.expected[i] {
					t.Errorf("expected %q, got %q", tc.expected[i], lang)
				}
			}
		})
	}
}

func TestProcessPage_AcceptsNativeText(t *testing.T) {
	cfg := baseConfig()
	cfg.NativeExtractor = mockNativeExtractor{text: "hello world clean text", source: "pdftotext"}
	cfg.OCRExtractor = mockOCRExtractor{text: "", source: "tesseract"}

	res := processPage(context.Background(), cfg, 0)

	if res.Method != progress.MethodFast {
		t.Fatalf("method=%q want %q", res.Method, progress.MethodFast)
	}
	if strings.TrimSpace(res.Text) == "" {
		t.Fatal("expected non-empty text")
	}
	if res.ErrorCode != ErrorCodeNone {
		t.Fatalf("unexpected error code %q", res.ErrorCode)
	}
}

func TestConvertExtractedTables(t *testing.T) {
	in := []table.Table{{
		Rows: 2,
		Cols: 2,
		Box:  table.BBox{X_min: 10, Y_min: 20, X_max: 110, Y_max: 60},
		Cells: []table.Cell{
			{Row: 0, Col: 0, RowSpan: 1, ColSpan: 1, Text: "A"},
			{Row: 0, Col: 1, RowSpan: 1, ColSpan: 1, Text: "B"},
		},
	}}
	out := convertExtractedTables(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 table, got %d", len(out))
	}
	if out[0].Rows != 2 || out[0].Cols != 2 {
		t.Fatalf("unexpected shape rows=%d cols=%d", out[0].Rows, out[0].Cols)
	}
}

func TestProcessPage_RejectUsesOCR(t *testing.T) {
	cfg := baseConfig()
	cfg.NativeExtractor = mockNativeExtractor{text: "cid(123) cid(456) !!! ### xyz qqq vvv zzz", source: "pdftotext"}
	cfg.OCRExtractor = mockOCRExtractor{text: "hello world from ocr", source: "tesseract"}

	res := processPage(context.Background(), cfg, 0)

	if res.Method != progress.MethodOCR && res.Method != progress.MethodCompare {
		t.Fatalf("method=%q want ocr/compare", res.Method)
	}
	if !res.OCRApplied {
		t.Fatal("expected OCRApplied=true")
	}
	if !strings.Contains(res.Text, "ocr") {
		t.Fatalf("expected OCR text, got %q", res.Text)
	}
}

func TestProcessPage_OCRFailureSetsErrorCode(t *testing.T) {
	cfg := baseConfig()
	cfg.NativeExtractor = mockNativeExtractor{text: "", source: "pdftotext", err: errors.New("native failed")}
	cfg.OCRExtractor = mockOCRExtractor{err: errors.New("tesseract failed")}

	res := processPage(context.Background(), cfg, 0)

	if res.Method != progress.MethodOCRFail {
		t.Fatalf("method=%q want %q", res.Method, progress.MethodOCRFail)
	}
	if res.ErrorCode == "" {
		t.Fatal("expected non-empty error code")
	}
}

func TestProcessPage_DependencyMissing(t *testing.T) {
	cfg := baseConfig()
	cfg.NativeExtractor = mockNativeExtractor{text: "cid(1) cid(2) ### !!!", source: "pdftotext"}
	cfg.OCRExtractor = nil

	res := processPage(context.Background(), cfg, 0)
	if res.ErrorCode != ErrorCodeDependencyMissing {
		t.Fatalf("error_code=%q want %q", res.ErrorCode, ErrorCodeDependencyMissing)
	}
}

func TestProcessPage_VLMRescue(t *testing.T) {
	cfg := baseConfig()
	cfg.EnableVLMRescue = true
	cfg.NativeExtractor = mockNativeExtractor{text: "", source: "pdftotext"}
	cfg.OCRExtractor = mockOCRExtractor{text: "", source: "tesseract"}
	cfg.VLMRescueExtractor = mockVLMExtractor{text: "hello world rescued", source: "ollama_vlm"}

	res := processPage(context.Background(), cfg, 0)

	if strings.TrimSpace(res.Text) == "" {
		t.Fatal("expected rescue text")
	}
	if res.Method != progress.MethodCompare {
		t.Fatalf("method=%q want %q", res.Method, progress.MethodCompare)
	}
}

func TestProcessPage_ContextTimeout(t *testing.T) {
	cfg := baseConfig()
	cfg.NativeExtractor = timeoutNative{}
	cfg.OCRExtractor = mockOCRExtractor{err: context.DeadlineExceeded}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	res := processPage(ctx, cfg, 0)
	if res.ErrorCode != ErrorCodeTimeout {
		t.Fatalf("error_code=%q want %q", res.ErrorCode, ErrorCodeTimeout)
	}
}

func TestRun_ConcurrentNoDeadlock(t *testing.T) {
	cfg := baseConfig()
	cfg.NumWorkers = 8
	cfg.NativeExtractor = mockNativeExtractor{text: "hello world clean text", source: "pdftotext"}
	cfg.OCRExtractor = mockOCRExtractor{text: "", source: "tesseract"}
	cfg.QualityDecider = defaultQualityDecider{}

	progressCh := make(chan progress.Event, 100)
	res := Run(cfg, 50, progressCh)
	close(progressCh)

	if len(res) != 50 {
		t.Fatalf("got %d results, want 50", len(res))
	}
	for _, r := range res {
		if r.Method != progress.MethodFast && r.Method != progress.MethodCompare {
			t.Fatalf("unexpected method %q", r.Method)
		}
	}
}

func TestProfileDecisionAccuracy(t *testing.T) {
	q := quality.QualityResult{Confidence: 0.90, Decision: quality.Accept}
	if d := profileDecision("accuracy", q); d != quality.Compare {
		t.Fatalf("expected compare, got %v", d)
	}
}
