# Troubleshooting

## "not a PDF file: invalid header"

Input is not a real PDF (often HTML content with `.pdf` extension).

Check with:

```bash
file your_input.pdf
```

## OCR not running

Ensure both commands exist:

```bash
which pdftoppm
which tesseract
```

## Benchmark fails with missing truth

Ground-truth file is missing or empty. Populate truth text before running gate.

## High CER/WER but pass rate looks good

Pass rate only measures extraction success, not textual fidelity.
Increase OCR usage or improve truth alignment and thresholds.

## Timeouts on low-memory hosts

- use `--workers 1`
- increase `--timeout-page-sec`
- keep deterministic mode
- disable VLM rescue

## VLM issues

If Ollama is unstable/unavailable, run deterministic mode only.
