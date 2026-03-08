package main

import (
	"errors"
	"testing"

	"vargasparse/internal/llamacpp"
	"vargasparse/internal/ocr"
	"vargasparse/internal/pipeline"
)

func TestShouldInitVLM(t *testing.T) {
	if !shouldInitVLM(pipeline.EngineHybrid, false) {
		t.Fatal("hybrid should initialize VLM")
	}
	if !shouldInitVLM(pipeline.EngineLegacy, false) {
		t.Fatal("legacy should initialize VLM")
	}
	if !shouldInitVLM(pipeline.EngineDeterministic, true) {
		t.Fatal("deterministic + enable should initialize VLM")
	}
	if shouldInitVLM(pipeline.EngineDeterministic, false) {
		t.Fatal("deterministic without flag should not initialize VLM")
	}
}

func TestBuildRuntimeDegradesWhenDepsMissing(t *testing.T) {
	origOCR := newOCREngine
	origVLM := newVLMEngine
	defer func() {
		newOCREngine = origOCR
		newVLMEngine = origVLM
	}()

	newOCREngine = func() (*ocr.OCR, error) {
		return nil, errors.New("tesseract missing")
	}
	newVLMEngine = func(model string) (*llamacpp.Engine, error) {
		return nil, errors.New("ollama missing")
	}

	deps, err := buildRuntime(options{
		engine:          pipeline.EngineHybrid,
		enableVLMRescue: true,
		failDeps:        false,
	})
	if err != nil {
		t.Fatalf("buildRuntime error: %v", err)
	}
	if deps.native == nil || deps.quality == nil {
		t.Fatal("expected default native+quality deps")
	}
	if len(deps.warnings) < 2 {
		t.Fatalf("expected dependency warnings, got %v", deps.warnings)
	}
}

func TestBuildRuntimeFailsWhenRequiredAndFailDepsEnabled(t *testing.T) {
	origOCR := newOCREngine
	defer func() { newOCREngine = origOCR }()

	newOCREngine = func() (*ocr.OCR, error) {
		return nil, errors.New("ocr init failed")
	}

	_, err := buildRuntime(options{failDeps: true})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRuntimeWiresDepsWhenAvailable(t *testing.T) {
	origOCR := newOCREngine
	origVLM := newVLMEngine
	defer func() {
		newOCREngine = origOCR
		newVLMEngine = origVLM
	}()

	newOCREngine = func() (*ocr.OCR, error) {
		return &ocr.OCR{}, nil
	}
	newVLMEngine = func(model string) (*llamacpp.Engine, error) {
		return &llamacpp.Engine{}, nil
	}

	deps, err := buildRuntime(options{
		engine:          pipeline.EngineDeterministic,
		enableVLMRescue: true,
		failDeps:        true,
	})
	if err != nil {
		t.Fatalf("buildRuntime error: %v", err)
	}
	if deps.ocr == nil {
		t.Fatal("expected OCR extractor")
	}
	if deps.vlm == nil {
		t.Fatal("expected VLM extractor")
	}
	deps.cleanup()
}

func TestBuildRuntimeFailsWhenVLMRequiredAndFailDepsEnabled(t *testing.T) {
	origOCR := newOCREngine
	origVLM := newVLMEngine
	defer func() {
		newOCREngine = origOCR
		newVLMEngine = origVLM
	}()

	newOCREngine = func() (*ocr.OCR, error) {
		return &ocr.OCR{}, nil
	}
	newVLMEngine = func(model string) (*llamacpp.Engine, error) {
		return nil, errors.New("vlm init failed")
	}

	_, err := buildRuntime(options{
		engine:          pipeline.EngineHybrid,
		enableVLMRescue: true,
		failDeps:        true,
	})
	if err == nil {
		t.Fatal("expected VLM initialization error")
	}
}

func TestBuildRuntimeWarnsWhenVLMOptionalAndMissing(t *testing.T) {
	origOCR := newOCREngine
	origVLM := newVLMEngine
	defer func() {
		newOCREngine = origOCR
		newVLMEngine = origVLM
	}()

	newOCREngine = func() (*ocr.OCR, error) {
		return &ocr.OCR{}, nil
	}
	newVLMEngine = func(model string) (*llamacpp.Engine, error) {
		return nil, errors.New("ollama missing")
	}

	deps, err := buildRuntime(options{
		engine:          pipeline.EngineDeterministic,
		enableVLMRescue: true,
		failDeps:        false,
	})
	if err != nil {
		t.Fatalf("buildRuntime error: %v", err)
	}
	if len(deps.warnings) == 0 {
		t.Fatal("expected warning when VLM init fails with failDeps disabled")
	}
}

func TestBuildRuntimeDeterministicSkipsVLMInitWhenDisabled(t *testing.T) {
	origOCR := newOCREngine
	origVLM := newVLMEngine
	defer func() {
		newOCREngine = origOCR
		newVLMEngine = origVLM
	}()

	vlmCalls := 0
	newOCREngine = func() (*ocr.OCR, error) {
		return &ocr.OCR{}, nil
	}
	newVLMEngine = func(model string) (*llamacpp.Engine, error) {
		vlmCalls++
		return nil, errors.New("should not be called")
	}

	deps, err := buildRuntime(options{
		engine:          pipeline.EngineDeterministic,
		enableVLMRescue: false,
		failDeps:        true,
	})
	if err != nil {
		t.Fatalf("buildRuntime error: %v", err)
	}
	if deps.vlm != nil {
		t.Fatal("expected no VLM extractor when deterministic rescue is disabled")
	}
	if vlmCalls != 0 {
		t.Fatalf("expected no VLM init calls, got %d", vlmCalls)
	}
}
