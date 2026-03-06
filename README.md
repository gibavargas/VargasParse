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
- Produces `.txt`, `.md`, and `.json` with per-page diagnostics.
- Supports benchmark gating with pass/fail thresholds and CER/WER reporting.

## Quick Start

```bash
go build -o vargasparse ./cmd/vargasparse
./vargasparse --engine deterministic input.pdf output.json
```

## Core Commands

```bash
# Unit/integration checks
make test

# Benchmark one sample
make benchmark

# Manifest benchmark gate
make benchmark-gate
```

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
