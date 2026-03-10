# Architecture

## Goals

- Deterministic, reproducible parsing first.
- Local/offline operation by default.
- Explainable per-page decisions.
- Clear failure modes and benchmark gating.

## High-Level Flow

1. **Native extraction**
- `pdftotext` per page.
- fallback: Go text extraction via `ledongthuc/pdf`.

2. **Quality scoring**
- Signals include printable ratio, lexicon score, garbage score,
  entropy/consistency/symbol/repetition heuristics.
- Produces `accept | compare | reject`.

3. **OCR fallback**
- Rasterize with `pdftoppm`.
- OCR with `tesseract`.

4. **Optional rescue**
- If enabled and quality still low: VLM rescue through Ollama.

5. **Output + report**
- Per page: method, confidence, warnings, `engine_trace`,
  `quality_signals`, `ocr_applied`, `error_code`.
- Document-level report and benchmark gate output.

## Engine Modes

- `deterministic`: native + OCR; no VLM unless explicitly enabled.
- `hybrid`: deterministic path + VLM rescue allowed.
- `legacy`: VLM-first attempt, then deterministic fallback.

## Key Packages

- `internal/pipeline`: orchestration, routing, worker pool.
- `internal/quality`: scoring heuristics and decision.
- `internal/ocr`: OCR adapter (`pdftoppm + tesseract`).
- `internal/output`: structured document/report/benchmark serialization.
- `internal/table`: table extraction from page geometry.
- `internal/deps`: preflight dependency checks.

## Pipeline File Map

`internal/pipeline` is split by responsibility:

- `pipeline_types.go`: shared public/internal pipeline types and interfaces.
- `pipeline_pdf.go`: PDF page counting helper.
- `pipeline_extractors.go`: default native/OCR/VLM extractor adapters and conversion helpers.
- `pipeline_quality.go`: language parsing/detection and quality decision helpers.
- `pipeline_orchestrator.go`: per-page decision tree (`processPage`, OCR/VLM routing).
- `pipeline_run.go`: worker pool runtime (`Run`) and worker-count logic.

This split is intentional and behavior-preserving: same package and exported symbols, cleaner ownership.

## Dependency Wiring Ownership

- `pipeline.Run` no longer instantiates concrete dependencies.
- Runtime construction is owned by `cmd/vargasparse` via `buildRuntime(...)` in `cmd/vargasparse/runtime.go`.
- The cmd layer handles:
- default extractor/decider wiring
- optional VLM/OCR initialization
- cleanup lifecycle (`defer runtimeDeps.cleanup()`)
- degrade-vs-fail policy with `--fail-on-missing-deps`

## Failure Taxonomy

Per-page machine-readable `error_code` values include:

- `timeout`
- `dependency_missing`
- `native_failed`
- `ocr_failed`
- `vlm_failed`
- `no_text`

## Concurrency Model

- Worker pool with per-page timeout context.
- Results channel is sorted by page number before final output.
- Memory-aware worker auto-selection (`ComputeWorkers`) with caps.
