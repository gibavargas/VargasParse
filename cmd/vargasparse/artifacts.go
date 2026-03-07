package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vargasparse/internal/output"
	"vargasparse/internal/pipeline"
	"vargasparse/internal/progress"
)

func countNonEmpty(pages []output.PageData) int {
	n := 0
	for _, p := range pages {
		if strings.TrimSpace(p.Text) != "" {
			n++
		}
	}
	return n
}

func resolveFormat(formatFlag, outputPath string) string {
	if formatFlag != "" {
		switch formatFlag {
		case "txt":
			return ".txt"
		case "md":
			return ".md"
		case "json":
			return ".json"
		case "all":
			return ".all"
		}
	}
	ext := strings.ToLower(filepath.Ext(outputPath))
	if ext != ".md" && ext != ".json" && ext != ".txt" {
		ext = ".txt"
	}
	return ext
}

func toPageData(results []pipeline.PageResult) []output.PageData {
	pages := make([]output.PageData, len(results))
	for i, r := range results {
		pages[i] = output.PageData{
			PageNum:          r.PageNum,
			Text:             r.Text,
			Method:           r.Method,
			Confidence:       r.Confidence,
			DurationMs:       r.Duration.Milliseconds(),
			Warnings:         r.Warnings,
			Blocks:           convertBlocks(r.Blocks),
			Tables:           convertTables(r.Tables),
			EngineTrace:      r.EngineTrace,
			QualitySignals:   r.QualitySignals,
			LanguageDetected: r.LanguageDetected,
			OCRApplied:       r.OCRApplied,
			ErrorCode:        r.ErrorCode,
		}
	}
	return pages
}

func convertBlocks(pBlocks []pipeline.Block) []output.Block {
	if len(pBlocks) == 0 {
		return nil
	}
	out := make([]output.Block, len(pBlocks))
	for i, b := range pBlocks {
		out[i] = output.Block{
			ID:           b.ID,
			Type:         b.Type,
			Text:         b.Text,
			ReadingOrder: b.ReadingOrder,
			Confidence:   b.Confidence,
			SourceMethod: b.SourceMethod,
		}
		if b.BBox != nil {
			out[i].BBox = &output.BBox{X: b.BBox.X, Y: b.BBox.Y, W: b.BBox.W, H: b.BBox.H}
		}
	}
	return out
}

func convertTables(pTables []pipeline.Table) []output.Table {
	if len(pTables) == 0 {
		return nil
	}
	out := make([]output.Table, len(pTables))
	for i, t := range pTables {
		out[i] = output.Table{ID: t.ID, Rows: t.Rows, Cols: t.Cols}
		if t.BBox != nil {
			out[i].BBox = &output.BBox{X: t.BBox.X, Y: t.BBox.Y, W: t.BBox.W, H: t.BBox.H}
		}
		for _, c := range t.Cells {
			out[i].Cells = append(out[i].Cells, output.Cell{
				Row: c.Row, Col: c.Col,
				RowSpan: c.RowSpan, ColSpan: c.ColSpan,
				Text: c.Text,
			})
		}
	}
	return out
}

func writeOutputs(ext, outputPath string, pages []output.PageData, pdfPath string, numPages, nonEmpty int, wallDuration time.Duration, profile string) {
	switch ext {
	case ".all":
		base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
		if base == "" {
			base = strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
		}
		writeFile(base+".txt", output.FormatTxt(pages))
		writeFile(base+".md", output.FormatMd(pages))
		writeJSON(base+".json", pages, pdfPath, numPages, nonEmpty, wallDuration, profile)
	case ".json":
		writeJSON(outputPath, pages, pdfPath, numPages, nonEmpty, wallDuration, profile)
	case ".md":
		writeFile(outputPath, output.FormatMd(pages))
	default:
		writeFile(outputPath, output.FormatTxt(pages))
	}
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
	}
}

func writeJSON(path string, pages []output.PageData, pdfPath string, numPages, nonEmpty int, wallDuration time.Duration, profile string) {
	doc := output.Document{
		SourceFile:    pdfPath,
		TotalPages:    numPages,
		PagesWithText: nonEmpty,
		ProcessingMs:  wallDuration.Milliseconds(),
		Profile:       profile,
		Pages:         pages,
	}

	for _, p := range pages {
		switch p.Method {
		case progress.MethodFast:
			doc.Metrics.FastPages++
		case progress.MethodOCR:
			doc.Metrics.OCRPages++
		case progress.MethodCompare:
			doc.Metrics.ComparePages++
		case progress.MethodOCRFail:
			doc.Metrics.FailedPages++
		case progress.MethodSkip:
			doc.Metrics.SkippedPages++
		}
	}

	jsonStr, err := output.FormatJSON(doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		return
	}
	writeFile(path, jsonStr)
}

func writeReportArtifact(pathOrDir string, batchMode bool, pdfPath string, report output.Report) {
	reportPath := resolvePerFilePath(pathOrDir, batchMode, pdfPath, ".report.json")
	if reportPath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		return
	}

	reportJSON, err := output.FormatReport(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building report for %s: %v\n", pdfPath, err)
		return
	}
	if err := os.WriteFile(reportPath, []byte(reportJSON), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report for %s: %v\n", pdfPath, err)
	}
}

func writeBenchmarkArtifact(pathOrDir string, batchMode bool, pdfPath, truthPath, truthText string, pages []output.PageData, report output.Report, minPassRate, maxFailRate float64) bool {
	benchmarkPath := resolvePerFilePath(pathOrDir, batchMode, pdfPath, ".benchmark.json")
	if benchmarkPath == "" {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(benchmarkPath), 0755); err != nil {
		return false
	}

	benchmark := output.BuildBenchmarkReport(
		pdfPath,
		truthPath,
		output.FormatTxt(pages),
		truthText,
		report,
		minPassRate,
		maxFailRate,
	)
	bJSON, err := output.FormatBenchmarkReport(benchmark)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building benchmark report for %s: %v\n", pdfPath, err)
		return false
	}
	if err := os.WriteFile(benchmarkPath, []byte(bJSON), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing benchmark report for %s: %v\n", pdfPath, err)
	}
	if !benchmark.Passed {
		fmt.Fprintf(os.Stderr, "Benchmark gate failed for %s: %s\n", pdfPath, benchmark.FailureReason)
		return true
	}
	return false
}

func inferTruthFile(pdfPath string) (string, string) {
	base := strings.TrimSuffix(pdfPath, filepath.Ext(pdfPath))
	truth := base + ".md"
	b, err := os.ReadFile(truth)
	if err != nil {
		return "", ""
	}
	return truth, string(b)
}
