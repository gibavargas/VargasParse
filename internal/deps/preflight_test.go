package deps

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func toolByName(tools []Tool, name string) (Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func TestBuildToolsRequirementMatrix(t *testing.T) {
	tests := []struct {
		name         string
		opts         PreflightOptions
		wantRequired map[string]bool
	}{
		{
			name: "deterministic defaults",
			opts: PreflightOptions{EngineMode: EngineDeterministic},
			wantRequired: map[string]bool{
				"pdftoppm":  true,
				"pdftotext": false,
				"tesseract": true,
				"ollama":    false,
			},
		},
		{
			name: "hybrid keeps ollama optional",
			opts: PreflightOptions{EngineMode: EngineHybrid},
			wantRequired: map[string]bool{
				"ollama": false,
			},
		},
		{
			name: "legacy requires ollama",
			opts: PreflightOptions{EngineMode: EngineLegacy},
			wantRequired: map[string]bool{
				"ollama": true,
			},
		},
		{
			name: "deterministic rescue still optional ollama",
			opts: PreflightOptions{EngineMode: EngineDeterministic, EnableVLMRescue: true},
			wantRequired: map[string]bool{
				"ollama": false,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tools := buildTools(tc.opts)
			for name, wantReq := range tc.wantRequired {
				gotTool, ok := toolByName(tools, name)
				if !ok {
					t.Fatalf("tool %q not found", name)
				}
				if gotTool.Required != wantReq {
					t.Fatalf("tool %q required=%v want %v", name, gotTool.Required, wantReq)
				}
			}
		})
	}
}

func TestFormatResultsIncludesRequiredAndOptionalTags(t *testing.T) {
	results := []CheckResult{
		{
			Tool: Tool{Name: "pdftoppm", Required: true, Install: map[string]string{"darwin": "brew install poppler"}},
		},
		{
			Tool:    Tool{Name: "pdftotext", Required: false, Install: map[string]string{"darwin": "brew install poppler"}},
			Found:   true,
			Version: "pdftotext version 24.02",
		},
	}

	out := FormatResults(results)
	if !strings.Contains(out, "[REQUIRED]") {
		t.Fatalf("expected REQUIRED tag in %q", out)
	}
	if !strings.Contains(out, "pdftotext version 24.02") {
		t.Fatalf("expected version string in %q", out)
	}
}

func TestPreflightMissingRequired(t *testing.T) {
	origLookPath := lookPath
	origCommand := commandRunner
	defer func() {
		lookPath = origLookPath
		commandRunner = origCommand
	}()

	lookPath = func(file string) (string, error) {
		switch file {
		case "pdftotext":
			return "/usr/bin/pdftotext", nil
		default:
			return "", errors.New("missing")
		}
	}
	commandRunner = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo tool-version")
	}

	results, err := Preflight(PreflightOptions{EngineMode: EngineDeterministic})
	if err == nil {
		t.Fatal("expected missing required tools error")
	}
	if len(results) == 0 {
		t.Fatal("expected check results")
	}
}

func TestPreflightSuccessAndVersion(t *testing.T) {
	origLookPath := lookPath
	origCommand := commandRunner
	defer func() {
		lookPath = origLookPath
		commandRunner = origCommand
	}()

	lookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	commandRunner = func(name string, arg ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo mock-version")
	}

	results, err := Preflight(PreflightOptions{EngineMode: EngineLegacy})
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("results=%d want 4", len(results))
	}
	for _, r := range results {
		if !r.Found {
			t.Fatalf("expected %s found", r.Tool.Name)
		}
		if r.Version != "mock-version" {
			t.Fatalf("unexpected version for %s: %q", r.Tool.Name, r.Version)
		}
	}
}
