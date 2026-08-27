# reorder

`reorder` safely rewrites Go source files so their functions, constructors, and methods follow the ordering rules enforced by [`funcorder`](https://github.com/manuelarte/funcorder).

The project exists to close one specific gap: `funcorder` can diagnose declaration-order violations, but its previous autofix implementation was reverted after it could delete comments inside moved functions. `reorder` uses a different approach: the AST decides **where** declarations belong, while the original source bytes decide **what** gets moved.

## Status

The current rule model is compatible with `funcorder` v0.6.0 semantics:

- constructors after their declared type and before that type's methods;
- exported methods before unexported methods;
- optional alphabetical ordering inside constructor and method groups;
- optional exported-before-unexported top-level function ordering;
- `init` excluded from the top-level function rule.

`reorder` deliberately does not invent additional style rules. In particular, it does not implement `funcorder` TODOs that are not currently diagnostics.

## Safety model

Only ordinary function declarations are movable. Non-function declarations and `init` functions are anchors, so `const`, `var`, `type`, import, and `init` relative order is preserved.

Moved functions are copied from their original source bytes. Function-body comments, doc comments, compiler directives, line endings, and formatting inside a moved declaration are therefore preserved rather than regenerated from a partial AST.

If satisfying a rule would require crossing standalone, unattached source trivia such as a section comment, `reorder` refuses the automatic edit with an error instead of guessing the comment's ownership.

See [`docs/design.md`](docs/design.md) for the algorithm and invariants.

## Install

```bash
go install github.com/JarynXu/reorder/cmd/reorder@latest
```

The module requires Go 1.23 or newer and has no runtime dependencies outside the standard library.

## Usage

Use stdin/stdout as an external formatter:

```bash
reorder < service.go > service.reordered.go
```

Rewrite a file or directory in place:

```bash
reorder -write service.go
reorder -write ./internal
reorder -write ./...
```

Check in CI without modifying files:

```bash
reorder -check ./...
```

`-check` prints files that need reordering and exits with status `1`. Parse errors, unsafe edits, and I/O errors exit with status `2`.

### Rule flags

The rule flags intentionally mirror `funcorder`:

```text
-constructor=true
-struct-method=true
-alphabetical=false
-function=false
```

For example:

```bash
reorder -write -alphabetical -function ./...
```

## Editor integration

`reorder` supports both common external-tool contracts:

- **stdin -> stdout** for editor formatter integrations;
- **file -> in-place** via `reorder -write <file>` for save hooks and file watchers.

Run `reorder -h` for the complete CLI contract. Keep `gofmt`, `gofumpt`, or `goimports` in the normal formatting pipeline; `reorder` is a declaration arranger, not a replacement for Go formatting.

A typical save pipeline is:

```text
reorder -> goimports/gofumpt
```

## Library API

```go
cfg := reorder.DefaultConfig()
cfg.Alphabetical = true

out, changed, err := reorder.Rewrite("service.go", source, cfg)
```

For linter/IDE integration, `PlanFile` returns a single byte-offset edit that maps directly to a Go `analysis.TextEdit`. See [`docs/upstream.md`](docs/upstream.md).

## Development

```bash
make check
```

The test suite includes regression coverage for the historical `funcorder` comment-loss bug, idempotency, directives, CRLF, `init`/package-variable ordering, conservative comment barriers, and CLI batch safety.

## Relationship to funcorder

`reorder` is an independent project and is not an official `funcorder` component. Its ordering behavior is intentionally derived from `funcorder`'s public rule semantics so the implementation can be evaluated for a future upstream contribution.

## License

Apache License 2.0. See [`LICENSE`](LICENSE).
