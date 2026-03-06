package output

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func samplePages() []PageData {
	return []PageData{
		{PageNum: 1, Text: "Hello world", Method: "fast", Confidence: 0.95, DurationMs: 10},
		{PageNum: 2, Text: "Olá mundo", Method: "ocr", Confidence: 0.82, DurationMs: 2500},
		{PageNum: 3, Text: "", Method: "ocr-fail", Confidence: 0, DurationMs: 3000, Warnings: []string{"timeout"}},
	}
}

func TestFormatTxt(t *testing.T) {
	result := FormatTxt(samplePages())
	if !strings.Contains(result, "Hello world") {
		t.Error("missing page 1 text")
	}
	if !strings.Contains(result, "Olá mundo") {
		t.Error("missing page 2 text")
	}
	if strings.Contains(result, "ocr-fail") {
		t.Error("should not contain method names")
	}
	// Empty page should be skipped
	parts := strings.Split(result, "---")
	if len(parts) != 2 {
		t.Errorf("expected 2 parts (2 non-empty pages), got %d", len(parts))
	}
}

func TestFormatMd(t *testing.T) {
	result := FormatMd(samplePages())
	if !strings.Contains(result, "## Página 1") {
		t.Error("missing page 1 heading")
	}
	if !strings.Contains(result, "## Página 2") {
		t.Error("missing page 2 heading")
	}
	// Page 3 is empty — should not have a heading
	if strings.Contains(result, "## Página 3") {
		t.Error("should not include empty page heading")
	}
}

func TestFormatJSON(t *testing.T) {
	doc := Document{
		SourceFile: "test.pdf",
		TotalPages: 3,
		Pages:      samplePages(),
	}
	result, err := FormatJSON(doc)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}

	var parsed Document
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("cannot round-trip JSON: %v", err)
	}
	if parsed.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", parsed.TotalPages)
	}
	if len(parsed.Pages) != 3 {
		t.Errorf("len(Pages) = %d, want 3", len(parsed.Pages))
	}
}

func TestBuildReport(t *testing.T) {
	pages := samplePages()
	report := BuildReport("test.pdf", pages, 5000)

	if report.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", report.TotalPages)
	}
	// Page 1 (conf 0.95) and Page 2 (conf 0.82) pass; Page 3 (empty) fails
	expectedPassRate := float64(2) / float64(3) * 100
	if report.PassRate != expectedPassRate {
		t.Errorf("PassRate = %.1f, want %.1f", report.PassRate, expectedPassRate)
	}
	wantFail := 100 - expectedPassRate
	if math.Abs(report.FailRate-wantFail) > 0.001 {
		t.Errorf("FailRate = %.4f, want %.4f", report.FailRate, wantFail)
	}
}

func TestCERAndWER(t *testing.T) {
	ext := "hello world"
	truth := "hello brave world"
	cer, wer := CERAndWER(ext, truth)
	if cer <= 0 {
		t.Fatalf("expected CER > 0, got %.4f", cer)
	}
	if wer <= 0 {
		t.Fatalf("expected WER > 0, got %.4f", wer)
	}
}

func TestBuildBenchmarkReport(t *testing.T) {
	pages := samplePages()
	report := BuildReport("test.pdf", pages, 5000)
	b := BuildBenchmarkReport(
		"test.pdf",
		"truth.md",
		"hello world",
		"hello world",
		report,
		60,
		50,
	)
	if !b.Passed {
		t.Fatalf("expected benchmark to pass, reason=%q", b.FailureReason)
	}

	b2 := BuildBenchmarkReport(
		"test.pdf",
		"truth.md",
		"hello world",
		"hello world",
		report,
		99.9,
		0.1,
	)
	if b2.Passed {
		t.Fatal("expected benchmark to fail")
	}
}

func TestBuildBenchmarkReport_MissingTruthFails(t *testing.T) {
	report := BuildReport("test.pdf", samplePages(), 5000)
	b := BuildBenchmarkReport("test.pdf", "", "hello", "", report, 0, 100)
	if b.Passed {
		t.Fatal("expected failure when truth is missing")
	}
	if b.TruthPresent {
		t.Fatal("expected TruthPresent=false")
	}
}
