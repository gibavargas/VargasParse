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
