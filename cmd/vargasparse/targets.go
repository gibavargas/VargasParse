package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var httpGet = http.Get

type target struct {
	PDFPath     string
	OutputPath  string
	CleanupFile string
}

func resolveTargets(args []string, opts options) ([]target, bool, error) {
	var explicitOutput string
	if len(args) > 1 {
		explicitOutput = args[1]
	}

	if opts.inputURL != "" {
		pdfPath, cleanup, err := downloadPDF(opts.inputURL)
		if err != nil {
			return nil, false, err
		}
		out := explicitOutput
		if out == "" {
			name := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
			out = name + ".txt"
		}
		return []target{{PDFPath: pdfPath, OutputPath: out, CleanupFile: cleanup}}, false, nil
	}

	if opts.batchPath != "" {
		return resolveBatchTargets(opts.batchPath, explicitOutput, opts), true, nil
	}

	if len(args) >= 1 {
		in := args[0]
		st, err := os.Stat(in)
		if err != nil {
			return nil, false, fmt.Errorf("cannot open %q: %w", in, err)
		}
		if st.IsDir() {
			return resolveBatchTargets(in, explicitOutput, opts), true, nil
		}
		out := explicitOutput
		if out == "" {
			base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
			out = base + ".txt"
		}
		return []target{{PDFPath: in, OutputPath: out}}, false, nil
	}

	if opts.defaultBatch != "" {
		return resolveBatchTargets(opts.defaultBatch, explicitOutput, opts), true, nil
	}

	return nil, false, fmt.Errorf("no input specified")
}

func resolveBatchTargets(batchPath, explicitOutput string, opts options) []target {
	pdfs := collectPDFs(batchPath)
	if len(pdfs) == 0 {
		return []target{}
	}
	outDir := opts.batchOutputDir
	if outDir == "" {
		if explicitOutput != "" {
			outDir = explicitOutput
		} else {
			outDir = batchPath
		}
	}
	_ = os.MkdirAll(outDir, 0755)

	targets := make([]target, 0, len(pdfs))
	for _, pdfPath := range pdfs {
		base := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		out := filepath.Join(outDir, base+".txt")
		targets = append(targets, target{PDFPath: pdfPath, OutputPath: out})
	}
	return targets
}

func collectPDFs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			full := filepath.Join(root, e.Name())
			if isLikelyPDF(full) {
				out = append(out, full)
			}
		}
	}
	sort.Strings(out)
	return out
}

func isLikelyPDF(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil || n < 5 {
		return false
	}
	return string(buf) == "%PDF-"
}

func downloadPDF(inputURL string) (tmpPath string, cleanupPath string, err error) {
	resp, err := httpGet(inputURL)
	if err != nil {
		return "", "", fmt.Errorf("download url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	fileName := path.Base(resp.Request.URL.Path)
	if fileName == "." || fileName == "/" || fileName == "" {
		fileName = "downloaded.pdf"
	}
	if !strings.EqualFold(filepath.Ext(fileName), ".pdf") {
		fileName += ".pdf"
	}

	tmpFile, err := os.CreateTemp("", "vargasparse-url-*.pdf")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", "", fmt.Errorf("write temp file: %w", err)
	}
	return tmpFile.Name(), tmpFile.Name(), nil
}

func cleanupTargets(targets []target) {
	for _, t := range targets {
		if t.CleanupFile != "" {
			_ = os.Remove(t.CleanupFile)
		}
	}
}

func resolvePerFilePath(pathOrDir string, batchMode bool, pdfPath string, suffix string) string {
	if pathOrDir == "" {
		return ""
	}
	if !batchMode {
		return pathOrDir
	}
	base := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	return filepath.Join(pathOrDir, base+suffix)
}
