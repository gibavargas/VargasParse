package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vargasparse/internal/deps"
	"vargasparse/internal/output"
	"vargasparse/internal/pipeline"
	"vargasparse/internal/progress"
	"vargasparse/internal/quality"
)

type runContext struct {
	opts         options
	dict         map[string]bool
	dictDuration time.Duration
	numWorkers   int
	runtimeDeps  *runtimeDeps
	batchMode    bool
}

func runPreflight(opts options) error {
	preflightOpts := deps.PreflightOptions{
		EngineMode:      opts.engine,
		EnableVLMRescue: opts.enableVLMRescue,
	}
	results, preflightErr := deps.Preflight(preflightOpts)
	fmt.Print(deps.FormatResults(results))

	if preflightErr == nil {
		fmt.Println()
		return nil
	}
	if opts.failDeps {
		return preflightErr
	}
	fmt.Fprintf(os.Stderr, "\n⚠ Warning: %v (continuing anyway)\n\n", preflightErr)
	return nil
}

func loadDictionary() (map[string]bool, time.Duration) {
	fmt.Print("📖 Loading dictionary... ")
	start := time.Now()
	dict := quality.EmbeddedDictionary()
	return dict, time.Since(start)
}

func printRuntimeWarnings(warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "⚠ Runtime warning: %s\n", warning)
	}
}

func runTargets(targets []target, ctx runContext) (int, bool) {
	failedCount := 0
	benchmarkFailed := false

	for i, t := range targets {
		failed, benchFailed := runTarget(i, len(targets), t, ctx)
		if failed {
			failedCount++
		}
		if benchFailed {
			benchmarkFailed = true
		}
	}

	return failedCount, benchmarkFailed
}

func runTarget(index, total int, t target, ctx runContext) (bool, bool) {
	numPages, err := pipeline.PageCount(t.PDFPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening PDF %q: %v\n", t.PDFPath, err)
		return true, false
	}

	if ctx.batchMode {
		fmt.Printf("\n[%d/%d] Processing %s\n", index+1, total, filepath.Base(t.PDFPath))
	}
	progress.PrintConfig(len(ctx.dict), ctx.dictDuration, filepath.Base(t.PDFPath), numPages, ctx.numWorkers, ctx.opts.profile)

	progressCh := make(chan progress.Event, numPages)
	printerDone, stats := progress.Printer(progressCh, numPages, progress.IsTTY())

	cfg := &pipeline.Config{
		PDFPath:            t.PDFPath,
		Dict:               ctx.dict,
		NumWorkers:         ctx.numWorkers,
		Profile:            ctx.opts.profile,
		LangHint:           ctx.opts.lang,
		PageTimeoutSec:     ctx.opts.timeoutSec,
		ModelName:          ctx.opts.model,
		EngineMode:         ctx.opts.engine,
		EnableVLMRescue:    ctx.opts.enableVLMRescue,
		NativeExtractor:    ctx.runtimeDeps.native,
		OCRExtractor:       ctx.runtimeDeps.ocr,
		VLMRescueExtractor: ctx.runtimeDeps.vlm,
		QualityDecider:     ctx.runtimeDeps.quality,
	}

	wallStart := time.Now()
	pageResults := pipeline.Run(cfg, numPages, progressCh)
	close(progressCh)
	<-printerDone
	wallDuration := time.Since(wallStart)

	pages := toPageData(pageResults)
	nonEmpty := countNonEmpty(pages)
	ext := resolveFormat(ctx.opts.format, t.OutputPath)
	writeOutputs(ext, t.OutputPath, pages, t.PDFPath, numPages, nonEmpty, wallDuration, ctx.opts.profile)

	report := output.BuildReport(t.PDFPath, pages, wallDuration)
	truthPath, truthText := inferTruthFile(t.PDFPath)
	if strings.TrimSpace(truthText) != "" {
		extracted := output.FormatTxt(pages)
		cer, wer := output.CERAndWER(extracted, truthText)
		output.AttachErrorMetrics(&report, []float64{cer}, []float64{wer})
	}

	writeReportArtifact(ctx.opts.reportPath, ctx.batchMode, t.PDFPath, report)
	benchmarkFailed := writeBenchmarkArtifact(
		ctx.opts.benchmarkPath,
		ctx.batchMode,
		t.PDFPath,
		truthPath,
		truthText,
		pages,
		report,
		ctx.opts.minPassRate,
		ctx.opts.maxFailRate,
	)

	progress.PrintSummary(stats, wallDuration, t.OutputPath, nonEmpty, numPages)
	return false, benchmarkFailed
}
