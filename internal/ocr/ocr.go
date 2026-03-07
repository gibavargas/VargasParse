package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var lookPath = exec.LookPath

type command interface {
	CombinedOutput() ([]byte, error)
	Output() ([]byte, error)
}

// OCR wraps local OCR toolchain calls (pdftoppm + tesseract).
type OCR struct {
	pdftoppmPath  string
	tesseractPath string

	makeCommand func(ctx context.Context, name string, args ...string) command
	mkTempDir   func(dir, pattern string) (string, error)
	glob        func(pattern string) ([]string, error)
	removeAll   func(path string) error
}

// NewOCR validates that OCR dependencies are available.
func NewOCR() (*OCR, error) {
	pdftoppmPath, err := lookPath("pdftoppm")
	if err != nil {
		return nil, fmt.Errorf("pdftoppm not found: %w", err)
	}
	tesseractPath, err := lookPath("tesseract")
	if err != nil {
		return nil, fmt.Errorf("tesseract not found: %w", err)
	}
	return &OCR{
		pdftoppmPath:  pdftoppmPath,
		tesseractPath: tesseractPath,
	}, nil
}

func (o *OCR) commandFactory(ctx context.Context, name string, args ...string) command {
	if o.makeCommand != nil {
		return o.makeCommand(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}

func (o *OCR) tempDir(dir, pattern string) (string, error) {
	if o.mkTempDir != nil {
		return o.mkTempDir(dir, pattern)
	}
	return os.MkdirTemp(dir, pattern)
}

func (o *OCR) globFiles(pattern string) ([]string, error) {
	if o.glob != nil {
		return o.glob(pattern)
	}
	return filepath.Glob(pattern)
}

func (o *OCR) cleanup(path string) {
	if o.removeAll != nil {
		_ = o.removeAll(path)
		return
	}
	_ = os.RemoveAll(path)
}

// ProcessPage rasterizes a PDF page to PNG and runs Tesseract OCR.
// pageIndex is 0-based.
func (o *OCR) ProcessPage(ctx context.Context, pdfPath string, pageIndex int, langHint string) (string, float64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	pageNum := pageIndex + 1
	tmpDir, err := o.tempDir("", "vargasparse-ocr-*")
	if err != nil {
		return "", 0, fmt.Errorf("mktemp: %w", err)
	}
	defer o.cleanup(tmpDir)

	prefix := filepath.Join(tmpDir, "page")
	rasterCmd := o.commandFactory(
		ctx,
		o.pdftoppmPath,
		"-f", fmt.Sprintf("%d", pageNum),
		"-l", fmt.Sprintf("%d", pageNum),
		"-png",
		"-r", "300",
		pdfPath,
		prefix,
	)
	if out, err := rasterCmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(out)))
	}

	pngFiles, err := o.globFiles(prefix + "*.png")
	if err != nil {
		return "", 0, fmt.Errorf("glob png: %w", err)
	}
	if len(pngFiles) == 0 {
		return "", 0, fmt.Errorf("no rasterized page found")
	}

	lang := buildTesseractLang(langHint)
	args := []string{pngFiles[0], "stdout"}
	if lang != "" {
		args = append(args, "-l", lang)
	}
	// Treat page as a uniform text block for stability on mixed PDFs.
	args = append(args, "--psm", "6")

	ocrCmd := o.commandFactory(ctx, o.tesseractPath, args...)
	out, err := ocrCmd.Output()
	if err != nil {
		return "", 0, fmt.Errorf("tesseract: %w", err)
	}

	return string(out), 0, nil
}

func buildTesseractLang(langHint string) string {
	if langHint == "" || strings.EqualFold(langHint, "auto") {
		return "por+eng"
	}
	parts := strings.Split(langHint, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return "por+eng"
	}
	return strings.Join(out, "+")
}
