// VargasParse — Production-ready PDF Text Extraction Pipeline
//
// Usage: vargasparse [flags] <input.pdf|input_dir> [output.txt|output.md|output.json|output_dir]
package main

import (
	"flag"
	"fmt"
	"os"

	"vargasparse/internal/pipeline"
	"vargasparse/internal/progress"
)

func main() {
	opts := parseFlags()

	if !validEngine(opts.engine) {
		fmt.Fprintf(os.Stderr, "Error: invalid --engine %q (must be deterministic, hybrid, legacy)\n", opts.engine)
		os.Exit(1)
	}

	targets, batchMode, err := resolveTargets(flag.Args(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no PDF files found to process")
		os.Exit(1)
	}
	defer cleanupTargets(targets)

	progress.PrintBanner()

	if err := runPreflight(opts); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	dict, dictDuration := loadDictionary()
	numWorkers := pipeline.ComputeWorkers(opts.workers)

	runtimeDeps, err := buildRuntime(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing runtime: %v\n", err)
		os.Exit(1)
	}
	defer runtimeDeps.cleanup()
	printRuntimeWarnings(runtimeDeps.warnings)

	failedCount, benchmarkFailed := runTargets(targets, runContext{
		opts:         opts,
		dict:         dict,
		dictDuration: dictDuration,
		numWorkers:   numWorkers,
		runtimeDeps:  runtimeDeps,
		batchMode:    batchMode,
	})

	if benchmarkFailed {
		os.Exit(2)
	}
	if failedCount > 0 {
		os.Exit(1)
	}
}
