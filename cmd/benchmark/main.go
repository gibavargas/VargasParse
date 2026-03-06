package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type manifest struct {
	Version int             `json:"version"`
	Entries []manifestEntry `json:"entries"`
}

type manifestEntry struct {
	Name           string  `json:"name"`
	PDF            string  `json:"pdf"`
	Truth          string  `json:"truth"`
	MinPassRatePct float64 `json:"min_pass_rate_pct"`
	MaxFailRatePct float64 `json:"max_fail_rate_pct"`
}

type entryResult struct {
	Name       string `json:"name"`
	PDF        string `json:"pdf"`
	ReportPath string `json:"report_path"`
	Passed     bool   `json:"passed"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error,omitempty"`
}

type summary struct {
	Manifest string        `json:"manifest"`
	Passed   bool          `json:"passed"`
	Results  []entryResult `json:"results"`
}

func main() {
	manifestPath := flag.String("manifest", "test_pdfs/corpus_manifest.json", "Path to benchmark manifest")
	binaryPath := flag.String("binary", "./vargasparse", "Path to vargasparse binary")
	outDir := flag.String("out-dir", "/tmp/vargasparse-benchmark", "Directory for benchmark artifacts")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create output directory: %v\n", err)
		os.Exit(1)
	}

	data, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read manifest: %v\n", err)
		os.Exit(1)
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "invalid manifest json: %v\n", err)
		os.Exit(1)
	}
	if len(m.Entries) == 0 {
		fmt.Fprintln(os.Stderr, "manifest has no entries")
		os.Exit(1)
	}

	s := summary{Manifest: *manifestPath, Passed: true}

	for _, e := range m.Entries {
		truthBytes, truthErr := os.ReadFile(e.Truth)
		if truthErr != nil || len(strings.TrimSpace(string(truthBytes))) == 0 {
			res := entryResult{
				Name:     e.Name,
				PDF:      e.PDF,
				Passed:   false,
				ExitCode: 2,
				Error:    "missing or empty truth file: " + e.Truth,
			}
			s.Passed = false
			s.Results = append(s.Results, res)
			continue
		}

		reportPath := filepath.Join(*outDir, e.Name+".benchmark.json")
		outputPath := filepath.Join(*outDir, e.Name+".txt")

		cmd := exec.Command(
			*binaryPath,
			"--engine", "deterministic",
			"--benchmark-report", reportPath,
			"--min-pass-rate", fmt.Sprintf("%.2f", e.MinPassRatePct),
			"--max-fail-rate", fmt.Sprintf("%.2f", e.MaxFailRatePct),
			e.PDF,
			outputPath,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()

		res := entryResult{
			Name:       e.Name,
			PDF:        e.PDF,
			ReportPath: reportPath,
			Passed:     err == nil,
		}
		if err != nil {
			res.Error = err.Error()
			s.Passed = false
			if exitErr, ok := err.(*exec.ExitError); ok {
				res.ExitCode = exitErr.ExitCode()
			} else {
				res.ExitCode = 1
			}
		}
		s.Results = append(s.Results, res)
	}

	summaryPath := filepath.Join(*outDir, "summary.json")
	encoded, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(summaryPath, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write summary: %v\n", err)
		os.Exit(1)
	}

	if !s.Passed {
		fmt.Fprintf(os.Stderr, "benchmark gate failed; see %s\n", summaryPath)
		os.Exit(2)
	}

	fmt.Printf("benchmark gate passed; summary: %s\n", summaryPath)
}
