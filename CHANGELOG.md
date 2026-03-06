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

### Changed

- README updated with architecture and docs index.
- Make targets expanded for benchmark workflows.

### Notes

- Benchmark corpus currently excludes non-PDF artifacts.
