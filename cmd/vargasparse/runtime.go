package main

import (
	"fmt"

	"vargasparse/internal/llamacpp"
	"vargasparse/internal/ocr"
	"vargasparse/internal/pipeline"
)

var (
	newOCREngine = ocr.NewOCR
	newVLMEngine = llamacpp.NewEngine
)

type runtimeDeps struct {
	quality  pipeline.QualityDecider
	native   pipeline.NativeExtractor
	ocr      pipeline.OCRExtractor
	vlm      pipeline.VLMRescueExtractor
	warnings []string
	cleanup  func()
}

func buildRuntime(opts options) (*runtimeDeps, error) {
	deps := &runtimeDeps{
		quality: pipeline.NewDefaultQualityDecider(),
		native:  pipeline.NewDefaultNativeExtractor(),
		cleanup: func() {},
	}

	ocrEngine, err := newOCREngine()
	if err != nil {
		if opts.failDeps {
			return nil, fmt.Errorf("initialize OCR extractor: %w", err)
		}
		deps.warnings = append(deps.warnings, fmt.Sprintf("OCR unavailable: %v", err))
	} else {
		deps.ocr = pipeline.NewTesseractOCRExtractor(ocrEngine)
	}

	if shouldInitVLM(opts.engine, opts.enableVLMRescue) {
		model := opts.model
		if model == "" {
			model = "llava"
		}
		vlmEngine, err := newVLMEngine(model)
		if err != nil {
			if opts.failDeps {
				return nil, fmt.Errorf("initialize VLM rescue extractor: %w", err)
			}
			deps.warnings = append(deps.warnings, fmt.Sprintf("VLM rescue unavailable: %v", err))
		} else {
			ex := pipeline.NewOllamaVLMRescueExtractor(vlmEngine)
			deps.vlm = ex
			deps.cleanup = func() { _ = ex.Close() }
		}
	}

	return deps, nil
}

func shouldInitVLM(engine string, enableVLMRescue bool) bool {
	switch engine {
	case pipeline.EngineLegacy, pipeline.EngineHybrid:
		return true
	default:
		return enableVLMRescue
	}
}
