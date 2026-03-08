# CLI Reference

## Usage

```bash
vargasparse [flags] <input.pdf> [output.txt|output.md|output.json]
```

## Main Flags

- `--format txt|md|json|all`
- `--lang auto|por,eng,...`
- `--profile accuracy|balanced`
- `--workers N`
- `--timeout-page-sec N`

## Engine/Rescue Flags

- `--engine deterministic|hybrid|legacy`
- `--enable-vlm-rescue`
- `--model <ollama_model_name>`
- `--render-dpi <float>` DPI for rasterizing PDF pages for VLM (default `150`)
- `--vlm-retry-delay <duration>` delay between VLM retry attempts (default `300ms`)

## Input Source Flags

- `--input-url <pdf_url>` download and process one PDF from URL
- `--batch-path <dir>` process all valid PDFs in a directory
- `--default-batch-path <dir>` directory used for batch mode when no input argument is provided
- `--batch-output-dir <dir>` output destination for batch files

## Reporting/Gating Flags

- `--emit-report <path>`
- `--benchmark-report <path>`
- `--min-pass-rate <percent>`
- `--max-fail-rate <percent>`
- `--fail-on-missing-deps`

## Exit Codes

- `0`: success
- `1`: execution/config/dependency error
- `2`: benchmark gate failed

## Examples

Deterministic parse:

```bash
./vargasparse --engine deterministic input.pdf output.json
```

Accuracy profile with conservative workers:

```bash
./vargasparse --engine deterministic --profile accuracy --workers 1 input.pdf output.md
```

Benchmark gate:

```bash
./vargasparse \
  --engine deterministic \
  --benchmark-report /tmp/report.json \
  --min-pass-rate 97 \
  --max-fail-rate 0.5 \
  input.pdf output.txt
```
