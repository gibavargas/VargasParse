package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vargasparse/internal/progress"
	"vargasparse/internal/quality"
)

func processPage(ctx context.Context, cfg *Config, pageIndex int) PageResult {
	start := time.Now()
	pageNum := pageIndex + 1
	result := PageResult{PageNum: pageNum, ErrorCode: ErrorCodeNone}

	decider := cfg.QualityDecider
	if decider == nil {
		decider = defaultQualityDecider{}
	}

	// Legacy mode allows VLM as primary attempt, but deterministic path still handles failures.
	if cfg.EngineMode == EngineLegacy && cfg.VLMRescueExtractor != nil {
		result.EngineTrace = append(result.EngineTrace, "vlm_primary:start")
		vlmText, vlmSource, vlmErr := cfg.VLMRescueExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
		if vlmErr == nil && strings.TrimSpace(vlmText) != "" {
			q := decider.Assess(vlmText, cfg.Dict)
			if q.Confidence >= 0.70 {
				result.Text = vlmText
				result.Method = progress.MethodFast
				result.Confidence = q.Confidence
				result.Blocks = buildBlock(pageNum, vlmText, vlmSource)
				result.QualitySignals = qualitySignals(q)
				result.LanguageDetected = detectLanguage(vlmText, cfg.LangHint)
				result.EngineTrace = append(result.EngineTrace, "vlm_primary:ok")
				result.Duration = time.Since(start)
				return result
			}
		}
		if vlmErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("vlm primary failed: %v", vlmErr))
		}
		result.EngineTrace = append(result.EngineTrace, "vlm_primary:fallback")
	}

	nativeText, nativeSource, nativeBlocks, nativeTables, nativeErr := cfg.NativeExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
	if nativeErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("native extraction error: %v", nativeErr))
		result.EngineTrace = append(result.EngineTrace, "native:error")
		if isDependencyErr(nativeErr) {
			result.ErrorCode = ErrorCodeDependencyMissing
		} else {
			result.ErrorCode = ErrorCodeNativeFailed
		}
	} else {
		result.EngineTrace = append(result.EngineTrace, "native:"+nativeSource)
	}

	nativeQ := decider.Assess(nativeText, cfg.Dict)
	result.Confidence = nativeQ.Confidence
	result.QualitySignals = qualitySignals(nativeQ)
	result.LanguageDetected = detectLanguage(nativeText, cfg.LangHint)

	decision := profileDecision(cfg.Profile, nativeQ)
	if decision == quality.Accept && strings.TrimSpace(nativeText) != "" {
		result.Text = nativeText
		result.Method = progress.MethodFast
		result.Blocks = nativeBlocks
		result.Tables = nativeTables
		result.Duration = time.Since(start)
		return result
	}

	oText, oConf, oBlocks, oSource, oErrCode, oWarnings := runOCR(ctx, cfg, pageIndex)
	result.Warnings = append(result.Warnings, oWarnings...)
	if strings.TrimSpace(oText) != "" {
		result.OCRApplied = true
		result.EngineTrace = append(result.EngineTrace, "ocr:"+oSource)
	}

	switch decision {
	case quality.Compare:
		if strings.TrimSpace(oText) != "" && oConf > nativeQ.Confidence {
			result.Text = oText
			result.Confidence = oConf
			result.Blocks = oBlocks
		} else {
			result.Text = nativeText
			result.Blocks = nativeBlocks
			result.Tables = nativeTables
		}
		result.Method = progress.MethodCompare
	case quality.Reject:
		if strings.TrimSpace(oText) != "" {
			result.Text = oText
			result.Confidence = oConf
			result.Blocks = oBlocks
			result.Method = progress.MethodOCR
		} else {
			result.Method = progress.MethodOCRFail
			if oErrCode != "" {
				result.ErrorCode = oErrCode
			} else {
				result.ErrorCode = ErrorCodeNoText
			}
		}
	default:
		result.Text = nativeText
		result.Blocks = nativeBlocks
		result.Tables = nativeTables
		result.Method = progress.MethodFast
	}

	needsRescue := shouldUseVLM(cfg) && cfg.VLMRescueExtractor != nil &&
		(strings.TrimSpace(result.Text) == "" || result.Confidence < 0.55)
	if needsRescue {
		result.EngineTrace = append(result.EngineTrace, "vlm_rescue:start")
		vlmText, vlmConf, vlmBlocks, vlmErr := runVLMRescue(ctx, cfg, pageIndex)
		if vlmErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("vlm rescue failed: %v", vlmErr))
			if result.ErrorCode == "" {
				result.ErrorCode = ErrorCodeVLMFailed
			}
			result.EngineTrace = append(result.EngineTrace, "vlm_rescue:error")
		} else if strings.TrimSpace(vlmText) != "" {
			result.Text = vlmText
			if vlmConf > result.Confidence {
				result.Confidence = vlmConf
			}
			result.Blocks = vlmBlocks
			result.Method = progress.MethodCompare
			result.ErrorCode = ErrorCodeNone
			result.EngineTrace = append(result.EngineTrace, "vlm_rescue:ok")
		}
	}

	if strings.TrimSpace(result.Text) == "" {
		result.Method = progress.MethodOCRFail
		if result.ErrorCode == "" {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.ErrorCode = ErrorCodeTimeout
			} else {
				result.ErrorCode = ErrorCodeNoText
			}
		}
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.ErrorCode = ErrorCodeTimeout
		result.Warnings = append(result.Warnings, "page timed out")
	}

	result.Duration = time.Since(start)
	return result
}

func runOCR(ctx context.Context, cfg *Config, pageIndex int) (string, float64, []Block, string, string, []string) {
	pageNum := pageIndex + 1
	var warnings []string

	if cfg.OCRExtractor == nil {
		warnings = append(warnings, "OCR extractor unavailable")
		return "", 0, nil, "", ErrorCodeDependencyMissing, warnings
	}

	text, source, err := cfg.OCRExtractor.Extract(ctx, cfg.PDFPath, pageIndex, cfg.LangHint)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("OCR failed: %v", err))
		if isDependencyErr(err) {
			return "", 0, nil, source, ErrorCodeDependencyMissing, warnings
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", 0, nil, source, ErrorCodeTimeout, warnings
		}
		return "", 0, nil, source, ErrorCodeOCRFailed, warnings
	}

	if strings.TrimSpace(text) == "" {
		warnings = append(warnings, "OCR returned empty text")
		return "", 0, nil, source, ErrorCodeNoText, warnings
	}

	q := cfg.QualityDecider.Assess(text, cfg.Dict)
	blocks := []Block{{
		ID:           fmt.Sprintf("p%d_ocr", pageNum),
		Type:         "paragraph",
		Text:         text,
		ReadingOrder: 0,
		Confidence:   q.Confidence,
		SourceMethod: source,
	}}
	return text, q.Confidence, blocks, source, ErrorCodeNone, warnings
}

func runVLMRescue(ctx context.Context, cfg *Config, pageIndex int) (string, float64, []Block, error) {
	if cfg.VLMRescueExtractor == nil {
		return "", 0, nil, fmt.Errorf("vlm rescue extractor unavailable")
	}
	text, source, err := cfg.VLMRescueExtractor.Extract(ctx, cfg.PDFPath, pageIndex)
	if err != nil {
		return "", 0, nil, err
	}
	if strings.TrimSpace(text) == "" {
		return "", 0, nil, nil
	}
	q := cfg.QualityDecider.Assess(text, cfg.Dict)
	blocks := []Block{{
		ID:           fmt.Sprintf("p%d_vlm", pageIndex+1),
		Type:         "paragraph",
		Text:         text,
		ReadingOrder: 0,
		Confidence:   q.Confidence,
		SourceMethod: source,
	}}
	return text, q.Confidence, blocks, nil
}
