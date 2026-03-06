# Operations Guide

## Install Dependencies (Ubuntu)

```bash
sudo apt-get update
sudo apt-get install -y poppler-utils tesseract-ocr tesseract-ocr-eng tesseract-ocr-por
```

Optional for VLM rescue:

```bash
# Install Ollama using official method for your distro
```

## Build

```bash
go build -o vargasparse ./cmd/vargasparse
```

## Recommended Runtime Profiles

Low-resource VM:

```bash
./vargasparse --engine deterministic --workers 1 --timeout-page-sec 180 input.pdf out.json
```

Higher resource host:

```bash
./vargasparse --engine deterministic --workers 4 input.pdf out.json
```

## Preflight Behavior

Dependency checks are engine-aware:

- deterministic: OCR/native tools required
- hybrid/legacy: Ollama may be required/optional depending on mode

Use `--fail-on-missing-deps` in production automation.

## Artifacts

- output content (`txt`, `md`, `json`)
- optional extraction report (`--emit-report`)
- optional benchmark report (`--benchmark-report`)

## Upgrade Checklist

1. `go test ./...`
2. `go vet ./...`
3. benchmark gate on manifest
4. inspect report diffs before deployment
