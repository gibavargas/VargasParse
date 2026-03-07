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
3. `make test-risk-gates`
4. benchmark gate on manifest
5. inspect report diffs before deployment

## CI Pipeline

Default GitHub Actions pipeline (`.github/workflows/ci.yml`) runs on every PR and push to `main`:

1. `go test ./...`
2. `go vet ./...`
3. `./scripts/test-risk-gates.sh`

Smoke tests are excluded from default CI because they depend on local OCR/VLM binaries.

## CI Failure Triage

1. If `go test` fails, fix regressions first before rerunning any gates.
2. If `go vet` fails, address correctness warnings (not formatting-only issues).
3. If risk-gates fail, inspect package coverage drift and add targeted tests.
4. If benchmark gate fails, inspect benchmark report and compare CER/WER + pass/fail rates against previous baseline.

## Smoke Tests (Opt-In)

Use smoke tests only in environments with local binaries/models configured.

```bash
# OCR smoke only
VARGASPARSE_SMOKE=1 go test -v -run Smoke ./internal/ocr

# OCR + VLM smoke
VARGASPARSE_SMOKE=1 VARGASPARSE_TEST_OLLAMA=1 go test -v -run Smoke ./internal/ocr ./internal/llamacpp
```

Default CI should keep these off unless a dedicated smoke runner is provisioned.
