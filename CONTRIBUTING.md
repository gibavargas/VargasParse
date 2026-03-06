# Contributing

## Development Flow

1. Create a branch.
2. Implement changes with tests.
3. Run checks:

```bash
go test ./...
go vet ./...
make benchmark-gate
```

4. Open PR with:
- change summary
- risk assessment
- before/after metrics (if parser behavior changed)

## Code Standards

- Keep deterministic behavior as default.
- Add explicit error codes for new failure modes.
- Maintain backward compatibility for JSON output fields.
- Add tests for routing, failure handling, and metrics logic.

## Commit Guidance

Use clear commit messages describing behavior changes, not only refactors.

## Licensing

By contributing, you agree your contribution is distributed under GPL v2.
