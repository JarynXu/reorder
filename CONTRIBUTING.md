# Contributing

Contributions are welcome through GitHub issues and pull requests.

## Principles

- Keep ordering semantics aligned with `funcorder`; new style policy belongs upstream first.
- Prefer a conservative no-fix result over an edit whose semantic or comment ownership is uncertain.
- Every bug fix should include a regression test.
- Preserve deterministic, idempotent output.
- Avoid new dependencies unless they materially improve correctness or interoperability.

## Development

Requirements: Go 1.23 or newer.

```bash
make check
```

Before opening a pull request, ensure:

```bash
gofmt -w .
go vet ./...
go test -race ./...
```

PRs should explain the rule or safety invariant affected and include before/after source examples when behavior changes.

## Commits

Use focused commits with imperative subjects. Conventional prefixes such as `feat:`, `fix:`, `test:`, `docs:`, and `chore:` are encouraged but not required.

## License

By submitting a contribution, you agree that it is licensed under the Apache License 2.0 used by this repository.
