# Security Policy

## Supported Scope

This project processes local files and executes local binaries (`pdftotext`,
`pdftoppm`, `tesseract`, optionally `ollama`).

## Reporting

Report security issues privately to project maintainers before public disclosure.

## Threat Notes

- Treat input PDFs as untrusted data.
- Keep parsing dependencies up to date.
- Avoid enabling network-exposed model daemons unless required.
- Run on isolated infrastructure for sensitive workloads.

## Hardening Suggestions

- Run with least privilege.
- Prefer deterministic mode in production.
- Use explicit dependency preflight with `--fail-on-missing-deps`.
