// Package deps checks that required external tools are installed
// and provides actionable install hints when they are missing.
package deps

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var (
	lookPath      = exec.LookPath
	commandRunner = exec.Command
)

const (
	EngineDeterministic = "deterministic"
	EngineHybrid        = "hybrid"
	EngineLegacy        = "legacy"
)

// PreflightOptions controls required/optional tool checks.
type PreflightOptions struct {
	EngineMode      string
	EnableVLMRescue bool
}

// Tool describes an external dependency with install hints.
type Tool struct {
	Name     string            // binary name looked up on PATH
	Purpose  string            // what we use it for
	Required bool              // true = pipeline cannot work without it
	Install  map[string]string // os → install command
}

// CheckResult holds the result of checking one tool.
type CheckResult struct {
	Tool    Tool
	Found   bool
	Path    string // resolved path if found
	Version string // version string if found
}

func buildTools(opts PreflightOptions) []Tool {
	if opts.EngineMode == "" {
		opts.EngineMode = EngineDeterministic
	}

	needsVLM := opts.EnableVLMRescue || opts.EngineMode == EngineHybrid || opts.EngineMode == EngineLegacy

	tools := []Tool{
		{
			Name: "pdftoppm", Purpose: "rasterise PDF pages for OCR",
			Required: true,
			Install: map[string]string{
				"darwin": "brew install poppler",
				"linux":  "apt-get install -y poppler-utils",
			},
		},
		{
			Name: "pdftotext", Purpose: "extract text layer from PDF pages",
			Required: false,
			Install: map[string]string{
				"darwin": "brew install poppler",
				"linux":  "apt-get install -y poppler-utils",
			},
		},
		{
			Name: "tesseract", Purpose: "OCR engine for rasterised pages",
			Required: true,
			Install: map[string]string{
				"darwin": "brew install tesseract tesseract-lang",
				"linux":  "apt-get install -y tesseract-ocr tesseract-ocr-por tesseract-ocr-eng",
			},
		},
		{
			Name: "ollama", Purpose: "optional VLM rescue for hard pages",
			Required: needsVLM && opts.EngineMode == EngineLegacy,
			Install: map[string]string{
				"darwin": "brew install ollama",
				"linux":  "curl -fsSL https://ollama.com/install.sh | sh",
			},
		},
	}
	return tools
}

// Preflight checks tool dependencies for the selected runtime mode.
func Preflight(opts PreflightOptions) ([]CheckResult, error) {
	tools := buildTools(opts)

	var results []CheckResult
	var missing []string

	for _, t := range tools {
		cr := CheckResult{Tool: t}
		path, err := lookPath(t.Name)
		if err == nil {
			cr.Found = true
			cr.Path = path
			if out, err := commandRunner(path, "--version").CombinedOutput(); err == nil {
				lines := strings.SplitN(string(out), "\n", 2)
				cr.Version = strings.TrimSpace(lines[0])
			}
		} else if t.Required {
			missing = append(missing, t.Name)
		}
		results = append(results, cr)
	}

	if len(missing) > 0 {
		return results, fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
	}
	return results, nil
}

// FormatResults returns a human-readable summary of the preflight check.
func FormatResults(results []CheckResult) string {
	var sb strings.Builder
	for _, r := range results {
		if r.Found {
			sb.WriteString(fmt.Sprintf("  ✓ %-12s  %s\n", r.Tool.Name, r.Version))
		} else {
			hint := r.Tool.Install[runtime.GOOS]
			if hint == "" {
				hint = "(install manually)"
			}
			tag := "optional"
			if r.Tool.Required {
				tag = "REQUIRED"
			}
			sb.WriteString(fmt.Sprintf("  ✗ %-12s  [%s] — %s\n", r.Tool.Name, tag, hint))
		}
	}
	return sb.String()
}
