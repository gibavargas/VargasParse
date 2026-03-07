package main

import (
	"flag"
	"fmt"
	"os"

	"vargasparse/internal/pipeline"
)

type options struct {
	format          string
	lang            string
	profile         string
	workers         int
	reportPath      string
	failDeps        bool
	timeoutSec      int
	model           string
	engine          string
	enableVLMRescue bool
	benchmarkPath   string
	minPassRate     float64
	maxFailRate     float64
	inputURL        string
	batchPath       string
	defaultBatch    string
	batchOutputDir  string
}

func parseFlags() options {
	formatFlag := flag.String("format", "", "Output format: txt, md, json, all (default: infer from extension)")
	langFlag := flag.String("lang", "auto", "OCR languages: auto or comma-separated (e.g. por,eng)")
	profileFlag := flag.String("profile", "balanced", "Quality profile: accuracy, balanced")
	workersFlag := flag.Int("workers", 0, "Worker count override (0 = auto)")
	reportFlag := flag.String("emit-report", "", "Path (single) or directory (batch) to write extraction report JSON")
	failDepsFlag := flag.Bool("fail-on-missing-deps", false, "Exit 1 if any required dependency is missing")
	timeoutFlag := flag.Int("timeout-page-sec", 120, "Per-page timeout in seconds (0 = no timeout)")
	modelFlag := flag.String("model", "llava", "Ollama Vision-Language Model for rescue mode")

	engineFlag := flag.String("engine", pipeline.EngineDeterministic, "Engine mode: deterministic, hybrid, legacy")
	enableVLMRescueFlag := flag.Bool("enable-vlm-rescue", false, "Enable optional VLM rescue for hard pages")
	benchmarkReportFlag := flag.String("benchmark-report", "", "Path (single) or directory (batch) to write benchmark report JSON")
	minPassRateFlag := flag.Float64("min-pass-rate", 97.0, "Minimum acceptable pass rate percentage for benchmark gate")
	maxFailRateFlag := flag.Float64("max-fail-rate", 0.5, "Maximum acceptable fail rate percentage for benchmark gate")

	inputURLFlag := flag.String("input-url", "", "Download and process a single PDF from URL")
	batchPathFlag := flag.String("batch-path", "", "Process all PDFs from this directory")
	defaultBatchPathFlag := flag.String("default-batch-path", "test_pdfs", "Default batch directory used when no input path is provided")
	batchOutputDirFlag := flag.String("batch-output-dir", "", "Output directory for batch mode (default: same as batch directory)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: vargasparse [flags] <input.pdf|input_dir> [output_path_or_dir]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	return options{
		format:          *formatFlag,
		lang:            *langFlag,
		profile:         *profileFlag,
		workers:         *workersFlag,
		reportPath:      *reportFlag,
		failDeps:        *failDepsFlag,
		timeoutSec:      *timeoutFlag,
		model:           *modelFlag,
		engine:          *engineFlag,
		enableVLMRescue: *enableVLMRescueFlag,
		benchmarkPath:   *benchmarkReportFlag,
		minPassRate:     *minPassRateFlag,
		maxFailRate:     *maxFailRateFlag,
		inputURL:        *inputURLFlag,
		batchPath:       *batchPathFlag,
		defaultBatch:    *defaultBatchPathFlag,
		batchOutputDir:  *batchOutputDirFlag,
	}
}

func validEngine(v string) bool {
	switch v {
	case pipeline.EngineDeterministic, pipeline.EngineHybrid, pipeline.EngineLegacy:
		return true
	default:
		return false
	}
}
