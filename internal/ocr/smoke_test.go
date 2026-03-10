package ocr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSmoke_OCR(t *testing.T) {
	if os.Getenv("VARGASPARSE_SMOKE") != "1" {
		t.Skip("set VARGASPARSE_SMOKE=1 to run smoke OCR test")
	}

	pdfPath := filepath.Join("..", "..", "test_pdfs", "attention.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Fatalf("smoke PDF not found: %v", err)
	}

	engine, err := NewOCR()
	if err != nil {
		t.Fatalf("NewOCR: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, _, err := engine.ProcessPage(ctx, pdfPath, 0, "por,eng")
	if err != nil {
		t.Fatalf("ProcessPage: %v", err)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("expected non-empty OCR text")
	}
}
