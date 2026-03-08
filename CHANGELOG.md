# Changelog

## Unreleased

### Added

- Deterministic-first extraction flow with explicit engine modes.
- OCR adapter backed by `pdftoppm + tesseract`.
- Optional VLM rescue path with retry.
- Extended per-page diagnostics in JSON output.
- Benchmark gate reporting with pass/fail thresholds.
- Ground-truth presence enforcement for benchmark runs.
- Heuristic table extraction from positioned page text.
- New docs set and GPL v2 license.
- Risk-based coverage gate script (`scripts/test-risk-gates.sh`) and Make targets:
  - `test-cover`
  - `test-smoke`
  - `test-risk-gates`
- Opt-in smoke tests for OCR and VLM (`VARGASPARSE_SMOKE`, `VARGASPARSE_TEST_OLLAMA`).
- Configurable VLM render DPI (`--render-dpi`) and retry delay (`--vlm-retry-delay`) flags, replacing hardcoded values.

### Changed

- README updated with architecture and docs index.
- Make targets expanded for benchmark workflows.
- `internal/pipeline/pipeline.go` split by responsibility into focused files.
- Dependency construction moved out of pipeline runtime path into cmd-layer runtime builder (`cmd/vargasparse/runtime.go`).
- Expanded automated test coverage for `cmd/vargasparse`, `internal/deps`, `internal/llamacpp`, `internal/progress`, `internal/renderer`, and `internal/pipeline`.

### Notes

- Benchmark corpus currently excludes non-PDF artifacts.
