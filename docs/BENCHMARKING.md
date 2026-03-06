# Benchmarking & Quality Gates

## Purpose

Benchmarking ensures parser quality remains stable and regressions fail fast.

## Inputs

- PDF file
- Ground-truth text file (Markdown/plain text)

The benchmark path now treats missing/empty truth as a hard failure.

## Metrics

- **Pass rate** (% pages with non-empty text and confidence >= threshold)
- **Fail rate**
- **CER** (character error rate)
- **WER** (word error rate)

## Gate Thresholds

Defaults:

- `min-pass-rate = 97.0`
- `max-fail-rate = 0.5`

Configurable via CLI.

## Single-File Benchmark

```bash
./vargasparse \
  --engine deterministic \
  --benchmark-report /tmp/bench.json \
  --min-pass-rate 97 \
  --max-fail-rate 0.5 \
  test_pdfs/attention.pdf /tmp/attention.txt
```

## Manifest Benchmark

```bash
go run ./cmd/benchmark \
  --manifest test_pdfs/corpus_manifest.json \
  --binary ./vargasparse \
  --out-dir /tmp/vargasparse-benchmark
```

## Corpus Rules

Each manifest entry should have:

- valid PDF path
- non-empty truth path
- explicit thresholds

Invalid PDF artifacts (for example HTML masquerading as PDF) should not be listed.

## Interpreting Results

- `truth_present=false`: invalid benchmark setup; fix corpus first.
- high pass rate + high CER/WER: parser is producing text but fidelity may be low.
- low pass rate: routing or extraction engine failure likely.
