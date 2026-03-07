package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vargasparse/internal/output"
	"vargasparse/internal/pipeline"
	"vargasparse/internal/progress"
)

func writePDF(t *testing.T, path string, valid bool) {
	t.Helper()
	content := "not a pdf"
	if valid {
		content = "%PDF-1.7\n%test\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func TestCollectPDFsFiltersFake(t *testing.T) {
	dir := t.TempDir()
	writePDF(t, filepath.Join(dir, "ok.pdf"), true)
	writePDF(t, filepath.Join(dir, "fake.pdf"), false)
	writePDF(t, filepath.Join(dir, "doc.txt"), true)

	got := collectPDFs(dir)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 (%v)", len(got), got)
	}
	if !strings.HasSuffix(got[0], "ok.pdf") {
		t.Fatalf("unexpected file %v", got[0])
	}
}

func TestResolveBatchTargetsOutputDir(t *testing.T) {
	dir := t.TempDir()
	outDir := t.TempDir()
	writePDF(t, filepath.Join(dir, "a.pdf"), true)
	writePDF(t, filepath.Join(dir, "b.pdf"), true)

	targets := resolveBatchTargets(dir, "", options{batchOutputDir: outDir})
	if len(targets) != 2 {
		t.Fatalf("len=%d want 2", len(targets))
	}
	for _, tg := range targets {
		if filepath.Dir(tg.OutputPath) != outDir {
			t.Fatalf("output dir mismatch: %s", tg.OutputPath)
		}
	}
}

func TestResolveTargetsSingleInput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "one.pdf")
	writePDF(t, p, true)

	targets, batch, err := resolveTargets([]string{p}, options{})
	if err != nil {
		t.Fatalf("resolveTargets error: %v", err)
	}
	if batch {
		t.Fatal("expected single mode")
	}
	if len(targets) != 1 {
		t.Fatalf("len=%d want 1", len(targets))
	}
	if targets[0].OutputPath != "one.txt" {
		t.Fatalf("output=%q want one.txt", targets[0].OutputPath)
	}
}

func TestResolveTargetsDefaultBatch(t *testing.T) {
	dir := t.TempDir()
	writePDF(t, filepath.Join(dir, "one.pdf"), true)

	targets, batch, err := resolveTargets(nil, options{defaultBatch: dir})
	if err != nil {
		t.Fatalf("resolveTargets error: %v", err)
	}
	if !batch {
		t.Fatal("expected batch mode")
	}
	if len(targets) != 1 {
		t.Fatalf("len=%d want 1", len(targets))
	}
}

func TestResolveTargetsInputURL(t *testing.T) {
	origHTTPGet := httpGet
	defer func() { httpGet = origHTTPGet }()
	httpGet = func(url string) (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("%PDF-1.7\nurl\n")),
			Request:    req,
		}, nil
	}

	targets, batch, err := resolveTargets(nil, options{inputURL: "https://example.com/sample.pdf"})
	if err != nil {
		t.Fatalf("resolveTargets error: %v", err)
	}
	if batch {
		t.Fatal("expected single mode for URL input")
	}
	if len(targets) != 1 {
		t.Fatalf("len=%d want 1", len(targets))
	}
	if targets[0].CleanupFile == "" {
		t.Fatal("expected cleanup file")
	}
	if !strings.HasSuffix(targets[0].OutputPath, ".txt") {
		t.Fatalf("unexpected output path %q", targets[0].OutputPath)
	}

	cleanupTargets(targets)
	if _, err := os.Stat(targets[0].CleanupFile); !os.IsNotExist(err) {
		t.Fatalf("cleanup file still exists: %s", targets[0].CleanupFile)
	}
}

func TestResolvePerFilePath(t *testing.T) {
	p := resolvePerFilePath("/tmp/reports", true, "/tmp/invoice.pdf", ".report.json")
	if p != "/tmp/reports/invoice.report.json" {
		t.Fatalf("unexpected path %q", p)
	}
	if single := resolvePerFilePath("/tmp/out.json", false, "/tmp/invoice.pdf", ".report.json"); single != "/tmp/out.json" {
		t.Fatalf("unexpected single path %q", single)
	}
}

func TestValidEngine(t *testing.T) {
	if !validEngine("deterministic") || !validEngine("hybrid") || !validEngine("legacy") {
		t.Fatal("expected valid engines to pass")
	}
	if validEngine("x") {
		t.Fatal("expected invalid engine to fail")
	}
}

func TestFormatAndCountHelpers(t *testing.T) {
	if got := resolveFormat("txt", "out.json"); got != ".txt" {
		t.Fatalf("resolveFormat flag txt got %q", got)
	}
	if got := resolveFormat("", "out.md"); got != ".md" {
		t.Fatalf("resolveFormat extension got %q", got)
	}
	pages := []output.PageData{{Text: "a"}, {Text: " "}, {Text: "b"}}
	if got := countNonEmpty(pages); got != 2 {
		t.Fatalf("countNonEmpty=%d want 2", got)
	}
}

func TestToPageDataAndWriteOutputs(t *testing.T) {
	results := []pipeline.PageResult{
		{
			PageNum:    1,
			Text:       "hello",
			Method:     progress.MethodFast,
			Confidence: 0.9,
			Duration:   25 * time.Millisecond,
			Blocks: []pipeline.Block{{
				ID:           "b1",
				Type:         "paragraph",
				Text:         "hello",
				ReadingOrder: 0,
				Confidence:   0.9,
				SourceMethod: "pdftotext",
			}},
			Tables: []pipeline.Table{{
				ID:   "t1",
				Rows: 1,
				Cols: 1,
				Cells: []pipeline.Cell{{
					Row: 0, Col: 0, RowSpan: 1, ColSpan: 1, Text: "x",
				}},
			}},
		},
	}
	pages := toPageData(results)
	if len(pages) != 1 || len(pages[0].Blocks) != 1 || len(pages[0].Tables) != 1 {
		t.Fatalf("unexpected page conversion: %+v", pages)
	}

	dir := t.TempDir()
	txtPath := filepath.Join(dir, "out.txt")
	mdPath := filepath.Join(dir, "out.md")
	jsonPath := filepath.Join(dir, "out.json")
	allBase := filepath.Join(dir, "multi.any")

	writeOutputs(".txt", txtPath, pages, "in.pdf", 1, 1, 100*time.Millisecond, "balanced")
	writeOutputs(".md", mdPath, pages, "in.pdf", 1, 1, 100*time.Millisecond, "balanced")
	writeOutputs(".json", jsonPath, pages, "in.pdf", 1, 1, 100*time.Millisecond, "balanced")
	writeOutputs(".all", allBase, pages, "in.pdf", 1, 1, 100*time.Millisecond, "balanced")

	for _, p := range []string{
		txtPath, mdPath, jsonPath,
		strings.TrimSuffix(allBase, ".any") + ".txt",
		strings.TrimSuffix(allBase, ".any") + ".md",
		strings.TrimSuffix(allBase, ".any") + ".json",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected output file %s: %v", p, err)
		}
	}
}

func TestInferTruthFile(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "sample.pdf")
	mdPath := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.7\nx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mdPath, []byte("ground truth"), 0644); err != nil {
		t.Fatal(err)
	}

	truthPath, truth := inferTruthFile(pdfPath)
	if truthPath != mdPath || truth != "ground truth" {
		t.Fatalf("unexpected truth file result: %q %q", truthPath, truth)
	}
}
