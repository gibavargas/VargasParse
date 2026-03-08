# VargasParse

VargasParse is an offline-first PDF parsing pipeline focused on deterministic extraction quality for PT/EN documents.

## License

This project is licensed under **GNU GPL v2**.

- License file: [LICENSE](./LICENSE)
- SPDX intent: `GPL-2.0-only`

## What It Does

- Extracts text from PDFs using deterministic engines first (`pdftotext` + Go fallback).
- Uses OCR fallback (`pdftoppm` + `tesseract`) when quality is uncertain.
- Optionally uses VLM rescue (Ollama) for difficult pages.
- Shows a live BubbleTea TUI progress dashboard in TTY terminals.
- Produces `.txt`, `.md`, and `.json` with per-page diagnostics.
- Supports benchmark gating with pass/fail thresholds and CER/WER reporting.

## Quick Start

```bash
go build -o vargasparse ./cmd/vargasparse
./vargasparse --engine deterministic input.pdf output.json
```

When running in a non-TTY environment (CI, redirected output), progress automatically falls back to plain text logs.

## Core Commands

```bash
# Unit/integration checks
make test

# Coverage report
make test-cover

# Risk-based coverage gates
make test-risk-gates

# Full quality gate locally (same checks as CI)
go test ./... && go vet ./... && make test-risk-gates

# Optional smoke tests (real local OCR; VLM smoke only if VARGASPARSE_TEST_OLLAMA=1)
make test-smoke

# Benchmark one sample
make benchmark

# Manifest benchmark gate
make benchmark-gate
```

## Testing Policy

- Coverage is risk-based, not blanket-global:
- `internal/pipeline >= 70%`
- `internal/deps >= 80%`
- `internal/progress >= 60%`
- `internal/renderer >= 60%`
- `internal/llamacpp >= 55%`
- `internal/ocr >= 55%`
- `cmd/vargasparse >= 40%`
- Smoke tests are opt-in (`VARGASPARSE_SMOKE=1`) because they require external binaries/models.

## Quality Gates

- GitHub Actions runs the default quality gate on every PR and push to `main`:
- `go test ./...`
- `go vet ./...`
- `./scripts/test-risk-gates.sh`
- Smoke tests are intentionally excluded from default CI and should run on dedicated environments only.

## Input Modes

- Local file: `./vargasparse input.pdf output.json`
- PDF URL: `./vargasparse --input-url https://example.com/file.pdf output.json`
- Batch by path: `./vargasparse --batch-path ./incoming_pdfs --batch-output-dir ./out`
- Batch by default path: run with no positional input (uses `--default-batch-path`, default `test_pdfs`)

## Documentation Index

- [Architecture](./docs/ARCHITECTURE.md)
- [CLI Reference](./docs/CLI_REFERENCE.md)
- [Benchmarking & Quality Gates](./docs/BENCHMARKING.md)
- [Operations Guide](./docs/OPERATIONS.md)
- [Troubleshooting](./docs/TROUBLESHOOTING.md)
- [Contributing](./CONTRIBUTING.md)
- [Security Policy](./SECURITY.md)
- [Changelog](./CHANGELOG.md)

## Runtime Requirements

Deterministic mode requires local binaries:

- `pdftoppm` (Poppler)
- `pdftotext` (Poppler; optional but recommended)
- `tesseract` (`eng` and `por` language packs recommended)

Hybrid/legacy rescue mode additionally uses:

- `ollama` + a local vision model

## Current Corpus Note

`test_pdfs/chemistry.pdf` and `test_pdfs/rna_biology.pdf` are currently HTML files (not valid PDFs). They are excluded from benchmark manifest until replaced by real PDFs.
